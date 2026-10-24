package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/intent"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
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

func TestAddShowsCardAndCancels(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00-10:00 приём у врача @ клиника на Ленина 12"})
	if send.sent[0] != "📅 Приём у врача\nср, 30 сен, 09:00–10:00 (Europe/Warsaw)\n📍 клиника на Ленина 12\nНапомню завтра в 08:00" {
		t.Fatalf("card = %q", send.sent[0])
	}
	if !sameRows(dataOf(send.keyboards[0]), [][]string{{"✕ Отмена=evt:1:cancel"}}) {
		t.Fatalf("card buttons = %v", dataOf(send.keyboards[0]))
	}
	if _, err := st.PendingFor(ctx, 42); err != nil {
		t.Fatalf("no draft behind the card: %v", err)
	}

	press(a, send, now, 1, "evt:1:cancel")
	if ed := lastEdit(t, send); ed.text != "✕ Приём у врача — отменено" || ed.keyboard != nil {
		t.Fatalf("after cancel = %q %v", ed.text, ed.keyboard)
	}
	if _, err := st.PendingFor(ctx, 42); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("draft after cancel: %v", err)
	}

	// A double tap: the second press finds no draft and leaves the card as it is.
	edits := len(send.edits)
	press(a, send, now, 1, "evt:1:cancel")
	if len(send.edits) != edits {
		t.Fatalf("the second tap edited the card")
	}
}
