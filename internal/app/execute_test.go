package app

import (
	"context"
	"errors"
	"fmt"
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
	if !sameRows(dataOf(send.keyboards[0]), [][]string{{"✓ Создать=evt:1:create", "✕ Отмена=evt:1:cancel"}}) {
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

func TestAddCreatesEvent(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{}
	a, send, st := newTestApp(t, cal, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00-10:00 приём у врача @ клиника на Ленина 12"})
	press(a, send, now.Add(time.Minute), 1, "evt:1:create")

	if len(cal.created) != 1 {
		t.Fatalf("created %d events, want 1", len(cal.created))
	}
	e := cal.created[0]
	if e.ID != fmt.Sprintf("org1v%d", now.Unix()) || e.Title != "Приём у врача" || e.Location != "клиника на Ленина 12" ||
		!e.Start.Equal(at(t, "2026-09-30 09:00")) || !e.End.Equal(at(t, "2026-09-30 10:00")) || e.Zone != "Europe/Warsaw" {
		t.Fatalf("created %+v", e)
	}
	card := "📅 Приём у врача\nср, 30 сен, 09:00–10:00 (Europe/Warsaw)\n📍 клиника на Ленина 12"
	if ed := lastEdit(t, send); ed.text != card+"\n✓ В календаре" || ed.keyboard != nil {
		t.Fatalf("after create = %q %v", ed.text, ed.keyboard)
	}
	if _, err := st.PendingFor(ctx, 42); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("draft after create: %v, want it closed", err)
	}

	// A double tap: the second press finds no draft and leaves everything as it is.
	edits := len(send.edits)
	press(a, send, now.Add(time.Minute), 1, "evt:1:create")
	if len(cal.created) != 1 || len(send.edits) != edits {
		t.Fatalf("second tap: %d creates, %d edits", len(cal.created), len(send.edits)-edits)
	}

	// The event is in the cache at once: its reminder and the digest do not wait for a sync.
	a.Tick(ctx, at(t, "2026-09-30 08:00"))
	want := []string{
		"⏰ Приём у врача через 60 мин — 09:00, клиника на Ленина 12",
		"☀️ Сегодня, ср, 30 сен — 1 событие:\n• 09:00–10:00 Приём у врача, клиника на Ленина 12",
	}
	if got := send.sent[len(send.sent)-2:]; !sameStrings(got, want) {
		t.Fatalf("after the tick = %q", got)
	}
}

func TestAddCreateFailsThenRetries(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{createErr: errors.New("google is down")}
	a, send, _ := newTestApp(t, cal, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00-10:00 Приём у врача"})
	press(a, send, now, 1, "evt:1:create")
	ed := lastEdit(t, send)
	if ed.text != "📅 Приём у врача\nср, 30 сен, 09:00–10:00 (Europe/Warsaw)\n⚠️ Не получилось записать в календарь — нажми «Создать» ещё раз" ||
		!sameRows(dataOf(ed.keyboard), [][]string{{"✓ Создать=evt:1:create", "✕ Отмена=evt:1:cancel"}}) {
		t.Fatalf("after a failure = %q %v", ed.text, dataOf(ed.keyboard))
	}

	// The retry goes with the same id: had the first request reached Google, it would answer
	// "already exists" instead of making a second event.
	cal.createErr = nil
	press(a, send, now.Add(2*time.Minute), 1, "evt:1:create")
	if len(cal.created) != 1 || cal.created[0].ID != fmt.Sprintf("org1v%d", now.Unix()) {
		t.Fatalf("created %+v", cal.created)
	}
}

// The card waited until the event had begun: nothing is created in the past.
func TestAddCreateAfterStart(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{}
	a, send, _ := newTestApp(t, cal, nil)

	a.HandleText(ctx, at(t, "2026-09-29 10:00"), tg.Message{ChatID: 42, Text: "/add 30.09 9:00-10:00 Приём у врача"})
	press(a, send, at(t, "2026-09-30 09:30"), 1, "evt:1:create")

	if ed := lastEdit(t, send); ed.text != "Это время уже прошло: ср, 30 сен, 09:00" || ed.keyboard != nil {
		t.Fatalf("late create = %q %v", ed.text, ed.keyboard)
	}
	if len(cal.created) != 0 {
		t.Fatalf("created %v in the past", cal.created)
	}
}
