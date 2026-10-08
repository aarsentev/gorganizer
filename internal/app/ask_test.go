package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestPlainTextBecomesTask(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, ID: 10, Text: "Купить пива завтра 19:00"})
	if send.sent[0] != "Задача или заметка?" || send.replyTo[0] != 10 ||
		!sameRows(dataOf(send.keyboards[0]), [][]string{{"☐ Задача=ask:10:task", "📥 Заметка=ask:10:note"}}) {
		t.Fatalf("question = %q, reply to %d, buttons %v", send.sent[0], send.replyTo[0], dataOf(send.keyboards[0]))
	}

	// The question turns into the task card; the tail of the text sets the time, as in /task.
	a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: "ask:10:task", ReplyText: "Купить пива завтра 19:00"})
	e := lastEdit(t, send)
	if e.messageID != 1 || e.text != "☐ Купить пива\nНапомню завтра в 19:00, итог дня в 21:00" ||
		!sameRows(dataOf(e.keyboard), [][]string{{"✓ Ок=card:1:ok", "✎ Время=card:1:time", "✕ Отмена=card:1:cancel"}}) {
		t.Fatalf("after 'task' = %d %q %v", e.messageID, e.text, dataOf(e.keyboard))
	}
	if task, err := st.Task(ctx, 1); err != nil || !task.Remind.Equal(at(t, "2026-09-30 19:00")) {
		t.Fatalf("task = %+v, %v", task, err)
	}
}

func TestPlainTextBecomesNote(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, ID: 11, Text: "размер шин 205/55 R16"})
	a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: "ask:11:note", ReplyText: "размер шин 205/55 R16"})

	if e := lastEdit(t, send); e.text != "📥 Записал в ящик" || e.keyboard != nil {
		t.Fatalf("after 'note' = %q %v", e.text, e.keyboard)
	}
	ids, _ := st.OpenNoteIDs(ctx)
	if len(ids) != 1 {
		t.Fatalf("open notes = %v, want one", ids)
	}
	if n, _ := st.Note(ctx, ids[0]); n.Text != "размер шин 205/55 R16" {
		t.Fatalf("note = %q", n.Text)
	}
}

func TestAskEdgeCases(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	// The user deleted the message before answering: nothing to save.
	a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: "ask:12:task", ReplyText: ""})
	if e := lastEdit(t, send); e.text != "Не вижу исходного сообщения — отправь текст ещё раз" {
		t.Fatalf("lost message = %q", e.text)
	}

	// Only a time, no title: the usage instead of a card.
	a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 2, Data: "ask:13:task", ReplyText: "18:00"})
	if e := lastEdit(t, send); !strings.HasPrefix(e.text, "Пример: /task") {
		t.Fatalf("time only = %q", e.text)
	}

	if _, err := st.Task(ctx, 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a task was created: %v", err)
	}
	if ids, _ := st.OpenNoteIDs(ctx); len(ids) != 0 {
		t.Fatalf("notes were created: %v", ids)
	}
}

func TestNoteCommand(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/note"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/note идея: экспорт в CSV"})

	if !sameStrings(send.sent, []string{"Пример: /note размер шин 205/55 R16", "📥 Записал в ящик"}) {
		t.Fatalf("sent %q", send.sent)
	}
	if ids, _ := st.OpenNoteIDs(ctx); len(ids) != 1 {
		t.Fatalf("open notes = %v, want one", ids)
	}
}
