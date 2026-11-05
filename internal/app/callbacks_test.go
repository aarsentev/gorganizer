package app

import (
	"context"
	"testing"

	"go-organizer/internal/tg"
)

func TestStaleButtonGetsNotice(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")
	notice := func(messageID int64, data string) string {
		return a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: messageID, Data: data})
	}

	// The second /add replaces the first draft: the first card's buttons refer to nothing.
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00-10:00 first"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 11:00-12:00 second"})

	for _, data := range []string{"evt:1:create", "evt:1:cancel", "card:99:ok", "rem:99:done", "chk:99:done"} {
		if got := notice(1, data); got != "Уже неактуально" {
			t.Errorf("%s: notice = %q", data, got)
		}
	}
	if len(send.edits) != 0 {
		t.Fatalf("a stale button edited a message: %+v", send.edits)
	}

	// A button that works and a press the bot cannot read have nothing to say.
	if got := notice(2, "evt:2:cancel"); got != "" {
		t.Errorf("working button: notice = %q", got)
	}
	if got := notice(2, "zzz:1:ok"); got != "" {
		t.Errorf("unreadable press: notice = %q", got)
	}

	if err := a.SetLang(ctx, "en"); err != nil {
		t.Fatal(err)
	}
	if got := notice(1, "evt:1:create"); got != "No longer relevant" {
		t.Errorf("in English: notice = %q", got)
	}
}
