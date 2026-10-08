package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-organizer/internal/tg"
)

// Plain text without a command may be a task or a note, so the bot asks.
// Nothing is stored while the question waits: the question replies to the user's
// message, and a press on its buttons brings that message's text back (tg.Callback.ReplyText).

// askKind asks whether the message is a task or a note.
func (a *App) askKind(ctx context.Context, m tg.Message) {
	text, err := a.render("ask_kind", nil)
	if err != nil {
		a.log.Error("render", "err", err)
		return
	}
	keyboard := [][]tg.Button{{
		{Text: a.label("btn_as_task", nil), Data: buttonData("ask", m.ID, "task")},
		{Text: a.label("btn_as_note", nil), Data: buttonData("ask", m.ID, "note")},
	}}
	if _, err := a.send.Reply(ctx, m.ChatID, m.ID, text, keyboard); err != nil {
		a.log.Error("ask kind", "err", err)
	}
}

// onAsk turns the question into a task card or a saved note, in the same message.
func (a *App) onAsk(ctx context.Context, now time.Time, cb tg.Callback, action string) error {
	text := strings.TrimSpace(cb.ReplyText)
	if text == "" {
		// The user deleted the message before answering: there is nothing to save.
		return a.editBlock(ctx, cb, "ask_lost", nil, nil)
	}
	switch action {
	case "task":
		card, keyboard, err := a.createTask(ctx, now, text)
		if errors.Is(err, errNoTitle) {
			return a.editBlock(ctx, cb, "task_usage", nil, nil)
		}
		if err != nil {
			return err
		}
		return a.send.Edit(ctx, cb.ChatID, cb.MessageID, card, keyboard)
	case "note":
		if _, err := a.store.CreateNote(ctx, text, now); err != nil {
			return err
		}
		return a.editBlock(ctx, cb, "note_saved", nil, nil)
	}
	return errors.New("unknown ask action " + action)
}
