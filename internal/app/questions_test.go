package app

import (
	"context"
	"strconv"
	"strings"
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

func TestTypedEndAnswer(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{}
	a, send, st := newTestApp(t, cal, nil)
	now := at(t, "2026-09-29 10:00")
	say := func(text string) {
		a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: text})
	}

	say("/add 30.09 9:00 Приём")
	say("10:30")

	// The question stays above the answer, so the card comes as a new message.
	card := "📅 Приём\nср, 30 сен, 09:00–10:30 (Europe/Warsaw)\nНапомню завтра в 08:00"
	if len(send.sent) != 2 || send.sent[1] != card {
		t.Fatalf("sent %q, want the card after the question", send.sent)
	}
	if !sameRows(dataOf(send.keyboards[1]), [][]string{{"✓ Создать=evt:1:create", "✕ Отмена=evt:1:cancel"}}) {
		t.Fatalf("card buttons = %v", dataOf(send.keyboards[1]))
	}
	if p, err := st.PendingFor(ctx, 42); err != nil || p.Asking != "" {
		t.Fatalf("draft = %+v, %v; want nothing asked", p, err)
	}
	// The buttons of the answered question have nothing left to do.
	if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: "evt:1:dur60"}); notice != "Уже неактуально" {
		t.Fatalf("old question button: notice = %q", notice)
	}

	press(a, send, now.Add(time.Minute), 2, "evt:1:create")
	if len(cal.created) != 1 || !cal.created[0].End.Equal(at(t, "2026-09-30 10:30")) {
		t.Fatalf("created %+v, want one event until 10:30", cal.created)
	}
}

func TestTypedEndOverMidnight(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 23:00 Смена"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "7:00"})

	if !strings.HasPrefix(send.sent[1], "📅 Смена\nср, 30 сен, 23:00–чт, 1 окт, 07:00") {
		t.Fatalf("card = %q", send.sent[1])
	}
}

// Only what looks like an answer is one: a forgotten question never takes a message.
func TestTextThatIsNoAnswer(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")
	say := func(text string) {
		a.HandleText(ctx, now, tg.Message{ChatID: 42, ID: int64(len(send.sent) + 100), Text: text})
	}

	say("/add 30.09 9:00 Приём")
	say("купить хлеб") // not a time
	say("9:00")        // the start itself: no length
	say("/today")      // a command stays a command

	if send.sent[1] != "Задача или заметка?" || send.sent[2] != "Задача или заметка?" ||
		!strings.HasPrefix(send.sent[3], "☀️ Сегодня") {
		t.Fatalf("sent %q", send.sent)
	}
	if p, err := st.PendingFor(ctx, 42); err != nil || p.Asking != intent.AskEnd {
		t.Fatalf("draft = %+v, %v; want the question still open", p, err)
	}

	// Under a card nothing is asked: a time is just text.
	say("/add 30.09 9:00-10:00 Встреча")
	say("10:30")
	if got := send.sent[len(send.sent)-1]; got != "Задача или заметка?" {
		t.Fatalf("text under a card got %q", got)
	}
}

func TestReadAnswer(t *testing.T) {
	a, _, _ := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")
	start := at(t, "2026-09-30 09:00")
	title := intent.Intent{Kind: "event", Title: "Приём"}
	dated := intent.Intent{Kind: "event", Title: "Приём", When: "2026-09-30"}
	timed := intent.Intent{Kind: "event", Title: "Приём", Start: &start}

	// The answer as "start/minutes", "when" for a date alone; "" for no answer.
	show := func(in intent.Intent, ok bool) string {
		switch {
		case !ok:
			return ""
		case in.Start == nil:
			return in.When
		}
		return in.Start.In(warsaw).Format("2006-01-02 15:04") + "/" + strconv.Itoa(in.DurationMin)
	}
	cases := []struct {
		in     intent.Intent
		asking string
		text   string
		want   string
	}{
		{title, intent.AskWhen, "30.09 9:00", "2026-09-30 09:00/0"},
		{title, intent.AskWhen, "30.09", "2026-09-30"},
		{title, intent.AskWhen, "9:00-10:30", "2026-09-30 09:00/90"}, // 9:00 has passed today
		{title, intent.AskWhen, "11:00", "2026-09-29 11:00/0"},
		{title, intent.AskWhen, "купить хлеб", ""},
		{title, intent.AskWhen, "31.02", ""},
		{dated, intent.AskTime, "9:00", "2026-09-30 09:00/0"},
		{dated, intent.AskTime, "9:00-10:30", "2026-09-30 09:00/90"},
		{dated, intent.AskTime, "1.10 9:00", "2026-10-01 09:00/0"}, // a new date wins
		{dated, intent.AskTime, "1.10", ""},                        // a date alone says no time
		{timed, intent.AskEnd, "10:30", "2026-09-30 09:00/90"},
		{timed, intent.AskEnd, "9:00", ""},
		{timed, "location", "клиника", ""}, // nothing reads that question yet
	}
	for _, c := range cases {
		in, ok := a.readAnswer(c.in, c.asking, c.text, now)
		if got := show(in, ok); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.asking, c.text, got, c.want)
		}
	}
}

// TestAddTitleOnly is the doctor scenario from the spec, on /add and without the place:
// every missing field is asked in turn, and the draft stays the same one.
func TestAddTitleOnly(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")
	say := func(text string) {
		a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: text})
	}

	say("/add приём у врача")
	say("30.09")
	say("9:00")
	say("10:00")

	want := []string{
		"Приём у врача\nКогда? Например: 30.09 9:00",
		"Приём у врача — ср, 30 сен\nВо сколько? Например: 9:00 или 9:00-10:00",
		"Приём у врача — ср, 30 сен, 09:00\nДо скольки?",
		"📅 Приём у врача\nср, 30 сен, 09:00–10:00 (Europe/Warsaw)\nНапомню завтра в 08:00",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
	for i, keyboard := range send.keyboards[:2] {
		if !sameRows(dataOf(keyboard), [][]string{{"✕ Отмена=evt:1:cancel"}}) {
			t.Errorf("question %d buttons = %v, want only cancel", i+1, dataOf(keyboard))
		}
	}
}

func TestAnswerInThePast(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 29.09 Приём"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "8:00"})
	if got := send.sent[1]; got != "Это время уже прошло: вт, 29 сен, 08:00" {
		t.Fatalf("answer in the past got %q", got)
	}
	if p, err := st.PendingFor(ctx, 42); err != nil || p.Asking != intent.AskTime {
		t.Fatalf("draft = %+v, %v; want the question still open", p, err)
	}

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "11:00"})
	if got := send.sent[2]; got != "Приём — вт, 29 сен, 11:00\nДо скольки?" {
		t.Fatalf("the next answer got %q", got)
	}
}
