package app

import (
	"context"
	"errors"
	"testing"

	"go-organizer/internal/tg"
)

// announce starts the app as the given build.
func announce(a *App, version, built string) {
	a.SetBuild(version, built)
	a.AnnounceBuild(context.Background())
}

func TestAnnounceBuild(t *testing.T) {
	cal := &fakeCalendar{}
	a, send, _ := newTestApp(t, cal, nil)

	// 10:20 UTC is 12:20 in Warsaw, still on summer time on 24 October.
	announce(a, "a3ae58f", "2026-10-24T10:20Z")
	announce(a, "a3ae58f", "2026-10-24T10:20Z") // the same image restarted
	cal.checkErr = errors.New("google is down")
	announce(a, "b4bf69a", "2026-10-25T08:00Z")

	want := []string{
		"Версия a3ae58f, собрана сб, 24 окт, 12:20\nTelegram ✓ · база ✓ (схема 4) · календарь ✓",
		"Версия b4bf69a, собрана вс, 25 окт, 09:00\nTelegram ✓ · база ✓ (схема 4) · календарь ⚠️ google is down",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
}

func TestAnnounceBuildEdges(t *testing.T) {
	// Without a calendar there is nothing to check.
	a, send, _ := newTestApp(t, nil, nil)
	announce(a, "a3ae58f", "2026-10-24T10:20Z")
	if !sameStrings(send.sent, []string{"Версия a3ae58f, собрана сб, 24 окт, 12:20\nTelegram ✓ · база ✓ (схема 4) · календарь не подключён"}) {
		t.Fatalf("without a calendar: %q", send.sent)
	}

	// A message that did not go out is not remembered: the next start tries again.
	b, failing, _ := newTestApp(t, withCalendar(), nil)
	failing.failSends = 1
	announce(b, "a3ae58f", "2026-10-24T10:20Z")
	announce(b, "a3ae58f", "2026-10-24T10:20Z")
	if len(failing.sent) != 1 {
		t.Fatalf("sent %q, want one announcement after the failure", failing.sent)
	}

	// A build time in another form still shows the version.
	c, odd, _ := newTestApp(t, withCalendar(), nil)
	announce(c, "a3ae58f", "yesterday")
	if !sameStrings(odd.sent, []string{"Версия a3ae58f\nTelegram ✓ · база ✓ (схема 4) · календарь ✓"}) {
		t.Fatalf("odd build time: %q", odd.sent)
	}
}

func TestStatus(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	a.SetBuild("a3ae58f", "2026-10-24T10:20Z")
	now := at(t, "2026-10-24 15:05")
	status := func() { a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/status"}) }

	status()
	if err := st.SetPausedUntil(ctx, at(t, "2026-10-27 14:05")); err != nil {
		t.Fatal(err)
	}
	status()
	if err := st.SetPausedUntil(ctx, at(t, "2026-10-24 15:00")); err != nil { // already over
		t.Fatal(err)
	}
	status()
	a.SetBuild("dev", "unknown") // go run
	status()

	head := "Версия a3ae58f, собрана сб, 24 окт, 12:20\nEurope/Warsaw (GMT+2), сейчас 15:05\n"
	want := []string{
		head + "Паузы нет",
		head + "⏸ Пауза до вт, 27 окт, 14:05",
		head + "Паузы нет",
		"Версия dev\nEurope/Warsaw (GMT+2), сейчас 15:05\nПаузы нет",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
}
