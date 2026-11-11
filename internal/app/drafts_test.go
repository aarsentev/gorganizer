package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-organizer/internal/intent"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestDraftRoundTrip(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newTestApp(t, nil, nil)
	start := at(t, "2026-09-30 09:00")
	in := intent.Intent{Action: "create", Kind: "event", Title: "Приём у врача", Start: &start, DurationMin: 60, Location: "клиника"}

	id, err := a.newDraft(ctx, 42, in, "location", at(t, "2026-09-29 10:00"))
	if err != nil {
		t.Fatal(err)
	}
	p, got, err := a.draft(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Asking != "location" || got.Title != in.Title || !got.Start.Equal(start) ||
		got.DurationMin != 60 || got.Location != "клиника" || got.Kind != "event" {
		t.Fatalf("draft = %+v, %+v", p, got)
	}

	if _, _, err := a.draft(ctx, id+1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing draft err = %v, want ErrNotFound", err)
	}
}

func TestQuestionTimesOut(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")

	// Under a pause, so the ticks below send nothing else: drafts expire before the pause check.
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/pause 1d"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00 Приём"})
	sent := len(send.sent)

	a.Tick(ctx, now.Add(9*time.Minute))
	if _, err := st.PendingFor(ctx, 42); err != nil {
		t.Fatalf("the question closed before its time: %v", err)
	}
	a.Tick(ctx, now.Add(11*time.Minute))
	if _, err := st.PendingFor(ctx, 42); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("draft after the timeout: %v, want it closed", err)
	}
	if len(send.sent) != sent {
		t.Fatalf("the timeout sent %q", send.sent[sent:])
	}
	notice := a.HandleCallback(ctx, now.Add(12*time.Minute), tg.Callback{ChatID: 42, MessageID: 2, Data: "evt:1:dur60"})
	if notice != "Уже неактуально" {
		t.Fatalf("answer after the timeout: notice = %q", notice)
	}

	// A ready card has no question to forget: it waits.
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00-10:00 Приём"})
	a.Tick(ctx, now.Add(time.Hour))
	if p, err := st.PendingFor(ctx, 42); err != nil || p.Asking != "" {
		t.Fatalf("card draft = %+v, %v; want it kept", p, err)
	}
}

func TestCancelCommand(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")
	say := func(text string) {
		a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: text})
	}

	say("/cancel")
	say("/add 30.09 9:00 Приём") // a question
	say("/cancel")
	say("/add 30.09 9:00-10:00 Встреча") // a card
	say("/cancel")

	want := []string{"Нечего отменять", "✕ Приём — отменено", "✕ Встреча — отменено"}
	got := []string{send.sent[0], send.sent[2], send.sent[4]}
	if !sameStrings(got, want) {
		t.Fatalf("replies %q, want %q", got, want)
	}
	if _, err := st.PendingFor(ctx, 42); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("draft after /cancel: %v", err)
	}
	if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 2, Data: "evt:1:dur60"}); notice != "Уже неактуально" {
		t.Fatalf("the cancelled question's button: notice = %q", notice)
	}
}
