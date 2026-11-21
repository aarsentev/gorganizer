package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestMoodCheckinDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, lang, status, question string
	}{
		{"English", "en", store.TrackerActive, "How was your mood today?"},
		{"Russian", "ru", store.TrackerActive, "Какое у тебя сегодня настроение?"},
		{"missing", "en", "", ""},
		{"paused tracker", "en", store.TrackerPaused, ""},
		{"archived", "en", store.TrackerArchived, ""},
		{"global pause", "en", store.TrackerActive, ""},
		{"quiet hours", "en", store.TrackerActive, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			a, send, st := newTestApp(t, nil, nil)
			a.cfg.Anchors["eod"] = "22:15"
			if err := a.SetLang(ctx, tc.lang); err != nil {
				t.Fatal(err)
			}
			now := at(t, "2026-11-20 22:15")
			if tc.status != "" {
				id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
				if err != nil {
					t.Fatal(err)
				}
				if err := st.SetTrackerStatus(ctx, id, tc.status); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "global pause" {
				if err := st.SetPausedUntil(ctx, now.Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "quiet hours" {
				a.cfg.QuietHours.From = "22:00"
			}
			a.Tick(ctx, now.Add(-time.Minute))
			if len(send.sent) != 0 {
				t.Fatalf("sent before configured evening time: %q", send.sent)
			}
			a.Tick(ctx, now.UTC()) // Scheduling and the question's date use the user's zone.
			a.Tick(ctx, now.Add(time.Minute))
			if tc.question == "" {
				if len(send.sent) != 0 {
					t.Fatalf("unexpected question: %q", send.sent)
				}
			} else if len(send.sent) != 1 || !strings.Contains(send.sent[0], tc.question) || !strings.Contains(send.sent[0], "2026-11-20") {
				t.Fatalf("expected one dated question with no tasks: %q", send.sent)
			}
		})
	}
}

func TestMoodAnswerSavesOriginalDay(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		t.Run(lang, func(t *testing.T) {
			ctx := context.Background()
			a, send, st := newTestApp(t, nil, nil)
			if err := a.SetLang(ctx, lang); err != nil {
				t.Fatal(err)
			}
			now := at(t, "2026-11-21 21:00")
			id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
			if err != nil {
				t.Fatal(err)
			}
			a.Tick(ctx, now)
			if len(send.keyboards) != 1 || len(send.keyboards[0]) != 1 || len(send.keyboards[0][0]) != 5 {
				t.Fatalf("expected five mood buttons: %v", send.keyboards)
			}
			for i, button := range send.keyboards[0][0] {
				if want := buttonData("mood", id, "2026-11-21:"+string(rune('1'+i))); button.Data != want || len(button.Data) > 64 {
					t.Fatalf("button %d = %+v, want %s", i, button, want)
				}
			}
			if err := a.SetTZ(ctx, "Asia/Tokyo"); err != nil {
				t.Fatal(err)
			}
			// Answer the following day, then replay the same press after a failed edit.
			answerAt := now.Add(24 * time.Hour)
			cb := tg.Callback{ChatID: 42, MessageID: 1, Data: send.keyboards[0][0][3].Data}
			send.failEdits = 1
			if notice := a.HandleCallback(ctx, answerAt, cb); notice != a.label("press_failed", nil) {
				t.Fatalf("failed edit notice = %q", notice)
			}
			if notice := a.HandleCallback(ctx, answerAt.Add(time.Minute), cb); notice != "" {
				t.Fatalf("retry notice = %q", notice)
			}
			entries, err := st.TrackerEntries(ctx, id, "2026-11-21", "2026-11-24")
			if err != nil || len(entries) != 1 || entries[0].Day != "2026-11-21" || entries[0].Value != 4 || !entries[0].Updated.Equal(answerAt) {
				t.Fatalf("answer duplicated, moved or changed on retry: %+v, %v", entries, err)
			}
			want := "Saved: 4/5"
			if lang == "ru" {
				want = "Записано: 4/5"
			}
			e := lastEdit(t, send)
			if e.messageID != 1 || !strings.Contains(e.text, want) || !strings.Contains(e.text, "2026-11-21") || e.keyboard[0][3].Text != "✓ 🙂 4" {
				t.Fatalf("saved selection not shown: %+v", e)
			}
		})
	}
}

func TestMoodAnswerRejectsInvalidButtons(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-11-21 21:00")
	id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateTracker(ctx, "Energy", store.TrackerRating, "", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		buttonData("mood", id, "2026-11-21:0"), buttonData("mood", id, "2026-11-21:6"),
		buttonData("mood", id, "2026-11-21:no"), buttonData("mood", id, "2026-11-21"),
		buttonData("mood", id, "2026-02-30:4"), buttonData("mood", other, "2026-11-21:4"),
		"mood:999:2026-11-21:4",
	} {
		if notice := a.HandleCallback(ctx, now, tg.Callback{ChatID: 42, MessageID: 1, Data: data}); notice == "" {
			t.Errorf("accepted invalid callback %s", data)
		}
	}
	for _, trackerID := range []int64{id, other} {
		entries, err := st.TrackerEntries(ctx, trackerID, "2026-01-01", "2027-01-01")
		if err != nil || len(entries) != 0 {
			t.Fatalf("invalid button wrote entries: %+v, %v", entries, err)
		}
	}
	if len(send.edits) != 0 {
		t.Fatal("invalid button edited a question")
	}
}

func TestMoodCheckinRetryRestartAndTaskReview(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, nil, nil)
	now := at(t, "2026-11-20 21:00")
	id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
	if err != nil {
		t.Fatal(err)
	}
	// The task review fails first; the independent mood delivery still succeeds.
	taskID, err := st.CreateTask(ctx, "Read", now, now)
	if err != nil {
		t.Fatal(err)
	}
	send.failSends = 1
	a.dailyJobs(ctx, now)
	a.dailyJobs(ctx, now.Add(time.Minute))
	if len(send.sent) != 2 || !strings.Contains(send.sent[0], "2026-11-20") || !strings.Contains(send.sent[1], "Read") {
		t.Fatalf("retry duplicated mood or lost task review: %q", send.sent)
	}
	press(a, send, now, 2, buttonData("chk", taskID, "done"))
	if e := lastEdit(t, send); e.messageID != 2 {
		t.Fatalf("task update changed the mood message: %+v", e)
	}
	restarted, err := New(ctx, st, send, nil, a.cfg, discard)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Tick(ctx, now.Add(2*time.Minute))
	if len(send.sent) != 2 {
		t.Fatalf("restart repeated a delivered check-in: %q", send.sent)
	}
	// On the next day, a failed mood send is retried without repeating the old question.
	send.failSends = 1
	tomorrow := now.AddDate(0, 0, 1)
	restarted.Tick(ctx, tomorrow)
	restarted.Tick(ctx, tomorrow.Add(time.Minute))
	restarted.Tick(ctx, tomorrow.Add(2*time.Minute))
	if len(send.sent) != 3 || !strings.Contains(send.sent[2], "2026-11-21") || !strings.Contains(send.sent[0], "2026-11-20") {
		t.Fatalf("daily dates or delivery count wrong: %q", send.sent)
	}
	entries, err := st.TrackerEntries(ctx, id, "2026-11-20", "2026-11-22")
	if err != nil || len(entries) != 0 {
		t.Fatalf("sending a question recorded an answer: %+v, %v", entries, err)
	}
}
