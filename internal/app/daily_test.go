package app

import (
	"context"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/store"
)

func allDay(id, title string, y int, m time.Month, d int) store.Event {
	start := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return store.Event{ID: id, Title: title, AllDay: true, Start: start, End: start.AddDate(0, 0, 1)}
}

func TestEveningMessageIsReplacedByMorningDigest(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	put(t, st,
		allDay("birthday", "Birthday", 2026, 9, 30),
		store.Event{ID: "seminar", Title: "Seminar", Start: at(t, "2026-09-30 10:00")},
	)

	a.Tick(ctx, at(t, "2026-09-29 07:29"))
	if len(send.sent) != 0 {
		t.Fatalf("sent before the digest time: %q", send.sent)
	}

	// Tuesday morning: nothing today.
	for _, s := range []string{"2026-09-29 07:30", "2026-09-29 12:00", "2026-09-29 19:59"} {
		a.Tick(ctx, at(t, s))
	}
	// Tuesday evening: tomorrow's plan, once.
	for _, s := range []string{"2026-09-29 20:00", "2026-09-29 20:01", "2026-09-29 23:59"} {
		a.Tick(ctx, at(t, s))
	}
	// Wednesday morning: today's digest, then the evening message goes away.
	for _, s := range []string{"2026-09-30 07:00", "2026-09-30 07:30", "2026-09-30 07:31"} {
		a.Tick(ctx, at(t, s))
	}

	want := []string{
		"☀️ Сегодня, вт, 29 сен, событий нет",
		"📅 Завтра, ср, 30 сен:\n• весь день Birthday\n• 10:00 Seminar",
		"☀️ Сегодня, ср, 30 сен — 2 события:\n• весь день Birthday\n• 10:00 Seminar",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
	// Send the digest first, then delete the evening message (id 2).
	if !sameStrings(send.ops, []string{"send:1", "send:2", "send:3", "delete:2"}) {
		t.Fatalf("ops = %v", send.ops)
	}
	if id, _ := st.EveningMessageID(ctx); id != 0 {
		t.Fatalf("evening message id = %d after the digest, want cleared", id)
	}
}

func TestEveningWithNothingTomorrow(t *testing.T) {
	a, send, _ := newTestApp(t, nil, nil)
	if err := a.evening(context.Background(), at(t, "2026-09-29 20:00")); err != nil {
		t.Fatal(err)
	}

	if !sameStrings(send.sent, []string{"📅 На завтра ничего нет, отдыхайте"}) {
		t.Fatalf("sent %q", send.sent)
	}
}

func TestNoMorningDigestInTheEvening(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)

	// The bot was down all day and comes back at 20:30: only the evening message makes sense.
	a.Tick(ctx, at(t, "2026-09-29 20:30"))

	if len(send.sent) != 1 || send.sent[0] != "📅 На завтра ничего нет, отдыхайте" {
		t.Fatalf("sent %q, want only the evening message", send.sent)
	}
}

func TestDailyJobSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	a.Tick(ctx, at(t, "2026-09-29 07:30"))

	// Same database, new process.
	restarted, err := New(ctx, st, send, nil, a.cfg, discard)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Tick(ctx, at(t, "2026-09-29 07:45"))

	if len(send.sent) != 1 {
		t.Fatalf("sent %q, want the digest once", send.sent)
	}
}

func TestFailedDigestIsRetried(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)
	send.failSends = 1

	a.Tick(ctx, at(t, "2026-09-29 07:30"))
	a.Tick(ctx, at(t, "2026-09-29 07:31"))
	a.Tick(ctx, at(t, "2026-09-29 07:32"))

	if len(send.sent) != 1 {
		t.Fatalf("sent %q, want the digest once after one failure", send.sent)
	}
}

func TestStaleEveningMessageIsDeletedByTheNextEvening(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	if err := a.evening(ctx, at(t, "2026-09-29 20:00")); err != nil {
		t.Fatal(err)
	}
	// No morning digest (downtime), next evening comes.
	if err := a.evening(ctx, at(t, "2026-09-30 20:00")); err != nil {
		t.Fatal(err)
	}

	if !sameStrings(send.ops, []string{"send:1", "send:2", "delete:1"}) {
		t.Fatalf("ops = %v", send.ops)
	}
	if id, _ := st.EveningMessageID(ctx); id != 2 {
		t.Fatalf("evening message id = %d, want 2", id)
	}
}

func TestCleanupDeletesYesterdaysReminders(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		ctx := context.Background()
		a, send, st := newTestApp(t, nil, func(c *config.Config) { c.Reminders.Cleanup = cleanup })
		yesterday := at(t, "2026-09-28 09:00")
		if err := st.MarkSent(ctx, "seminar", yesterday.Add(time.Hour), "60m", 555, yesterday); err != nil {
			t.Fatal(err)
		}

		if err := a.digest(ctx, at(t, "2026-09-29 07:30")); err != nil {
			t.Fatal(err)
		}

		var want []int64
		if cleanup {
			want = []int64{555}
		}
		if len(send.deleted) != len(want) || (cleanup && send.deleted[0] != 555) {
			t.Errorf("cleanup=%v: deleted %v, want %v", cleanup, send.deleted, want)
		}
	}
}
