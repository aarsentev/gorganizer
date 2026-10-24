package app

import (
	"context"
	"errors"
	"testing"
)

func TestAnnounceBuild(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{}
	a, send, _ := newTestApp(t, cal, nil)

	// 10:20 UTC is 12:20 in Warsaw, still on summer time on 24 October.
	a.AnnounceBuild(ctx, "a3ae58f", "2026-10-24T10:20Z")
	a.AnnounceBuild(ctx, "a3ae58f", "2026-10-24T10:20Z") // the same image restarted
	cal.checkErr = errors.New("google is down")
	a.AnnounceBuild(ctx, "b4bf69a", "2026-10-25T08:00Z")

	want := []string{
		"Версия a3ae58f, собрана сб, 24 окт, 12:20\nTelegram ✓ · база ✓ (схема 4) · календарь ✓",
		"Версия b4bf69a, собрана вс, 25 окт, 09:00\nTelegram ✓ · база ✓ (схема 4) · календарь ⚠️ google is down",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
}

func TestAnnounceBuildEdges(t *testing.T) {
	ctx := context.Background()

	// Without a calendar there is nothing to check.
	a, send, _ := newTestApp(t, nil, nil)
	a.AnnounceBuild(ctx, "a3ae58f", "2026-10-24T10:20Z")
	if !sameStrings(send.sent, []string{"Версия a3ae58f, собрана сб, 24 окт, 12:20\nTelegram ✓ · база ✓ (схема 4) · календарь не подключён"}) {
		t.Fatalf("without a calendar: %q", send.sent)
	}

	// A message that did not go out is not remembered: the next start tries again.
	b, failing, _ := newTestApp(t, withCalendar(), nil)
	failing.failSends = 1
	b.AnnounceBuild(ctx, "a3ae58f", "2026-10-24T10:20Z")
	b.AnnounceBuild(ctx, "a3ae58f", "2026-10-24T10:20Z")
	if len(failing.sent) != 1 {
		t.Fatalf("sent %q, want one announcement after the failure", failing.sent)
	}

	// A build time in another form still shows the version.
	c, odd, _ := newTestApp(t, withCalendar(), nil)
	c.AnnounceBuild(ctx, "a3ae58f", "yesterday")
	if !sameStrings(odd.sent, []string{"Версия a3ae58f\nTelegram ✓ · база ✓ (схема 4) · календарь ✓"}) {
		t.Fatalf("odd build time: %q", odd.sent)
	}
}
