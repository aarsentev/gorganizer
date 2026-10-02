package app

import (
	"context"
	"errors"
	"time"

	"go-organizer/internal/gcal"
	"go-organizer/internal/store"
)

// Calendar is implemented by gcal.
type Calendar interface {
	Changes(ctx context.Context, syncToken string, now time.Time) (gcal.Changes, error)
}

// Sync runs every 5 minutes. Errors are only logged: the cache stays as it was
// and reminders keep working from it until the next successful sync.
func (a *App) Sync(ctx context.Context, now time.Time) {
	if a.cal == nil {
		return
	}
	if err := a.sync(ctx, now); err != nil {
		a.log.Error("sync", "err", err)
	}
}

func (a *App) sync(ctx context.Context, now time.Time) error {
	token, err := a.store.SyncToken(ctx)
	if err != nil {
		return err
	}

	changes, err := a.cal.Changes(ctx, token, now)
	if errors.Is(err, gcal.ErrSyncTokenExpired) {
		a.log.Warn("sync token expired, full sync")
		token = ""
		changes, err = a.cal.Changes(ctx, "", now)
	}
	if err != nil {
		return err
	}

	for _, id := range changes.Skipped {
		a.log.Warn("event skipped by sync", "event_id", id)
	}

	// gcal.Event and store.Event have the same fields; the conversion stops compiling if they drift.
	events := make([]store.Event, len(changes.Events))
	for i, e := range changes.Events {
		events[i] = store.Event(e)
	}

	// A full sync is a complete snapshot and replaces the cache; an incremental one patches it.
	full := token == ""
	if full {
		err = a.store.ReplaceEvents(ctx, events)
	} else {
		err = a.store.UpsertEvents(ctx, events)
	}
	if err != nil {
		return err
	}
	// The token is saved only after the events: a crash in between repeats the same changes.
	if err := a.store.SetSyncToken(ctx, changes.SyncToken); err != nil {
		return err
	}

	if full || len(events) > 0 {
		a.log.Info("synced", "full", full, "events", len(events))
	}
	return nil
}
