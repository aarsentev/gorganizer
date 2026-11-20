package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-organizer/internal/store"
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
