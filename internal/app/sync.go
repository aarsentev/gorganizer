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
	Create(ctx context.Context, e gcal.NewEvent) (gcal.Event, error)
	Check(ctx context.Context) error
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
	return a.followCalendarZone(ctx, now, changes.Zone)
}

// followCalendarZone switches the bot to the calendar's zone when that zone differs from
// the one seen at the last sync. /tz is not undone: the last change wins, whichever side
// made it. On the first sync nothing is seen yet, so a new install takes the calendar's zone.
func (a *App) followCalendarZone(ctx context.Context, now time.Time, zone string) error {
	if zone == "" {
		return nil
	}
	seen, err := a.store.CalendarTZ(ctx)
	if err != nil || zone == seen {
		return err
	}
	if zone != a.Loc().String() {
		if err := a.SetTZ(ctx, zone); err != nil {
			return err
		}
		a.log.Info("zone from calendar", "zone", zone)
		a.replyBlock(ctx, a.cfg.ChatID, "tz_from_calendar", tzView{Zone: zone, Now: now.In(a.Loc())})
	}
	// Saved after the switch: a crash in between switches again on the next sync, not never.
	return a.store.SetCalendarTZ(ctx, zone)
}
