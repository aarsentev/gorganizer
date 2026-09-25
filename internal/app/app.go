package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"go-organizer/internal/store"
)

// Sender is implemented by tg.
type Sender interface {
	Send(ctx context.Context, chatID int64, text string) error
}

type App struct {
	store *store.Store
	send  Sender
	log   *slog.Logger
	loc   atomic.Pointer[time.Location]
}

// New loads the time zone from state. On the very first run state is empty,
// so defaultTZ (from config.yaml) is saved there and config is never read again.
func New(ctx context.Context, st *store.Store, send Sender, defaultTZ string, log *slog.Logger) (*App, error) {
	a := &App{store: st, send: send, log: log}

	tz, err := st.TZ(ctx)
	if err != nil {
		return nil, err
	}
	if tz == "" {
		tz = defaultTZ
	}
	if err := a.SetTZ(ctx, tz); err != nil {
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
