package app

import (
	"context"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/store"
)

// minutes calls f for every minute in [from, to].
func minutes(from, to time.Time, f func(now time.Time)) {
	for now := from; !now.After(to); now = now.Add(time.Minute) {
		f(now)
	}
}

func TestReminderSentExactlyOnce(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	put(t, st, store.Event{ID: "seminar", Title: "Seminar", Location: "Room 5", Start: at(t, "2026-09-29 10:00")})

	minutes(at(t, "2026-09-29 08:30"), at(t, "2026-09-29 08:59"), func(now time.Time) { a.eventReminders(ctx, now) })
	if len(send.sent) != 0 {
		t.Fatalf("sent before the reminder time: %q", send.sent)
	}

	minutes(at(t, "2026-09-29 09:00"), at(t, "2026-09-29 10:05"), func(now time.Time) { a.eventReminders(ctx, now) })
	if len(send.sent) != 1 || send.sent[0] != "⏰ Seminar через 60 мин — 10:00, Room 5" {
		t.Fatalf("sent %q, want exactly one reminder", send.sent)
	}
}

func TestRescheduledEventIsRemindedAgain(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	put(t, st, store.Event{ID: "seminar", Title: "Seminar", Start: at(t, "2026-09-29 10:00")})
	a.eventReminders(ctx, at(t, "2026-09-29 09:00"))

	put(t, st, store.Event{ID: "seminar", Title: "Seminar", Start: at(t, "2026-09-29 14:00")})
	minutes(at(t, "2026-09-29 09:01"), at(t, "2026-09-29 13:30"), func(now time.Time) { a.eventReminders(ctx, now) })

	want := []string{"⏰ Seminar через 60 мин — 10:00", "⏰ Seminar через 60 мин — 14:00"}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent %q, want %q", send.sent, want)
	}
}

func TestLateEventGetsReminderRightAway(t *testing.T) {
	a, send, st := newTestApp(t, nil, nil)
	put(t, st, store.Event{ID: "call", Title: "Call", Start: at(t, "2026-09-29 10:00")})

	a.eventReminders(context.Background(), at(t, "2026-09-29 09:30"))

	if !sameStrings(send.sent, []string{"⏰ Call через 30 мин — 10:00"}) {
		t.Fatalf("sent %q", send.sent)
	}
}

func TestOverdueOffsetsGoOutAsOneMessage(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, func(c *config.Config) { c.Reminders.EventOffsetsMin = []int{60, 15} })
	put(t, st, store.Event{ID: "call", Title: "Call", Start: at(t, "2026-09-29 10:00")})

	// The bot comes back at 09:50: both 60 and 15 minute reminders are overdue.
	minutes(at(t, "2026-09-29 09:50"), at(t, "2026-09-29 09:59"), func(now time.Time) { a.eventReminders(ctx, now) })

	if !sameStrings(send.sent, []string{"⏰ Call через 10 мин — 10:00"}) {
		t.Fatalf("sent %q, want a single reminder", send.sent)
	}
}

func TestNoReminderForStartedOrAllDayEvents(t *testing.T) {
	a, send, st := newTestApp(t, nil, nil)
	put(t, st,
		store.Event{ID: "started", Title: "Started", Start: at(t, "2026-09-29 09:55")},
		store.Event{ID: "birthday", Title: "Birthday", AllDay: true,
			Start: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)},
	)

	a.eventReminders(context.Background(), at(t, "2026-09-29 10:00"))

	if len(send.sent) != 0 {
		t.Fatalf("sent %q, want nothing", send.sent)
	}
}

func TestFailedReminderIsRetried(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	put(t, st, store.Event{ID: "seminar", Title: "Seminar", Start: at(t, "2026-09-29 10:00")})
	send.failSends = 1

	a.eventReminders(ctx, at(t, "2026-09-29 09:00"))
	a.eventReminders(ctx, at(t, "2026-09-29 09:01"))
	a.eventReminders(ctx, at(t, "2026-09-29 09:02"))

	if !sameStrings(send.sent, []string{"⏰ Seminar через 59 мин — 10:00"}) {
		t.Fatalf("sent %q, want one retry after the failure", send.sent)
	}
}

func TestQuietHoursHoldRemindersUntilMorning(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	put(t, st,
		store.Event{ID: "early", Title: "Early", Start: at(t, "2026-09-29 06:45")},
		store.Event{ID: "gym", Title: "Gym", Start: at(t, "2026-09-29 07:30")},
	)

	// Stop before 07:30, when the morning digest would join in.
	minutes(at(t, "2026-09-29 05:00"), at(t, "2026-09-29 06:59"), func(now time.Time) { a.Tick(ctx, now) })
	if len(send.sent) != 0 {
		t.Fatalf("sent during quiet hours: %q", send.sent)
	}

	minutes(at(t, "2026-09-29 07:00"), at(t, "2026-09-29 07:20"), func(now time.Time) { a.Tick(ctx, now) })
	// "Early" started at 06:45, inside quiet hours: its reminder is dropped, not sent late.
	if !sameStrings(send.sent, []string{"⏰ Gym через 30 мин — 07:30"}) {
		t.Fatalf("sent %q", send.sent)
	}
}

func TestQuietHoursOverMidnight(t *testing.T) {
	a, _, _ := newTestApp(t, nil, func(c *config.Config) { c.QuietHours.From, c.QuietHours.To = "23:00", "08:00" })
	cases := map[string]bool{
		"2026-09-29 22:59": false, "2026-09-29 23:00": true, "2026-09-30 03:00": true,
		"2026-09-30 07:59": true, "2026-09-30 08:00": false,
	}
	for s, want := range cases {
		if got := a.quiet(at(t, s)); got != want {
			t.Errorf("quiet(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestEvery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Every(ctx, time.Millisecond, func(context.Context, time.Time) {
		calls++
		if calls == 3 {
			cancel()
		}
	})
	if err != nil || calls != 3 {
		t.Fatalf("calls = %d, err = %v; want 3 calls (first one immediate) and a clean stop", calls, err)
	}
}
