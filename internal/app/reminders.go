package app

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"go-organizer/internal/store"
)

// eventReminders sends "in N min" reminders. Rule: a reminder is due when
// remind time <= now < event start and it was not sent yet. So a reminder that
// came due while the bot was down or during quiet hours still goes out, but only
// before the event starts.
func (a *App) eventReminders(ctx context.Context, now time.Time) {
	offsets := a.cfg.Reminders.EventOffsetsMin
	if len(offsets) == 0 {
		return
	}
	lead := time.Duration(slices.Max(offsets)) * time.Minute

	events, err := a.store.TimedEventsBetween(ctx, now, now.Add(lead).Add(time.Minute))
	if err != nil {
		a.log.Error("reminders", "err", err)
		return
	}
	for _, e := range events {
		if err := a.remind(ctx, now, e, offsets); err != nil {
			a.log.Error("reminder", "event_id", e.ID, "err", err)
		}
	}
}

func (a *App) remind(ctx context.Context, now time.Time, e store.Event, offsets []int) error {
	if !e.Start.After(now) {
		return nil
	}

	// All due and unsent offsets go out as one message: after downtime an event
	// 10 minutes away must not get both "60 min" and "15 min" at once.
	var due []string
	for _, m := range offsets {
		if now.Before(e.Start.Add(-time.Duration(m) * time.Minute)) {
			continue
		}
		kind := fmt.Sprintf("%dm", m)
		sent, err := a.store.WasSent(ctx, e.ID, e.Start, kind)
		if err != nil {
			return err
		}
		if !sent {
			due = append(due, kind)
		}
	}
	if len(due) == 0 {
		return nil
	}

	text, err := a.render("reminder", struct {
		Event   eventView
		Minutes int
	}{a.view(e), int(math.Ceil(e.Start.Sub(now).Minutes()))})
	if err != nil {
		return err
	}
	id, err := a.send.Send(ctx, a.cfg.ChatID, text)
	if err != nil {
		return err // not marked, the next tick retries
	}
	for _, kind := range due {
		if err := a.store.MarkSent(ctx, e.ID, e.Start, kind, id, now); err != nil {
			return err
		}
	}
	return nil
}

// view prepares an event for a template: timed events in the user's zone,
// all-day events keep their date as is.
func (a *App) view(e store.Event) eventView {
	v := eventView{Title: e.Title, Start: e.Start, End: e.End, AllDay: e.AllDay, Location: e.Location}
	if !e.AllDay {
		v.Start = e.Start.In(a.Loc())
		if !e.End.IsZero() {
			v.End = e.End.In(a.Loc())
		}
	}
	return v
}
