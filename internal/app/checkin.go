package app

import (
	"context"
	"errors"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// The day review is one message for all open tasks: a queue of separate
// notifications at 21:00 quickly stops being read.

type checkinLine struct {
	N     int
	Title string
	State string // open, done, cancelled, moved
	When  whenView
}

// checkin is a daily job at anchors.eod. Tasks whose time is still ahead remind on their own;
// tasks due right now are in the review instead, so their reminder is disarmed.
func (a *App) checkin(ctx context.Context, local time.Time) ([]any, error) {
	tasks, err := a.store.OpenTasksDue(ctx, local)
	if err != nil {
		return nil, err
	}
	details := []any{"tasks", len(tasks)}
	if len(tasks) == 0 {
		return details, nil
	}
	ids := make([]int64, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	text, keyboard, err := a.renderCheckin(ctx, ids, local)
	if err != nil {
		return nil, err
	}
	if _, err := a.send.SendButtons(ctx, a.cfg.ChatID, text, keyboard); err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if err := a.store.MarkTaskReminded(ctx, t.ID, t.Remind); err != nil {
			a.log.Error("checkin", "task_id", t.ID, "err", err)
		}
	}
	return details, nil
}

// onCheckin applies the action and redraws the whole review. The task ids come
// from the message's own buttons, so nothing about the review is stored.
func (a *App) onCheckin(ctx context.Context, now time.Time, cb tg.Callback, t store.Task, action string) error {
	var err error
	switch action {
	case "done":
		err = a.store.SetTaskStatus(ctx, t.ID, store.TaskDone)
	case "delete":
		err = a.store.SetTaskStatus(ctx, t.ID, store.TaskCancelled)
	case "tomorrow":
		err = a.store.RescheduleTask(ctx, t.ID, a.sameTimeTomorrow(t, now))
	case "noop":
		return nil
	default:
		return errors.New("unknown checkin action " + action)
	}
	if err != nil {
		return err
	}

	var ids []int64
	for _, row := range cb.Keyboard {
		if len(row) == 0 {
			continue
		}
		if _, id, _, ok := parseData(row[0].Data); ok {
			ids = append(ids, id)
		}
	}
	text, keyboard, err := a.renderCheckin(ctx, ids, now)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}

// renderCheckin: open tasks get a row of actions, handled ones keep a one-button
// row with their state, which also keeps their id in the message for the next redraw.
func (a *App) renderCheckin(ctx context.Context, ids []int64, now time.Time) (string, [][]tg.Button, error) {
	var (
		lines    []checkinLine
		keyboard [][]tg.Button
	)
	for i, id := range ids {
		t, err := a.store.Task(ctx, id)
		if err != nil {
			return "", nil, err
		}
		line := checkinLine{N: i + 1, Title: t.Title, State: t.Status}
		if t.Status == store.TaskOpen && t.Remind.After(now) {
			line.State, line.When = "moved", a.when(t.Remind, now)
		}
		lines = append(lines, line)

		if line.State == store.TaskOpen {
			keyboard = append(keyboard, []tg.Button{
				{Text: a.label("btn_chk_done", line.N), Data: buttonData("chk", id, "done")},
				{Text: a.label("btn_chk_tomorrow", line.N), Data: buttonData("chk", id, "tomorrow")},
				{Text: a.label("btn_chk_delete", line.N), Data: buttonData("chk", id, "delete")},
			})
		} else {
			keyboard = append(keyboard, []tg.Button{
				{Text: a.label("btn_chk_state", line), Data: buttonData("chk", id, "noop")},
			})
		}
	}
	text, err := a.render("checkin", struct{ Tasks []checkinLine }{lines})
	return text, keyboard, err
}
