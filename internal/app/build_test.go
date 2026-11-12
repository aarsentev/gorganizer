package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go-organizer/internal/gcal"
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
	tail := "\nКалендарь не подключён\nОткрыто: 0 задач, 0 заметок"
	want := []string{
		head + "Паузы нет" + tail,
		head + "⏸ Пауза до вт, 27 окт, 14:05" + tail,
		head + "Паузы нет" + tail,
		"Версия dev\nEurope/Warsaw (GMT+2), сейчас 15:05\nПаузы нет" + tail,
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
}

func TestStatusShowsSyncAndOpenItems(t *testing.T) {
	ctx := context.Background()
	down := errors.New("google is down")
	cal := &fakeCalendar{replies: []calendarReply{
		{changes: gcal.Changes{SyncToken: "t1"}},
		{err: down},
		{err: down},
		{changes: gcal.Changes{SyncToken: "t2"}},
	}}
	a, send, st := newTestApp(t, cal, nil)
	a.SetBuild("dev", "unknown")
	if _, err := st.CreateTask(ctx, "Call mom", at(t, "2026-10-25 18:00"), at(t, "2026-10-24 09:00")); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Buy a bulb", "Tyre size"} {
		if _, err := st.CreateNote(ctx, text, at(t, "2026-10-24 09:00")); err != nil {
			t.Fatal(err)
		}
	}
	status := func(now string) string {
		a.HandleText(ctx, at(t, now), tg.Message{ChatID: 42, Text: "/status"})
		lines := strings.Split(send.sent[len(send.sent)-1], "\n")
		return strings.Join(lines[3:], "\n") // after the version, the zone and the pause
	}

	steps := []struct{ sync, now, want string }{
		{"", "15:00", "Календарь ещё не синхронизировался"},
		{"15:03", "15:05", "Календарь ✓ синхронизирован в 15:03"},
		// The failure is dated from the first one in a row, the text is the last one's.
		{"15:08", "15:09", "Календарь ⚠️ не синхронизируется с сб, 24 окт, 15:08: google is down"},
		{"15:13", "15:14", "Календарь ⚠️ не синхронизируется с сб, 24 окт, 15:08: google is down"},
		{"15:18", "15:19", "Календарь ✓ синхронизирован в 15:18"},
	}
	for _, step := range steps {
		if step.sync != "" {
			a.Sync(ctx, at(t, "2026-10-24 "+step.sync))
		}
		want := step.want + "\nОткрыто: 1 задача, 2 заметки"
		if got := status("2026-10-24 " + step.now); got != want {
			t.Errorf("after a sync at %q:\n%s\nwant\n%s", step.sync, got, want)
		}
	}
}
