package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// HandleCallback is called by tg for every button press in the allowed chat.
// Data is "<kind>:<task id>:<action>", kind tells which message the button is under.
func (a *App) HandleCallback(ctx context.Context, now time.Time, cb tg.Callback) {
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) != 3 {
		a.log.Warn("unknown callback", "data", cb.Data)
		return
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		a.log.Warn("unknown callback", "data", cb.Data)
		return
	}
	t, err := a.store.Task(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		a.log.Warn("callback for a missing task", "task_id", id)
		return
	}
	if err != nil {
		a.log.Error("callback", "err", err)
		return
	}

	switch kind, action := parts[0], parts[2]; kind {
	case "card":
		err = a.onCard(ctx, now, cb, t, action)
	case "rem":
		err = a.onReminder(ctx, now, cb, t, action)
	case "chk":
		err = a.onCheckin(ctx, now, cb, t, action)
	default:
		a.log.Warn("unknown callback", "data", cb.Data)
	}
	if err != nil {
		a.log.Error("callback", "data", cb.Data, "err", err)
	}
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
		remind = atClock(midnight(now.In(a.Loc())), a.cfg.Anchors["eod"])
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
