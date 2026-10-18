package app

import (
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/intent"
)

func intentEvent(title, location string, start *time.Time, minutes int) intent.Intent {
	return intent.Intent{Action: "create", Kind: "event", Title: title, Start: start, DurationMin: minutes, Location: location}
}

func TestEventCard(t *testing.T) {
	now := at(t, "2026-09-29 10:00")
	render := func(a *App, args string) string {
		t.Helper()
		title, location, s, err := parseAdd(args)
		if err != nil {
			t.Fatal(err)
		}
		start, _ := a.moment(s, now)
		in := intentEvent(title, location, start, s.duration)
		text, err := a.render("event_card", a.eventCard(in, now))
		if err != nil {
			t.Fatal(err)
		}
		return text
	}

	a, _, _ := newTestApp(t, withCalendar(), nil)
	cases := map[string]string{
		"30.09 9:00-10:00 Приём у врача @ клиника на Ленина 12": "📅 Приём у врача\nср, 30 сен, 09:00–10:00 (Europe/Warsaw)\n" +
			"📍 клиника на Ленина 12\nНапомню завтра в 08:00",
		// The first reminder time has already passed: it comes right after creating.
		"10:30-11:00 Созвон": "📅 Созвон\nвт, 29 сен, 10:30–11:00 (Europe/Warsaw)\nНапомню сразу",
		// Over midnight the end shows its date.
		"30.09 23:00-7:00 Смена": "📅 Смена\nср, 30 сен, 23:00–чт, 1 окт, 07:00 (Europe/Warsaw)\nНапомню завтра в 22:00",
	}
	for args, want := range cases {
		if got := render(a, args); got != want {
			t.Errorf("%s:\n%s\nwant\n%s", args, got, want)
		}
	}

	// Without offsets there is no reminder to promise.
	quiet, _, _ := newTestApp(t, withCalendar(), func(c *config.Config) { c.Reminders.EventOffsetsMin = nil })
	if got := render(quiet, "30.09 9:00-10:00 Приём"); got != "📅 Приём\nср, 30 сен, 09:00–10:00 (Europe/Warsaw)" {
		t.Errorf("no offsets:\n%s", got)
	}
}
