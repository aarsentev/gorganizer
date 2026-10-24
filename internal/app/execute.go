package app

import (
	"context"
	"errors"
	"slices"
	"time"

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
		{Text: a.label("btn_cancel", nil), Data: buttonData("evt", id, "cancel")},
	}}
}

// onEvent handles the buttons under the event card; id is the draft's.
func (a *App) onEvent(ctx context.Context, cb tg.Callback, id int64, action string) error {
	_, in, err := a.draft(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		// Already cancelled: the second tap of a double tap. Editing the card now
		// would overwrite what the first tap wrote there.
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
	}
	return errors.New("unknown event action " + action)
}
