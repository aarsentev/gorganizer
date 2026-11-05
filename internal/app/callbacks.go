package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// errStale: what the button refers to is gone. Not a failure: the message stays as it
// is, and the user gets a notice instead of a button that does nothing.
var errStale = errors.New("stale button")

// HandleCallback is called by tg for every button press in the allowed chat.
// Data is "<kind>:<id>:<action>": kind tells which message the button is under,
// id is the task or the note the button refers to. It returns a notice for
// Telegram to show, "" for none.
func (a *App) HandleCallback(ctx context.Context, now time.Time, cb tg.Callback) string {
	kind, id, action, ok := parseData(cb.Data)
	if !ok {
		a.log.Warn("unknown callback", "data", cb.Data)
		return ""
	}
	var err error
	switch kind {
	case "card", "rem", "chk":
		err = a.onTask(ctx, now, cb, kind, id, action)
	case "inb":
		err = a.onInbox(ctx, cb, id, action)
	case "ask":
		err = a.onAsk(ctx, now, cb, action)
	case "evt":
		err = a.onEvent(ctx, now, cb, id, action)
	default:
		a.log.Warn("unknown callback", "data", cb.Data)
		return ""
	}
	if errors.Is(err, errStale) {
		a.log.Info("stale button", "data", cb.Data)
		return a.label("stale_button", nil)
	}
	if err != nil {
		a.log.Error("callback", "data", cb.Data, "err", err)
	}
	return ""
}

// onTask loads the task a button refers to and passes it to the handler of that message.
func (a *App) onTask(ctx context.Context, now time.Time, cb tg.Callback, kind string, id int64, action string) error {
	t, err := a.store.Task(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return errStale
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
