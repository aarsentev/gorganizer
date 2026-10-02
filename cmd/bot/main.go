package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata"

	"golang.org/x/sync/errgroup"

	"go-organizer/internal/app"
	"go-organizer/internal/config"
	"go-organizer/internal/gcal"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Fatal", "err", err)
		os.Exit(1)
	}
}

// run holds all the logic so deferred Close calls execute before os.Exit.
func run() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	tgClient, err := tg.New(cfg.BotToken, cfg.ChatID, log)
	if err != nil {
		return err
	}

	// A broken calendar setup is a startup error: silently running without it
	// would only show up as a reminder that never came.
	cal, err := gcal.New(ctx, cfg.SAFile, cfg.CalendarID)
	if err != nil {
		return err
	}

	a, err := app.New(ctx, st, tgClient, cal, cfg, log)
	if err != nil {
		return err
	}

	log.Info("Started", "tz", a.Loc().String(), "lang", a.Lang(), "db", cfg.DBPath)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return tgClient.Run(ctx, func(ctx context.Context, m tg.Message) {
			a.HandleText(ctx, time.Now(), m.ChatID, m.Text)
		})
	})
	g.Go(func() error { return app.Every(ctx, 5*time.Minute, a.Sync) })
	g.Go(func() error { return app.Every(ctx, time.Minute, a.Tick) })
	err = g.Wait()

	log.Info("Stopped")
	return err
}
