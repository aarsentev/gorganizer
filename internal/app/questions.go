package app

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go-organizer/internal/intent"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// Questions about an event draft: what is missing is asked one field at a time, and the
// draft keeps that field as asking until the answer comes (SPEC §10).

// endChoices are the end-time buttons, in minutes after the start.
var endChoices = []int{30, 60, 90, 120}

// question is what a question shows of the draft before asking.
type question struct {
	Title string
	Start time.Time // in the user's zone
}

// draftMessage is what an event draft shows: the question about its first missing
// field, or the card when nothing is missing.
func (a *App) draftMessage(in intent.Intent, id int64, now time.Time) (string, [][]tg.Button, error) {
	if in.Missing() == intent.AskEnd {
		return a.askEnd(in, id)
	}
	text, err := a.render("event_card", a.eventCard(in, now))
	if err != nil {
		return "", nil, err
	}
	return text, a.eventKeyboard(id), nil
}

// askEnd is "until when?" with end times to choose from. Cancel is there too: without
// it the question could not be closed.
func (a *App) askEnd(in intent.Intent, id int64) (string, [][]tg.Button, error) {
	start := in.Start.In(a.Loc())
	text, err := a.render("ask_end", question{Title: in.Title, Start: start})
	if err != nil {
		return "", nil, err
	}
	var ends []tg.Button
	for _, minutes := range endChoices {
		end := start.Add(time.Duration(minutes) * time.Minute)
		ends = append(ends, tg.Button{Text: a.label("btn_end", end), Data: buttonData("evt", id, "dur"+strconv.Itoa(minutes))})
	}
	cancel := []tg.Button{{Text: a.label("btn_cancel", nil), Data: buttonData("evt", id, "cancel")}}
	return text, [][]tg.Button{ends, cancel}, nil
}

// onEnd takes the end chosen under "until when?" and turns the question into what comes next.
func (a *App) onEnd(ctx context.Context, now time.Time, cb tg.Callback, p store.Pending, in intent.Intent, minutes string) error {
	if p.Asking != intent.AskEnd {
		// A double tap, or the question was already answered: the first answer stands.
		return errStale
	}
	duration, err := strconv.Atoi(minutes)
	if err != nil || duration <= 0 {
		return fmt.Errorf("bad end button %q", minutes)
	}
	in.DurationMin = duration
	if err := a.updateDraft(ctx, p.ID, in, in.Missing(), now); err != nil {
		return err
	}
	text, keyboard, err := a.draftMessage(in, p.ID, now)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}
