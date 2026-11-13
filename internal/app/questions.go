package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	text, keyboard, err := a.takeEnd(ctx, now, p, in, duration)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}

// answerDraft takes plain text as the answer to the chat's open question when it looks
// like one: an end time such as 10:30 for "until when?". Anything else returns false and
// goes the usual way, so a forgotten question never takes a message; it times out instead.
func (a *App) answerDraft(ctx context.Context, now time.Time, chatID int64, text string) bool {
	p, err := a.store.PendingFor(ctx, chatID)
	if errors.Is(err, store.ErrNotFound) {
		return false
	}
	if err != nil {
		a.log.Error("answer draft", "err", err)
		return false
	}
	if p.Asking != intent.AskEnd {
		return false // a card asks nothing
	}
	in, err := decodeDraft(p)
	if err != nil {
		a.log.Error("answer draft", "err", err)
		return false
	}
	// An end before the start is on the next day, as in /add 23:00-7:00; equal to it is no answer.
	duration := rangeMinutes(in.Start.In(a.Loc()).Format("15:04"), strings.TrimSpace(text))
	if duration == 0 {
		return false
	}
	// The question is above the answer now, so what comes next is a new message.
	reply, keyboard, err := a.takeEnd(ctx, now, p, in, duration)
	if err != nil {
		a.log.Error("answer draft", "err", err)
		return true
	}
	if _, err := a.send.SendButtons(ctx, chatID, reply, keyboard); err != nil {
		a.log.Error("send draft", "err", err)
	}
	return true
}

// takeEnd fills the end, by a button or a typed time, and returns what the draft shows next.
func (a *App) takeEnd(ctx context.Context, now time.Time, p store.Pending, in intent.Intent, minutes int) (string, [][]tg.Button, error) {
	in.DurationMin = minutes
	if err := a.updateDraft(ctx, p.ID, in, in.Missing(), now); err != nil {
		return "", nil, err
	}
	return a.draftMessage(in, p.ID, now)
}
