package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"go-organizer/internal/tg"
)

// Notes are quick captures without a time: any plain text lands in the inbox,
// /inbox shows it page by page and each note can be marked as handled.

const (
	inboxPageSize = 5   // notes per page: their buttons fit one row
	noteMaxRunes  = 200 // a page of notes stays well under Telegram's 4096 characters
)

type noteLine struct {
	N    int
	Text string
	Done bool
}

type inboxView struct {
	Open  int // open notes in total
	Page  int
	Pages int
	Notes []noteLine
}

func (a *App) handleNote(ctx context.Context, now time.Time, chatID int64, text string) {
	if text == "" {
		return
	}
	if _, err := a.store.CreateNote(ctx, text, now); err != nil {
		a.log.Error("create note", "err", err)
		return
	}
	a.replyBlock(ctx, chatID, "note_saved", nil)
}

func (a *App) handleInbox(ctx context.Context, chatID int64) {
	open, err := a.store.OpenNoteIDs(ctx)
	if err != nil {
		a.log.Error("inbox", "err", err)
		return
	}
	text, keyboard, err := a.renderInbox(ctx, pickPage(open, "first", 0), open)
	if err != nil {
		a.log.Error("inbox", "err", err)
		return
	}
	if _, err := a.send.SendButtons(ctx, chatID, text, keyboard); err != nil {
		a.log.Error("send inbox", "err", err)
	}
}

// onInbox handles the buttons under the inbox: a note's button toggles it and redraws
// the same notes, the arrows show another page.
func (a *App) onInbox(ctx context.Context, cb tg.Callback, id int64, action string) error {
	var ids []int64
	switch action {
	case "done", "undo":
		if err := a.store.SetNoteDone(ctx, id, action == "done"); err != nil {
			return err
		}
		ids = pageNoteIDs(cb.Keyboard)
	case "first", "prev", "next", "last":
		// The page is picked below, from the notes open right now.
	default:
		return errors.New("unknown inbox action " + action)
	}
	open, err := a.store.OpenNoteIDs(ctx)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		ids = pickPage(open, action, id)
	}
	text, keyboard, err := a.renderInbox(ctx, ids, open)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}

// pageNoteIDs reads the notes of a page from the message's own buttons,
// so nothing about the page is stored.
func pageNoteIDs(keyboard [][]tg.Button) []int64 {
	var ids []int64
	for _, row := range keyboard {
		for _, b := range row {
			if _, id, action, ok := parseData(b.Data); ok && (action == "done" || action == "undo") {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// pickPage chooses the notes to show; open is sorted. prev and next go from the edge
// note of the current page (anchor), so a note closed meanwhile never makes a page
// skip one. Nothing left in that direction falls back to the first page.
func pickPage(open []int64, action string, anchor int64) []int64 {
	var page []int64
	switch action {
	case "prev":
		i := below(open, anchor)
		page = open[max(0, i-inboxPageSize):i]
	case "next":
		i := below(open, anchor+1)
		page = open[i:min(i+inboxPageSize, len(open))]
	case "last":
		if len(open) > 0 {
			page = open[(len(open)-1)/inboxPageSize*inboxPageSize:]
		}
	}
	if len(page) == 0 {
		page = open[:min(inboxPageSize, len(open))]
	}
	return page
}

// below counts the open notes with an id less than id; open is sorted.
func below(open []int64, id int64) int {
	i, _ := slices.BinarySearch(open, id)
	return i
}

// pages is how many pages n notes take.
func pages(n int) int {
	return (n + inboxPageSize - 1) / inboxPageSize
}

// renderInbox draws the given notes. The count, the page number and the arrows come
// from the notes open right now, so a redraw after a note was closed stays correct.
func (a *App) renderInbox(ctx context.Context, ids, open []int64) (string, [][]tg.Button, error) {
	if len(ids) == 0 {
		text, err := a.render("inbox_empty", nil)
		return text, nil, err
	}

	view := inboxView{Open: len(open)}
	var row []tg.Button
	for i, id := range ids {
		n, err := a.store.Note(ctx, id)
		if err != nil {
			return "", nil, err
		}
		line := noteLine{N: i + 1, Text: shorten(n.Text, noteMaxRunes), Done: n.Done}
		view.Notes = append(view.Notes, line)
		action := "done"
		if n.Done {
			action = "undo"
		}
		row = append(row, tg.Button{Text: a.label("btn_note", line), Data: buttonData("inb", id, action)})
	}
	keyboard := [][]tg.Button{row}

	// Pages are counted around this one: closed notes shift the pages, never the arithmetic.
	first, last := ids[0], ids[len(ids)-1]
	before := pages(below(open, first))
	after := pages(len(open) - below(open, last+1))
	view.Page, view.Pages = before+1, before+1+after

	var nav []tg.Button
	if before > 0 {
		nav = append(nav,
			tg.Button{Text: a.label("btn_first", nil), Data: buttonData("inb", 0, "first")},
			tg.Button{Text: a.label("btn_prev", nil), Data: buttonData("inb", first, "prev")})
	}
	if after > 0 {
		nav = append(nav,
			tg.Button{Text: a.label("btn_next", nil), Data: buttonData("inb", last, "next")},
			tg.Button{Text: a.label("btn_last", nil), Data: buttonData("inb", 0, "last")})
	}
	if len(nav) > 0 {
		keyboard = append(keyboard, nav)
	}

	text, err := a.render("inbox", view)
	return text, keyboard, err
}

// shorten puts a note on one line and cuts it to limit runes, marking the cut with "…".
func shorten(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit-1]) + "…"
}
