package app

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// Sender is implemented by tg.
type Sender interface {
	Send(ctx context.Context, chatID int64, text string) (messageID int64, err error)
	SendButtons(ctx context.Context, chatID int64, text string, keyboard [][]tg.Button) (messageID int64, err error)
	Edit(ctx context.Context, chatID, messageID int64, text string, keyboard [][]tg.Button) error
	Delete(ctx context.Context, chatID, messageID int64) error
}

type App struct {
	store *store.Store
	send  Sender
	cal   Calendar // nil: no calendar — no sync, no event reminders, no evening message
	cfg   *config.Config
	texts *texts
	log   *slog.Logger
	loc   atomic.Pointer[time.Location]
	lang  atomic.Pointer[string]
}

// New loads the zone and the language from state. On the very first run state is empty,
// so cfg.TZ and cfg.Lang are saved there; after that config never sets them again.
func New(ctx context.Context, st *store.Store, send Sender, cal Calendar, cfg *config.Config, log *slog.Logger) (*App, error) {
	tx, err := loadTexts()
	if err != nil {
		return nil, err
	}
	a := &App{store: st, send: send, cal: cal, cfg: cfg, texts: tx, log: log}

	tz, err := st.TZ(ctx)
	if err != nil {
		return nil, err
	}
	if tz == "" {
		tz = cfg.TZ
	}
	if err := a.SetTZ(ctx, tz); err != nil {
		return nil, err
	}

	lang, err := st.Lang(ctx)
	if err != nil {
		return nil, err
	}
	if lang == "" {
		lang = cfg.Lang
	}
	if err := a.SetLang(ctx, lang); err != nil {
		return nil, err
	}
	return a, nil
}

// Loc is the only source of the user's zone: rendering, anchors and parsing all go through it.
func (a *App) Loc() *time.Location {
	return a.loc.Load()
}

// SetTZ takes an IANA name, not an offset: offsets break on DST.
func (a *App) SetTZ(ctx context.Context, name string) error {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("tz %q: %w", name, err)
	}
	if err := a.store.SetTZ(ctx, name); err != nil {
		return err
	}
	a.loc.Store(loc)
	return nil
}

// Lang is the interface language: every text goes through it, never through cfg.Lang.
func (a *App) Lang() string {
	return *a.lang.Load()
}

func (a *App) SetLang(ctx context.Context, lang string) error {
	if !slices.Contains(config.Languages, lang) {
		return fmt.Errorf("lang %q: want one of %v", lang, config.Languages)
	}
	if err := a.store.SetLang(ctx, lang); err != nil {
		return err
	}
	a.lang.Store(&lang)
	return nil
}

// render is the only way to produce a user-facing text.
func (a *App) render(name string, data any) (string, error) {
	return a.texts.render(a.Lang(), name, data)
}

// label renders a button caption; a broken template must not hide the button.
func (a *App) label(name string, data any) string {
	text, err := a.render(name, data)
	if err != nil {
		a.log.Error("render label", "err", err)
		return name
	}
	return text
}
