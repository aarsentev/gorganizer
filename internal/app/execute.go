package app

import (
	"slices"
	"time"

	"go-organizer/internal/intent"
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
