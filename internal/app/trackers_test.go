package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestMoodSettings(t *testing.T) {
	for _, tc := range []struct{ lang, title, question, enable, pause, active, paused string }{
		{"en", "Mood · Not enabled", "How was your mood today?", "Enable", "Pause", "Mood · Active", "Mood · Paused"},
		{"ru", "Настроение · Выключено", "Какое у тебя сегодня настроение?", "Включить", "Пауза", "Настроение · Включено", "Настроение · На паузе"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			ctx := context.Background()
			a, send, st := newTestApp(t, nil, nil)
			a.cfg.Anchors["eod"] = "22:15"
			if err := a.SetLang(ctx, tc.lang); err != nil {
				t.Fatal(err)
			}
			now := at(t, "2026-11-22 10:00")
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/trackers@organizer"})
			if len(send.sent) != 1 || !strings.HasPrefix(send.sent[0], tc.title) ||
				!strings.Contains(send.sent[0], tc.question) || !strings.Contains(send.sent[0], "22:15 (Europe/Warsaw)") ||
				!sameRows(dataOf(send.keyboards[0]), [][]string{{tc.enable + "=trk:0:enable-mood"}}) {
				t.Fatalf("direct settings = %q %v", send.sent, send.keyboards)
			}
			if _, err := st.TrackerForPreset(ctx, moodPreset); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("opening settings enabled mood: %v", err)
			}
			keyboard := send.keyboards[0]
			for _, step := range []struct{ status, title, next string }{
				{store.TrackerActive, tc.active, tc.pause},
				{store.TrackerPaused, tc.paused, tc.enable},
				{store.TrackerActive, tc.active, tc.pause},
			} {
				if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: keyboard[0][0].Data}); notice != "" {
					t.Fatal(notice)
				}
				got, err := st.TrackerForPreset(ctx, moodPreset)
				if err != nil || got.Status != step.status {
					t.Fatalf("mood state = %+v, %v", got, err)
				}
				e := lastEdit(t, send)
				keyboard = e.keyboard
				if e.messageID != 1 || !strings.HasPrefix(e.text, step.title) || len(keyboard) != 1 || len(keyboard[0]) != 1 || keyboard[0][0].Text != step.next {
					t.Fatalf("settings must have one control: %+v", e)
				}
			}
			if len(send.sent) != 1 {
				t.Fatalf("controls sent extra messages: %q", send.sent)
			}
		})
	}
}

func TestTrackerEnableRetryAndRestart(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-11-19 10:00")
	cb := tg.Callback{ChatID: 42, MessageID: 1, Data: "trk:0:enable-mood"}
	send.failEdits = 1
	if notice := a.HandleCallback(ctx, now, cb); notice != "Не получилось, попробуй ещё раз" {
		t.Fatalf("failed Telegram edit notice = %q", notice)
	}
	for range 2 {
		if notice := a.HandleCallback(ctx, now.Add(time.Minute), cb); notice != "" {
			t.Fatalf("retry notice = %q", notice)
		}
	}
	trackers, err := st.Trackers(ctx)
	if err != nil || len(trackers) != 1 || !trackers[0].Created.Equal(now) {
		t.Fatalf("enable retries duplicated or replaced tracker: %+v, %v", trackers, err)
	}
	id := trackers[0].ID
	if err := st.SetTrackerStatus(ctx, id, store.TrackerPaused); err != nil {
		t.Fatal(err)
	}
	if err := a.SetLang(ctx, "en"); err != nil {
		t.Fatal(err)
	}
	// Restart and retry an older Enable button after the user has paused the tracker.
	a, err = New(ctx, st, send, nil, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if notice := a.HandleCallback(ctx, now.Add(time.Hour), cb); notice != "" {
		t.Fatalf("retry after restart: %q", notice)
	}
	trackers, err = st.Trackers(ctx)
	if err != nil || len(trackers) != 1 || trackers[0].ID != id || trackers[0].Status != store.TrackerPaused {
		t.Fatalf("old Enable changed a paused tracker: %+v, %v", trackers, err)
	}
	if e := lastEdit(t, send); !strings.HasPrefix(e.text, "Mood · Paused") || e.keyboard[0][0].Text != "Enable" {
		t.Fatalf("tracker did not follow language after restart: %+v", e)
	}
}

func TestPreviouslyArchivedMoodCanBeEnabled(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-11-22 10:00")
	id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveTrackerEntry(ctx, id, "2026-11-21", 4, now); err != nil {
		t.Fatal(err)
	}
	if err := st.SetTrackerStatus(ctx, id, store.TrackerArchived); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"trk:0:add", "trk:0:list", "trk:0:mood", "trk:999:enable",
		buttonData("trk", id, "archive"), buttonData("trk", id, "restore"), buttonData("trk", id, "resume"), buttonData("trk", id, "pause")} {
		if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: data}); notice != a.label("stale_button", nil) {
			t.Fatalf("retired/stale button %s: %q", data, notice)
		}
	}
	got, err := st.Tracker(ctx, id)
	if err != nil || got.Status != store.TrackerArchived || len(send.edits) != 0 {
		t.Fatalf("retired buttons changed state: %+v, %v", got, err)
	}
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/trackers"})
	if !strings.HasPrefix(send.sent[0], "Настроение · На паузе") || !sameRows(dataOf(send.keyboards[0]), [][]string{{"Включить=" + buttonData("trk", id, "enable")}}) {
		t.Fatalf("archived mood is not accessible: %q %v", send.sent, send.keyboards)
	}
	press(a, send, now, 1, send.keyboards[0][0][0].Data)
	got, err = st.TrackerForPreset(ctx, moodPreset)
	if err != nil || got.ID != id || got.Status != store.TrackerActive {
		t.Fatalf("enable did not reuse archived mood: %+v, %v", got, err)
	}
	entry, err := st.TrackerEntry(ctx, id, "2026-11-21")
	if err != nil || entry.Value != 4 {
		t.Fatalf("archive history lost: %+v, %v", entry, err)
	}
}

func TestTrackerMenuDoesNotReplaceEventDraft(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-11-19 10:00")
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 20.11 9:00 Doctor"})
	before, err := st.PendingFor(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/trackers"})
	messageID := int64(len(send.sent))
	press(a, send, now, messageID, "trk:0:enable-mood")
	after, err := st.PendingFor(ctx, 42)
	if err != nil || after.ID != before.ID || string(after.IntentRaw) != string(before.IntentRaw) || after.Asking != before.Asking {
		t.Fatalf("tracker setup changed event draft: before %+v, after %+v, %v", before, after, err)
	}
}

func TestTrackerMenuErrors(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-11-19 10:00")
	// A failed menu send produces a useful command retry, without enabling a tracker.
	send.failSends = 1
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/trackers"})
	if len(send.sent) != 1 || send.sent[0] != "Не получилось открыть настройки настроения. Попробуй /trackers ещё раз." {
		t.Fatalf("menu failure = %q", send.sent)
	}
	if _, err := st.TrackerForPreset(ctx, moodPreset); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed menu created tracker: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/trackers"})
	if len(send.sent) != 2 || send.sent[1] != send.sent[0] {
		t.Fatalf("database failure = %q", send.sent)
	}
}
