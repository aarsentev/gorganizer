package app

import (
	"context"
	"testing"
	"time"

	"go-organizer/internal/tg"
)

func TestResolveZone(t *testing.T) {
	cases := map[string]string{
		"toronto":                        "America/Toronto",
		"New York":                       "America/New_York",
		"kuala lumpur":                   "Asia/Kuala_Lumpur",
		"singapore":                      "Asia/Singapore", // not the old top-level alias "Singapore"
		"america/toronto":                "America/Toronto",
		"Europe/Warsaw":                  "Europe/Warsaw",
		"America/Argentina/Buenos_Aires": "America/Argentina/Buenos_Aires",
		"atlantis":                       "",
		"Local":                          "",
		"":                               "",
	}
	for input, want := range cases {
		got, ok := resolveZone(input)
		if got != want || ok != (want != "") {
			t.Errorf("resolveZone(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
}

func TestTZCommand(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)
	now := time.Date(2026, 9, 29, 7, 5, 0, 0, time.UTC)

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/tz atlantis"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/tz toronto"})

	want := []string{
		"Не нашёл зону «atlantis». Пример: /tz toronto или /tz America/Toronto",
		"America/Toronto (GMT-4), сейчас 03:05",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent %q, want %q", send.sent, want)
	}
	if got := a.Loc().String(); got != "America/Toronto" {
		t.Fatalf("Loc = %s", got)
	}
}

// TestFlightWarsawToToronto is the stage 3b acceptance scenario from the spec:
// after /tz the digest and the day review follow the new zone, and nothing repeats.
func TestFlightWarsawToToronto(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	utc := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if _, err := st.CreateTask(ctx, "Call mom", at(t, "2026-09-29 18:00"), at(t, "2026-09-29 06:00")); err != nil {
		t.Fatal(err)
	}

	a.Tick(ctx, utc("2026-09-29 05:30")) // 07:30 in Warsaw: the digest
	a.HandleText(ctx, utc("2026-09-29 17:00"), tg.Message{ChatID: 42, Text: "/tz toronto"})
	a.Tick(ctx, utc("2026-09-29 18:00")) // 14:00 in Toronto, 20:00 in Warsaw: no evening yet, the task is due
	a.Tick(ctx, utc("2026-09-30 00:00")) // 20:00 in Toronto: the evening message
	a.Tick(ctx, utc("2026-09-30 01:00")) // 21:00 in Toronto: the day review
	a.Tick(ctx, utc("2026-09-30 11:29")) // 07:29 in Toronto: nothing yet
	a.Tick(ctx, utc("2026-09-30 11:30")) // 07:30 in Toronto: the next digest

	want := []string{
		"☀️ Сегодня, вт, 29 сен, событий нет\n\nЗадачи:\n☐ Call mom",
		"America/Toronto (GMT-4), сейчас 13:00",
		"⏰ Call mom",
		"📅 На завтра ничего нет, отдыхайте",
		"🌙 Итог дня:\n1. ☐ Call mom",
		"☀️ Сегодня, ср, 30 сен, событий нет\n\nЗадачи:\n☐ Call mom",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
	// The Toronto digest replaces the Toronto evening message, as at home.
	if len(send.deleted) != 1 || send.deleted[0] != 4 {
		t.Fatalf("deleted %v, want the evening message 4", send.deleted)
	}
}
