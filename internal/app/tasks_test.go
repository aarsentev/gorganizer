package app

import (
	"context"
	"testing"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestParseTask(t *testing.T) {
	a, _, _ := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00") // Tuesday

	cases := []struct {
		args, title, remind string
	}{
		{"забрать запчасть вечером", "Забрать запчасть", "2026-09-29 18:00"},
		{"pick up the part evening", "Pick up the part", "2026-09-29 18:00"},
		{"buy bread", "Buy bread", "2026-09-29 21:00"},
		{"call the bank 9:30", "Call the bank", "2026-09-30 09:30"},
		{"call the bank 14:15", "Call the bank", "2026-09-29 14:15"},
		{"позвонить завтра утром", "Позвонить", "2026-09-30 09:00"},
		{"позвонить утром завтра", "Позвонить", "2026-09-30 09:00"},
		{"renew passport tomorrow", "Renew passport", "2026-09-30 21:00"},
		{"pick up tomorrow evening part", "Pick up tomorrow evening part", "2026-09-29 21:00"},
	}
	for _, c := range cases {
		title, remind, err := a.parseTask(c.args, now)
		if err != nil {
			t.Errorf("%q: %v", c.args, err)
			continue
		}
		if title != c.title || !remind.Equal(at(t, c.remind)) {
			t.Errorf("%q = %q at %s, want %q at %s", c.args, title, remind.Format("2006-01-02 15:04"), c.title, c.remind)
		}
	}

	for _, args := range []string{"", "evening", "tomorrow 18:00"} {
		if _, _, err := a.parseTask(args, now); err == nil {
			t.Errorf("%q: want an error for a missing title", args)
		}
	}
}

func dataOf(keyboard [][]tg.Button) [][]string {
	var out [][]string
	for _, row := range keyboard {
		var r []string
		for _, b := range row {
			r = append(r, b.Text+"="+b.Data)
		}
		out = append(out, r)
	}
	return out
}

func sameRows(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameStrings(a[i], b[i]) {
			return false
		}
	}
	return true
}

// skipJobs marks daily jobs as done so a test sees only what it is about.
func skipJobs(t *testing.T, st *store.Store, day string, jobs ...string) {
	t.Helper()
	for _, job := range jobs {
		if _, err := st.TryFire(context.Background(), job, day, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
}

// press simulates a button under the message with the given id.
func press(a *App, send *fakeSender, now time.Time, messageID int64, data string) {
	keyboard := send.keyboards[messageID-1]
	for i := len(send.edits) - 1; i >= 0; i-- {
		if send.edits[i].messageID == messageID {
			keyboard = send.edits[i].keyboard
			break
		}
	}
	a.HandleCallback(context.Background(), now, tg.Callback{ChatID: 42, MessageID: messageID, Data: data, Keyboard: keyboard})
}

func lastEdit(t *testing.T, send *fakeSender) edit {
	t.Helper()
	if len(send.edits) == 0 {
		t.Fatal("no edits")
	}
	return send.edits[len(send.edits)-1]
}

// TestSparePartScenario is the stage 3 acceptance scenario from the spec.
func TestSparePartScenario(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	skipJobs(t, st, "2026-09-29", jobDigest, jobEvening)
	skipJobs(t, st, "2026-09-30", jobDigest, jobEvening)
	tick := func(s string) { a.Tick(ctx, at(t, s)) }

	// 10:00 — the task and its card.
	a.HandleText(ctx, at(t, "2026-09-29 10:00"), 42, "/task забрать запчасть вечером")
	if send.sent[0] != "☐ Забрать запчасть\nНапомню сегодня в 18:00, итог дня в 21:00" {
		t.Fatalf("card = %q", send.sent[0])
	}
	if !sameRows(dataOf(send.keyboards[0]), [][]string{{"✓ Ок=card:1:ok", "✎ Время=card:1:time", "✕ Отмена=card:1:cancel"}}) {
		t.Fatalf("card buttons = %v", dataOf(send.keyboards[0]))
	}
	press(a, send, at(t, "2026-09-29 10:01"), 1, "card:1:ok")
	if e := lastEdit(t, send); e.text != send.sent[0] || e.keyboard != nil {
		t.Fatalf("after OK = %+v, want the same text without buttons", e)
	}

	// 18:00 — the reminder, offering the end of the day while it is ahead.
	tick("2026-09-29 17:59")
	tick("2026-09-29 18:00")
	tick("2026-09-29 18:01")
	if len(send.sent) != 2 || send.sent[1] != "⏰ Забрать запчасть" {
		t.Fatalf("sent %q, want one reminder at 18:00", send.sent)
	}
	if !sameRows(dataOf(send.keyboards[1]), [][]string{{"Сделано=rem:1:done", "Через час=rem:1:hour", "В 21:00=rem:1:eod"}}) {
		t.Fatalf("reminder buttons = %v", dataOf(send.keyboards[1]))
	}

	// "In an hour" — and the reminder comes back at 19:05.
	press(a, send, at(t, "2026-09-29 18:05"), 2, "rem:1:hour")
	if e := lastEdit(t, send); e.text != "⏰ Забрать запчасть → сегодня в 19:05" || e.keyboard != nil {
		t.Fatalf("after 'in an hour' = %+v", e)
	}
	tick("2026-09-29 19:04")
	tick("2026-09-29 19:05")
	if len(send.sent) != 3 || send.sent[2] != "⏰ Забрать запчасть" {
		t.Fatalf("sent %q, want the reminder again at 19:05", send.sent)
	}

	// 21:00 — the day review.
	tick("2026-09-29 21:00")
	tick("2026-09-29 21:01")
	if len(send.sent) != 4 || send.sent[3] != "🌙 Итог дня:\n1. ☐ Забрать запчасть" {
		t.Fatalf("sent %q, want one day review", send.sent)
	}
	if !sameRows(dataOf(send.keyboards[3]), [][]string{{"1 ✓=chk:1:done", "1 → завтра=chk:1:tomorrow", "1 ✕=chk:1:delete"}}) {
		t.Fatalf("review buttons = %v", dataOf(send.keyboards[3]))
	}

	// "Tomorrow" from the review keeps the time: 19:05.
	press(a, send, at(t, "2026-09-29 21:02"), 4, "chk:1:tomorrow")
	e := lastEdit(t, send)
	if e.text != "🌙 Итог дня:\n1. → Забрать запчасть (завтра в 19:05)" ||
		!sameRows(dataOf(e.keyboard), [][]string{{"1 →=chk:1:noop"}}) {
		t.Fatalf("review after 'tomorrow' = %q %v", e.text, dataOf(e.keyboard))
	}

	// Next day: the reminder at 19:05, "Done", and no review at 21:00.
	tick("2026-09-30 19:04")
	tick("2026-09-30 19:05")
	if len(send.sent) != 5 || send.sent[4] != "⏰ Забрать запчасть" {
		t.Fatalf("sent %q, want tomorrow's reminder", send.sent)
	}
	press(a, send, at(t, "2026-09-30 19:10"), 5, "rem:1:done")
	if e := lastEdit(t, send); e.text != "☑ Забрать запчасть" || e.keyboard != nil {
		t.Fatalf("after done = %+v", e)
	}
	tick("2026-09-30 21:00")
	if len(send.sent) != 5 {
		t.Fatalf("sent %q, a task closed during the day must not reach the review", send.sent[5:])
	}
}

func TestCardTimeAndCancel(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, 42, "/task call the bank evening")
	press(a, send, now, 1, "card:1:time")
	if e := lastEdit(t, send); !sameRows(dataOf(e.keyboard), [][]string{
		{"Утром=card:1:at-morning", "Днём=card:1:at-afternoon"},
		{"Вечером=card:1:at-evening", "В конце дня=card:1:at-eod"},
		{"На завтра=card:1:tomorrow"},
	}) {
		t.Fatalf("time choices = %v", dataOf(e.keyboard))
	}

	// Morning has passed today, so it means tomorrow morning.
	press(a, send, now, 1, "card:1:at-morning")
	if e := lastEdit(t, send); e.text != "☐ Call the bank\nНапомню завтра в 09:00, итог дня в 21:00" || len(e.keyboard) != 1 {
		t.Fatalf("after choosing morning = %+v", e)
	}
	if task, _ := st.Task(ctx, 1); !task.Remind.Equal(at(t, "2026-09-30 09:00")) {
		t.Fatalf("remind = %s", task.Remind.In(warsaw))
	}

	press(a, send, now, 1, "card:1:cancel")
	if e := lastEdit(t, send); e.text != "✕ Call the bank — отменено" || e.keyboard != nil {
		t.Fatalf("after cancel = %+v", e)
	}
	a.taskReminders(ctx, at(t, "2026-09-30 09:00"))
	if len(send.sent) != 1 {
		t.Fatalf("a cancelled task was reminded: %q", send.sent)
	}
}

func TestLateReminderOffersTomorrow(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	if _, err := st.CreateTask(ctx, "Water the plants", at(t, "2026-09-29 22:00"), at(t, "2026-09-29 10:00")); err != nil {
		t.Fatal(err)
	}

	a.taskReminders(ctx, at(t, "2026-09-29 22:00"))

	if !sameRows(dataOf(send.keyboards[0]), [][]string{{"Сделано=rem:1:done", "Через час=rem:1:hour", "На завтра=rem:1:tomorrow"}}) {
		t.Fatalf("buttons after 21:00 = %v", dataOf(send.keyboards[0]))
	}
}

func TestTaskUsageAndBadCallbacks(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)

	a.HandleText(ctx, at(t, "2026-09-29 10:00"), 42, "/task@organizer_bot evening")
	if len(send.sent) != 1 || send.sent[0][:len("Пример")] != "Пример" {
		t.Fatalf("sent %q, want the usage hint", send.sent)
	}

	// Garbage and presses for tasks that do not exist are ignored, not crashes.
	for _, data := range []string{"", "card", "card:x:ok", "card:99:ok", "zzz:1:ok"} {
		a.HandleCallback(ctx, at(t, "2026-09-29 10:00"), tg.Callback{ChatID: 42, MessageID: 1, Data: data})
	}
	if len(send.edits) != 0 {
		t.Fatalf("edits = %+v", send.edits)
	}
}

func TestSplitCommand(t *testing.T) {
	cases := map[string][2]string{
		"/task buy milk":         {"/task", "buy milk"},
		"/TASK@my_bot  buy milk": {"/task", "buy milk"},
		"/tz":                    {"/tz", ""},
		"just text":              {"", "just text"},
	}
	for in, want := range cases {
		cmd, args := splitCommand(in)
		if cmd != want[0] || args != want[1] {
			t.Errorf("splitCommand(%q) = %q, %q; want %q, %q", in, cmd, args, want[0], want[1])
		}
	}
}
