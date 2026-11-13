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
	Start time.Time // in the user's zone, once known
	Day   time.Time // the known date while the time is asked
}

// draftMessage is what an event draft shows: the question about its first missing
// field, or the card when nothing is missing.
func (a *App) draftMessage(in intent.Intent, id int64, now time.Time) (string, [][]tg.Button, error) {
	switch in.Missing() {
	case intent.AskWhen:
		return a.askTyped("ask_when", in, id)
	case intent.AskTime:
		return a.askTyped("ask_time", in, id)
	case intent.AskEnd:
		return a.askEnd(in, id)
	}
	text, err := a.render("event_card", a.eventCard(in, now))
	if err != nil {
		return "", nil, err
	}
	return text, a.eventKeyboard(id), nil
}

// askTyped is a question answered by typing, with only Cancel under it.
func (a *App) askTyped(block string, in intent.Intent, id int64) (string, [][]tg.Button, error) {
	text, err := a.render(block, question{Title: in.Title, Day: a.whenDay(in)})
	if err != nil {
		return "", nil, err
	}
	return text, [][]tg.Button{a.cancelRow(id)}, nil
}

// cancelRow closes the draft: without it a question could not be closed.
func (a *App) cancelRow(id int64) []tg.Button {
	return []tg.Button{{Text: a.label("btn_cancel", nil), Data: buttonData("evt", id, "cancel")}}
}

// whenDay is the local midnight of the draft's known date, zero if there is none.
func (a *App) whenDay(in intent.Intent) time.Time {
	day, err := time.ParseInLocation(time.DateOnly, in.When, a.Loc())
	if err != nil {
		return time.Time{}
	}
	return day
}

// askEnd is "until when?" with end times to choose from.
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
	return text, [][]tg.Button{ends, a.cancelRow(id)}, nil
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
	text, keyboard, err := a.advance(ctx, now, p, in)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}

// answerDraft takes plain text as the answer to the chat's open question when it looks
// like one. Anything else returns false and goes the usual way, so a forgotten question
// never takes a message; it times out instead.
func (a *App) answerDraft(ctx context.Context, now time.Time, chatID int64, text string) bool {
	p, err := a.store.PendingFor(ctx, chatID)
	if errors.Is(err, store.ErrNotFound) {
		return false
	}
	if err != nil {
		a.log.Error("answer draft", "err", err)
		return false
	}
	if p.Asking == "" {
		return false // a card asks nothing
	}
	in, err := decodeDraft(p)
	if err != nil {
		a.log.Error("answer draft", "err", err)
		return false
	}
	in, ok := a.readAnswer(in, p.Asking, text, now)
	if !ok {
		return false
	}
	if in.Start != nil && !in.Start.After(now) {
		// An answer, but not one an event can take: the question stays open for another.
		a.replyBlock(ctx, chatID, "add_past", in.Start.In(a.Loc()))
		return true
	}
	// The question is above the answer now, so what comes next is a new message.
	reply, keyboard, err := a.advance(ctx, now, p, in)
	if err != nil {
		a.log.Error("answer draft", "err", err)
		return true
	}
	if _, err := a.send.SendButtons(ctx, chatID, reply, keyboard); err != nil {
		a.log.Error("send draft", "err", err)
	}
	return true
}

// readAnswer reads text as the answer to the question about asking: in with the answer
// filled in, or false when the text is not an answer. Only forms without a language
// count, the same as in /add: a date, a time, a range.
func (a *App) readAnswer(in intent.Intent, asking, text string, now time.Time) (intent.Intent, bool) {
	text = strings.TrimSpace(text)
	switch asking {
	case intent.AskWhen:
		s, err := parseWhen(text)
		if err != nil {
			return in, false
		}
		in.Start, in.When = a.moment(s, now)
		in.DurationMin = s.duration
		return in, true
	case intent.AskTime:
		s, err := parseWhen(text)
		if err != nil || s.clock == "" {
			return in, false // a date alone does not say the time
		}
		day := a.whenDay(in)
		if s.date != "" {
			day = a.resolveDay(s.date, now) // a new date wins over the known one
		}
		start := atClock(day, s.clock)
		in.Start, in.When, in.DurationMin = &start, "", s.duration
		return in, true
	case intent.AskEnd:
		// An end before the start is on the next day, as in /add 23:00-7:00; equal to it is no answer.
		duration := rangeMinutes(in.Start.In(a.Loc()).Format("15:04"), text)
		if duration == 0 {
			return in, false
		}
		in.DurationMin = duration
		return in, true
	}
	return in, false
}

// advance saves the answered draft and returns what it shows next: the next question or the card.
func (a *App) advance(ctx context.Context, now time.Time, p store.Pending, in intent.Intent) (string, [][]tg.Button, error) {
	if err := a.updateDraft(ctx, p.ID, in, in.Missing(), now); err != nil {
		return "", nil, err
	}
	return a.draftMessage(in, p.ID, now)
}
