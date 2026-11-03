package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go-organizer/internal/gcal"
	"go-organizer/internal/intent"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// eventCard is the confirmation card of an event draft. It always shows the zone:
// on the day of a flight a wrong zone is the most expensive mistake (SPEC §9).
type eventCard struct {
	Title     string
	Start     time.Time // in the user's zone
	End       time.Time
	SameDay   bool // otherwise the end shows its date too
	Zone      string
	Location  string
	Remind    *whenView // the first reminder; nil if there is none or it comes at once
	RemindNow bool      // the first reminder time has passed: it is sent right after creating
	Created   bool
	Failed    bool // the calendar refused: the buttons stay for another try
}

func (a *App) eventCard(in intent.Intent, now time.Time) eventCard {
	loc := a.Loc()
	start := in.Start.In(loc)
	end := start.Add(time.Duration(in.DurationMin) * time.Minute)
	c := eventCard{
		Title: in.Title, Start: start, End: end, SameDay: midnight(start).Equal(midnight(end)),
		Zone: loc.String(), Location: in.Location,
	}
	// The earliest offset comes first; a time already passed is sent at once (reminders.go).
	if offsets := a.cfg.Reminders.EventOffsetsMin; len(offsets) > 0 {
		first := start.Add(-time.Duration(slices.Max(offsets)) * time.Minute)
		if first.After(now) {
			w := a.when(first, now)
			c.Remind = &w
		} else {
			c.RemindNow = true
		}
	}
	return c
}

func (a *App) eventKeyboard(id int64) [][]tg.Button {
	return [][]tg.Button{{
		{Text: a.label("btn_create", nil), Data: buttonData("evt", id, "create")},
		{Text: a.label("btn_cancel", nil), Data: buttonData("evt", id, "cancel")},
	}}
}

// onEvent handles the buttons under the event card; id is the draft's.
func (a *App) onEvent(ctx context.Context, now time.Time, cb tg.Callback, id int64, action string) error {
	p, in, err := a.draft(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		// Already created or cancelled: the second tap of a double tap. Editing the card
		// now would overwrite what the first tap wrote there.
		a.log.Info("event card without a draft", "draft_id", id, "action", action)
		return nil
	}
	if err != nil {
		return err
	}
	switch action {
	case "cancel":
		if err := a.store.DeletePending(ctx, id); err != nil {
			return err
		}
		return a.editBlock(ctx, cb, "event_cancelled", in, nil)
	case "create":
		return a.createEvent(ctx, now, cb, p, in)
	}
	return errors.New("unknown event action " + action)
}

// createEvent writes the draft to Google, puts the event into the cache so that its
// reminder does not wait for the next sync, and closes the draft.
func (a *App) createEvent(ctx context.Context, now time.Time, cb tg.Callback, p store.Pending, in intent.Intent) error {
	if !in.Start.After(now) {
		// The card waited until the start had passed: an event in the past is not created.
		if err := a.store.DeletePending(ctx, p.ID); err != nil {
			return err
		}
		return a.editBlock(ctx, cb, "add_past", in.Start.In(a.Loc()), nil)
	}
	card := a.eventCard(in, now)
	e, err := a.cal.Create(ctx, gcal.NewEvent{
		ID:       eventID(p),
		Title:    in.Title,
		Start:    card.Start,
		End:      card.End,
		Location: in.Location,
		Zone:     card.Zone,
	})
	if err != nil {
		// The draft stays, the buttons too: pressing again retries with the same id.
		a.log.Error("create event", "draft_id", p.ID, "err", err)
		card.Failed = true
		return a.editBlock(ctx, cb, "event_card", card, a.eventKeyboard(p.ID))
	}
	// The event is in Google now. What follows only logs: a failure here must not show
	// the card as failed and invite a retry; the next sync brings the event anyway.
	if err := a.store.UpsertEvents(ctx, []store.Event{store.Event(e)}); err != nil {
		a.log.Error("cache created event", "event_id", e.ID, "err", err)
	}
	if err := a.store.DeletePending(ctx, p.ID); err != nil {
		a.log.Error("close draft", "draft_id", p.ID, "err", err)
	}
	card.Created = true
	return a.editBlock(ctx, cb, "event_card", card, nil)
}

// eventID is the Google id of the draft's event, the same on every retry: a repeat
// after a lost response gets "already exists" instead of a second event. The draft's
// time is in it because draft ids start from 1 again in a new database, and an old
// event's id must not make a new event look already created. "v" between them is a
// letter Google allows (0-9, a-v) and keeps the two numbers apart.
func eventID(p store.Pending) string {
	return fmt.Sprintf("org%dv%d", p.ID, p.Updated.Unix())
}
