package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// HandleCallback is called by tg for every button press in the allowed chat.
// Data is "<kind>:<id>:<action>": kind tells which message the button is under,
// id is the task or the note the button refers to.
func (a *App) HandleCallback(ctx context.Context, now time.Time, cb tg.Callback) {
	kind, id, action, ok := parseData(cb.Data)
	if !ok {
		a.log.Warn("unknown callback", "data", cb.Data)
		return
	}
	var err error
	switch kind {
	case "card", "rem", "chk":
		err = a.onTask(ctx, now, cb, kind, id, action)
	case "inb":
		err = a.onInbox(ctx, cb, id, action)
	default:
		a.log.Warn("unknown callback", "data", cb.Data)
		return
	}
	if err != nil {
		a.log.Error("callback", "data", cb.Data, "err", err)
	}
}

// onTask loads the task a button refers to and passes it to the handler of that message.
func (a *App) onTask(ctx context.Context, now time.Time, cb tg.Callback, kind string, id int64, action string) error {
	t, err := a.store.Task(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		a.log.Warn("callback for a missing task", "task_id", id)
		return nil
	}
	if err != nil {
		return err
	}
	switch kind {
	case "card":
		return a.onCard(ctx, now, cb, t, action)
	case "rem":
		return a.onReminder(ctx, now, cb, t, action)
	}
	return a.onCheckin(ctx, now, cb, t, action)
}

// buttonData builds "<kind>:<id>:<action>": Telegram cuts button data at 64 bytes,
// so a button carries only a reference, never the content.
func buttonData(kind string, id int64, action string) string {
	return fmt.Sprintf("%s:%d:%s", kind, id, action)
}

// parseData splits data built by buttonData.
func parseData(data string) (kind string, id int64, action string, ok bool) {
	parts := strings.SplitN(data, ":", 3)
	if len(parts) != 3 {
		return "", 0, "", false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, "", false
	}
	return parts[0], id, parts[2], true
}

// onCard handles the confirmation card under "/task".
func (a *App) onCard(ctx context.Context, now time.Time, cb tg.Callback, t store.Task, action string) error {
	switch {
	case action == "ok":
		return a.editBlock(ctx, cb, "task_card", a.cardView(t, now), nil)
	case action == "cancel":
		if err := a.store.SetTaskStatus(ctx, t.ID, store.TaskCancelled); err != nil {
			return err
		}
		return a.editBlock(ctx, cb, "task_cancelled", t, nil)
	case action == "time":
		return a.editBlock(ctx, cb, "task_card", a.cardView(t, now), a.timeKeyboard(t.ID))
	case action == "tomorrow" || strings.HasPrefix(action, "at-"):
		remind := a.sameTimeTomorrow(t, now)
		if anchor, ok := strings.CutPrefix(action, "at-"); ok {
			clock, known := a.cfg.Anchors[anchor]
			if !known {
				return errors.New("unknown anchor " + anchor)
			}
			remind = a.nextAt(now, clock, false)
		}
		if err := a.store.RescheduleTask(ctx, t.ID, remind); err != nil {
			return err
		}
		t.Remind = remind
		return a.editBlock(ctx, cb, "task_card", a.cardView(t, now), a.cardKeyboard(t.ID))
	}
	return errors.New("unknown card action " + action)
}

// onReminder handles the buttons under "⏰ title".
func (a *App) onReminder(ctx context.Context, now time.Time, cb tg.Callback, t store.Task, action string) error {
	var remind time.Time
	switch action {
	case "done":
		if err := a.store.SetTaskStatus(ctx, t.ID, store.TaskDone); err != nil {
			return err
		}
		return a.editBlock(ctx, cb, "task_done", t, nil)
	case "hour":
		remind = now.Add(time.Hour)
	case "eod":
		// The button may be pressed after the time it offered: then it is tomorrow's.
		remind = a.nextAt(now, a.cfg.Anchors["eod"], false)
	case "tomorrow":
		remind = a.sameTimeTomorrow(t, now)
	default:
		return errors.New("unknown reminder action " + action)
	}
	if err := a.store.RescheduleTask(ctx, t.ID, remind); err != nil {
		return err
	}
	return a.editBlock(ctx, cb, "task_moved", taskView{Title: t.Title, When: a.when(remind, now)}, nil)
}

func (a *App) editBlock(ctx context.Context, cb tg.Callback, block string, data any, keyboard [][]tg.Button) error {
	text, err := a.render(block, data)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}

func (a *App) replyBlock(ctx context.Context, chatID int64, block string, data any) {
	text, err := a.render(block, data)
	if err != nil {
		a.log.Error("render", "err", err)
		return
	}
	a.reply(ctx, chatID, text)
}
