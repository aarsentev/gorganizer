package app

import (
	"context"
	"testing"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestPauseCommand(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 14:05") // Tuesday
	say := func(text string) { a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: text}) }

	say("/pause")
	say("/pause 3d")
	say("/pause")
	say("/pause 12ч") // a new pause replaces the old one
	say("/pause off")
	say("/pause 400d")

	want := []string{
		"▶️ Паузы нет. Поставить: /pause 3d или /pause 12h",
		"⏸ Пауза до пт, 2 окт, 14:05. Снять: /pause off",
		"⏸ Пауза до пт, 2 окт, 14:05. Снять: /pause off",
		"⏸ Пауза до ср, 30 сен, 02:05. Снять: /pause off",
		"▶️ Паузы нет. Поставить: /pause 3d или /pause 12h",
		"Пример: /pause 3d, /pause 12h, /pause off. Не больше года.",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
}

func TestParsePause(t *testing.T) {
	cases := map[string]time.Duration{
		"3d":     72 * time.Hour,
		"12h":    12 * time.Hour,
		"2Д":     48 * time.Hour,
		"5ч":     5 * time.Hour,
		"365d":   365 * 24 * time.Hour,
		"366d":   0, // more than a year
		"99999d": 0, // would overflow
		"0h":     0,
		"3 d":    0,
		"3w":     0,
		"":       0,
	}
	for in, want := range cases {
		got, ok := parsePause(in)
		if got != want || ok != (want != 0) {
			t.Errorf("parsePause(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
}

// While paused the bot says nothing, and when the pause ends nothing piles up:
// only what still matters comes, and missed task reminders go to the day review.
func TestPauseSilencesTheBot(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	put(t, st,
		store.Event{ID: "during", Title: "During", Start: at(t, "2026-09-29 15:00")},
		store.Event{ID: "after", Title: "After", Start: at(t, "2026-09-30 11:00")},
	)
	if _, err := st.CreateTask(ctx, "Call mom", at(t, "2026-09-29 18:00"), at(t, "2026-09-29 09:00")); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPausedUntil(ctx, at(t, "2026-09-30 10:00")); err != nil {
		t.Fatal(err)
	}

	for _, s := range []string{
		"2026-09-29 14:00", // reminder time of "During"
		"2026-09-29 18:00", // the task is due
		"2026-09-29 20:00", // evening message time
		"2026-09-29 21:00", // day review time
		"2026-09-30 03:00", // night inside the pause
		"2026-09-30 07:30", // digest time
		"2026-09-30 09:59",
	} {
		a.Tick(ctx, at(t, s))
	}
	if len(send.sent) != 0 {
		t.Fatalf("sent during the pause: %q", send.sent)
	}

	a.Tick(ctx, at(t, "2026-09-30 10:00")) // the pause is over
	a.Tick(ctx, at(t, "2026-09-30 21:00"))

	want := []string{
		"⏰ After через 60 мин — 11:00",
		"☀️ Сегодня, ср, 30 сен — 1 событие:\n• 11:00 After\n\nЗадачи:\n☐ Call mom",
		"📅 На завтра ничего нет, отдыхайте",
		"🌙 Итог дня:\n1. ☐ Call mom",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
}
