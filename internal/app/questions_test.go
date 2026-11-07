package app

import (
	"context"
	"testing"
	"time"

	"go-organizer/internal/intent"
	"go-organizer/internal/tg"
)

func TestAddAsksForEnd(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{}
	a, send, st := newTestApp(t, cal, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00 приём у врача @ клиника"})
	if send.sent[0] != "Приём у врача — ср, 30 сен, 09:00\nДо скольки?" {
		t.Fatalf("question = %q", send.sent[0])
	}
	want := [][]string{
		{"09:30=evt:1:dur30", "10:00=evt:1:dur60", "10:30=evt:1:dur90", "11:00=evt:1:dur120"},
		{"✕ Отмена=evt:1:cancel"},
	}
	if got := dataOf(send.keyboards[0]); !sameRows(got, want) {
		t.Fatalf("question buttons = %v", got)
	}
	if p, err := st.PendingFor(ctx, 42); err != nil || p.Asking != intent.AskEnd {
		t.Fatalf("draft = %+v, %v; want it asking for the end", p, err)
	}

	// The answer turns the question into the card, in the same message.
	press(a, send, now, 1, "evt:1:dur60")
	ed := lastEdit(t, send)
	card := "📅 Приём у врача\nср, 30 сен, 09:00–10:00 (Europe/Warsaw)\n📍 клиника\nНапомню завтра в 08:00"
	if ed.messageID != 1 || ed.text != card {
		t.Fatalf("after the answer = %d %q", ed.messageID, ed.text)
	}
	if !sameRows(dataOf(ed.keyboard), [][]string{{"✓ Создать=evt:1:create", "✕ Отмена=evt:1:cancel"}}) {
		t.Fatalf("card buttons = %v", dataOf(ed.keyboard))
	}
	if p, err := st.PendingFor(ctx, 42); err != nil || p.Asking != "" {
		t.Fatalf("draft = %+v, %v; want nothing asked", p, err)
	}

	// A double tap: the second answer finds the question answered and changes nothing.
	edits := len(send.edits)
	notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: "evt:1:dur120"})
	if notice != "Уже неактуально" || len(send.edits) != edits {
		t.Fatalf("second tap: notice %q, %d edits", notice, len(send.edits)-edits)
	}

	press(a, send, now.Add(time.Minute), 1, "evt:1:create")
	if len(cal.created) != 1 || !cal.created[0].End.Equal(at(t, "2026-09-30 10:00")) {
		t.Fatalf("created %+v, want one event until 10:00", cal.created)
	}
}

func TestEndQuestionCancels(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00 Приём"})
	press(a, send, now, 1, "evt:1:cancel")

	if ed := lastEdit(t, send); ed.text != "✕ Приём — отменено" || ed.keyboard != nil {
		t.Fatalf("after cancel = %q %v", ed.text, ed.keyboard)
	}
	if _, err := st.PendingFor(ctx, 42); err == nil {
		t.Fatal("the draft outlived its cancel")
	}
}
