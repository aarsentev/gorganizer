package app

import (
	"context"
	"slices"
	"strings"
	"testing"

	"go-organizer/internal/tg"
)

func TestInboxScenario(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	for _, text := range []string{
		"buy a bulb", "call about the meter", "idea: CSV export", "return the book",
		"book the dentist", "renew the passport", "fix the bike",
	} {
		a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/note " + text})
	}
	if len(send.sent) != 7 || send.sent[6] != "📥 Записал в ящик" {
		t.Fatalf("sent %q, want a confirmation for every note", send.sent)
	}

	// Page 1 of 2: five notes, a button for each, arrows forward only.
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/inbox"})
	if send.sent[7] != "📥 Ящик: 7 · стр. 1/2\n1. ☐ buy a bulb\n2. ☐ call about the meter\n"+
		"3. ☐ idea: CSV export\n4. ☐ return the book\n5. ☐ book the dentist" {
		t.Fatalf("inbox = %q", send.sent[7])
	}
	if !sameRows(dataOf(send.keyboards[7]), [][]string{
		{"1 ✓=inb:1:done", "2 ✓=inb:2:done", "3 ✓=inb:3:done", "4 ✓=inb:4:done", "5 ✓=inb:5:done"},
		{"›=inb:5:next", "»=inb:0:last"},
	}) {
		t.Fatalf("inbox buttons = %v", dataOf(send.keyboards[7]))
	}

	// A closed note stays on the page with ☑ until the page changes.
	press(a, send, now, 8, "inb:2:done")
	e := lastEdit(t, send)
	if e.text != "📥 Ящик: 6 · стр. 1/2\n1. ☐ buy a bulb\n2. ☑ call about the meter\n"+
		"3. ☐ idea: CSV export\n4. ☐ return the book\n5. ☐ book the dentist" {
		t.Fatalf("after closing = %q", e.text)
	}
	if got := dataOf(e.keyboard)[0][1]; got != "2 ☑=inb:2:undo" {
		t.Fatalf("closed note button = %s", got)
	}

	// Page 2 continues right after the last note of page 1, arrows back only.
	press(a, send, now, 8, "inb:5:next")
	e = lastEdit(t, send)
	if e.text != "📥 Ящик: 6 · стр. 2/2\n1. ☐ renew the passport\n2. ☐ fix the bike" ||
		!sameRows(dataOf(e.keyboard), [][]string{
			{"1 ✓=inb:6:done", "2 ✓=inb:7:done"},
			{"«=inb:0:first", "‹=inb:6:prev"},
		}) {
		t.Fatalf("page 2 = %q %v", e.text, dataOf(e.keyboard))
	}

	// Back to the start: the closed note is gone, the next one moves up.
	press(a, send, now, 8, "inb:0:first")
	if e := lastEdit(t, send); e.text != "📥 Ящик: 6 · стр. 1/2\n1. ☐ buy a bulb\n2. ☐ idea: CSV export\n"+
		"3. ☐ return the book\n4. ☐ book the dentist\n5. ☐ renew the passport" {
		t.Fatalf("page 1 again = %q", e.text)
	}
}

func TestInboxUndoAndEmpty(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/inbox"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/note buy a bulb"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/inbox"})
	if send.sent[0] != "📥 Ящик пуст" || send.keyboards[0] != nil {
		t.Fatalf("empty inbox = %q %v", send.sent[0], send.keyboards[0])
	}
	if send.sent[2] != "📥 Ящик: 1\n1. ☐ buy a bulb" {
		t.Fatalf("one page = %q, want no page number", send.sent[2])
	}

	press(a, send, now, 3, "inb:1:done")
	if e := lastEdit(t, send); e.text != "📥 Ящик: 0\n1. ☑ buy a bulb" {
		t.Fatalf("after closing = %q", e.text)
	}
	// Pressed by mistake: the note comes back.
	press(a, send, now, 3, "inb:1:undo")
	e := lastEdit(t, send)
	if e.text != "📥 Ящик: 1\n1. ☐ buy a bulb" || !sameRows(dataOf(e.keyboard), [][]string{{"1 ✓=inb:1:done"}}) {
		t.Fatalf("after undo = %q %v", e.text, dataOf(e.keyboard))
	}
}

func TestPickPage(t *testing.T) {
	// Twelve open notes; the gaps are notes closed earlier.
	open := []int64{1, 2, 3, 5, 8, 9, 10, 11, 12, 14, 15, 16}
	cases := []struct {
		action string
		anchor int64
		want   []int64
	}{
		{"first", 0, []int64{1, 2, 3, 5, 8}},
		{"next", 8, []int64{9, 10, 11, 12, 14}},
		{"next", 14, []int64{15, 16}},
		{"next", 6, []int64{8, 9, 10, 11, 12}}, // the anchor itself was closed meanwhile
		{"next", 16, []int64{1, 2, 3, 5, 8}},   // nothing after: the first page
		{"prev", 15, []int64{9, 10, 11, 12, 14}},
		{"prev", 9, []int64{1, 2, 3, 5, 8}},
		{"prev", 4, []int64{1, 2, 3}},
		{"prev", 1, []int64{1, 2, 3, 5, 8}}, // nothing before: the first page
		{"last", 0, []int64{15, 16}},
	}
	for _, c := range cases {
		if got := pickPage(open, c.action, c.anchor); !slices.Equal(got, c.want) {
			t.Errorf("%s from %d = %v, want %v", c.action, c.anchor, got, c.want)
		}
	}
	if got := pickPage(nil, "last", 0); len(got) != 0 {
		t.Errorf("empty inbox gave %v", got)
	}
}

func TestShorten(t *testing.T) {
	cases := map[string]string{
		"short":                   "short",
		" two\nlines  and\ttabs ": "two lines and tabs",
		strings.Repeat("я", 200):  strings.Repeat("я", 200),
		strings.Repeat("я", 201):  strings.Repeat("я", 199) + "…",
	}
	for in, want := range cases {
		if got := shorten(in, 200); got != want {
			t.Errorf("shorten(%q) = %q, want %q", in, got, want)
		}
	}
}
