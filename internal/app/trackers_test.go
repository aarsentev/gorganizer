package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// Follow buttons actually offered by the UI; replay/stale tests call the handler directly.
func trackerPress(t *testing.T, a *App, send *fakeSender, now time.Time, messageID int64, data string) edit {
	t.Helper()
	keyboard := send.keyboards[messageID-1]
	for i := len(send.edits) - 1; i >= 0; i-- {
		if send.edits[i].messageID == messageID {
			keyboard = send.edits[i].keyboard
			break
		}
	}
	found := false
	for _, row := range keyboard {
		for _, b := range row {
			if len(b.Data) > 64 || b.Text == "" {
				t.Fatalf("invalid Telegram button: %+v", b)
			}
			found = found || b.Data == data
		}
	}
	if !found {
		t.Fatalf("button %q not offered in %v", data, dataOf(keyboard))
	}
	before := len(send.edits)
	if notice := a.HandleCallback(context.Background(), now, tg.Callback{
		ChatID: 42, MessageID: messageID, Data: data, Keyboard: keyboard,
	}); notice != "" {
		t.Fatalf("press %q: %q", data, notice)
	}
	if len(send.edits) != before+1 || lastEdit(t, send).messageID != messageID {
		t.Fatalf("press %q did not update the same message", data)
	}
	return lastEdit(t, send)
}

func TestTrackerSetupAndManagement(t *testing.T) {
	for _, tc := range []struct {
		lang, empty, name, question, scale, active, paused, archived string
		add, enable, pause, resume, archive, back                    string
	}{
		{
			lang: "en", empty: "📊 Your trackers\nNo trackers yet. Add one to get started.",
			name: "Mood", question: "How was your mood today?",
			scale:  "😞 Very low · 🙁 Low · 😐 Okay · 🙂 Good · 😄 Great",
			active: "Active", paused: "Paused", archived: "Archived",
			add: "＋ Add tracker", enable: "Enable", pause: "Pause", resume: "Resume", archive: "Archive", back: "‹ Back",
		},
		{
			lang: "ru", empty: "📊 Твои трекеры\nТрекеров пока нет. Добавь первый.",
			name: "Настроение", question: "Какое у тебя сегодня настроение?",
			scale:  "😞 Очень плохое · 🙁 Плохое · 😐 Нормальное · 🙂 Хорошее · 😄 Отличное",
			active: "Включён", paused: "На паузе", archived: "В архиве",
			add: "＋ Добавить трекер", enable: "Включить", pause: "Пауза", resume: "Возобновить", archive: "В архив", back: "‹ Назад",
		},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			ctx := context.Background()
			a, send, st := newTestApp(t, nil, func(c *config.Config) {
				c.Lang = tc.lang
				c.Anchors["eod"] = "22:15"
			})
			if err := a.SetTZ(ctx, "Asia/Irkutsk"); err != nil {
				t.Fatal(err)
			}
			now := at(t, "2026-11-19 10:00")
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/trackers@organizer"})
			if len(send.sent) != 1 || send.sent[0] != tc.empty ||
				!sameRows(dataOf(send.keyboards[0]), [][]string{{tc.add + "=trk:0:add"}}) {
				t.Fatalf("empty menu = %q, %v", send.sent, send.keyboards)
			}
			click := func(data string) edit { return trackerPress(t, a, send, now, 1, data) }
			e := click("trk:0:add")
			if !sameRows(dataOf(e.keyboard), [][]string{{tc.name + "=trk:0:mood"}, {tc.back + "=trk:0:list"}}) {
				t.Fatalf("presets = %v", dataOf(e.keyboard))
			}
			e = click("trk:0:mood")
			for _, fragment := range []string{tc.question, tc.scale, "22:15 (Asia/Irkutsk)"} {
				if !strings.Contains(e.text, fragment) {
					t.Fatalf("preview %q does not include %q", e.text, fragment)
				}
			}
			if !sameRows(dataOf(e.keyboard), [][]string{{tc.enable + "=trk:0:enable-mood", tc.back + "=trk:0:add"}}) {
				t.Fatalf("preview controls = %v", dataOf(e.keyboard))
			}
			// Browsing/backing out of the preview never opts the user into tracking.
			click("trk:0:add")
			click("trk:0:list")
			if trackers, err := st.Trackers(ctx); err != nil || len(trackers) != 0 {
				t.Fatalf("preview created a tracker: %+v, %v", trackers, err)
			}
			click("trk:0:add")
			click("trk:0:mood")
			e = click("trk:0:enable-mood")
			mood, err := st.TrackerForPreset(ctx, moodPreset)
			if err != nil || mood.Kind != store.TrackerRating || mood.Status != store.TrackerActive {
				t.Fatalf("enabled mood = %+v, %v", mood, err)
			}
			if !strings.HasPrefix(e.text, tc.name+" · "+tc.active) ||
				!sameRows(dataOf(e.keyboard), [][]string{
					{tc.pause + "=" + buttonData("trk", mood.ID, "pause"), tc.archive + "=" + buttonData("trk", mood.ID, "archive")},
					{tc.back + "=trk:0:list"},
				}) {
				t.Fatalf("enabled card = %q %v", e.text, dataOf(e.keyboard))
			}
			click("trk:0:list")
			click(buttonData("trk", mood.ID, "view"))
			if err := st.SaveTrackerEntry(ctx, mood.ID, "2026-11-19", 4, now); err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct{ action, status, label, nextLabel, nextAction string }{
				{"pause", store.TrackerPaused, tc.paused, tc.resume, "resume"},
				{"resume", store.TrackerActive, tc.active, tc.pause, "pause"},
				{"archive", store.TrackerArchived, tc.archived, tc.enable, "restore"},
				{"restore", store.TrackerActive, tc.active, tc.pause, "pause"},
			} {
				e = click(buttonData("trk", mood.ID, step.action))
				got, err := st.Tracker(ctx, mood.ID)
				if err != nil || got.Status != step.status || !strings.HasPrefix(e.text, tc.name+" · "+step.label) {
					t.Fatalf("after %s = %+v, %v, %q", step.action, got, err, e.text)
				}
				if e.keyboard[0][0] != (tg.Button{Text: step.nextLabel, Data: buttonData("trk", mood.ID, step.nextAction)}) {
					t.Fatalf("next action after %s: %v", step.action, e.keyboard)
				}
				e = click("trk:0:list")
				if e.keyboard[0][0].Text != tc.name+" · "+step.label {
					t.Fatalf("list status after %s: %v", step.action, e.keyboard)
				}
				click(buttonData("trk", mood.ID, "view"))
				entry, err := st.TrackerEntry(ctx, mood.ID, "2026-11-19")
				if err != nil || entry.Value != 4 {
					t.Fatalf("history after %s: %+v, %v", step.action, entry, err)
				}
			}
			// Selecting an existing preset opens its controls, not another setup card.
			click("trk:0:list")
			click("trk:0:add")
			e = click("trk:0:mood")
			if !strings.HasPrefix(e.text, tc.name+" · "+tc.active) {
				t.Fatalf("existing preset did not open its controls: %q", e.text)
			}
			if len(send.sent) != 1 {
				t.Fatalf("navigation sent extra messages: %q", send.sent)
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
	if e := lastEdit(t, send); !strings.HasPrefix(e.text, "Mood · Paused") || e.keyboard[0][0].Text != "Resume" {
		t.Fatalf("tracker did not follow language after restart: %+v", e)
	}
}

func TestTrackerStaleAndInvalidCallbacks(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-11-19 10:00")
	id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetTrackerStatus(ctx, id, store.TrackerArchived); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"trk:999:view", "trk:999:pause", buttonData("trk", id, "pause"), buttonData("trk", id, "resume")} {
		if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: data}); notice != "Уже неактуально" {
			t.Fatalf("stale %s = %q", data, notice)
		}
	}
	for _, data := range []string{"trk:0:enable-workout", "trk:0:pause", buttonData("trk", id, "delete")} {
		if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: data}); notice != "Не получилось, попробуй ещё раз" {
			t.Fatalf("invalid %s = %q", data, notice)
		}
	}
	if len(send.edits) != 0 {
		t.Fatalf("invalid/stale buttons edited messages: %+v", send.edits)
	}
	if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: "trk:0:enable-mood"}); notice != "" {
		t.Fatalf("repeated enable: %q", notice)
	}
	tracker, err := st.Tracker(ctx, id)
	if err != nil || tracker.Status != store.TrackerArchived {
		t.Fatalf("old button brought back archived tracker: %+v, %v", tracker, err)
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
	trackerPress(t, a, send, now, messageID, "trk:0:add")
	trackerPress(t, a, send, now, messageID, "trk:0:mood")
	trackerPress(t, a, send, now, messageID, "trk:0:enable-mood")
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
	if len(send.sent) != 1 || send.sent[0] != "Не получилось открыть трекеры. Попробуй /trackers ещё раз." {
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
