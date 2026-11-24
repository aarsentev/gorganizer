package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/moodgrid"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestGridUsesLocalYearAndSavedMood(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 12, 31, 23, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		zone, lang, status, caption string
		want                        map[string]int
	}{
		{"Europe/Warsaw", "en", store.TrackerActive, "Mood · 2026", map[string]int{"2026-01-01": 5}},
		{"America/Toronto", "ru", store.TrackerPaused, "Настроение · 2025", map[string]int{"2025-12-31": 2}},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			a, send, st := newTestApp(t, nil, func(c *config.Config) { c.TZ, c.Lang = tc.zone, tc.lang })
			id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
			if err != nil {
				t.Fatal(err)
			}
			for day, value := range map[string]int{"2024-12-31": 1, "2025-12-31": 2, "2026-01-01": 5, "2026-01-02": 3} {
				if err := st.SaveTrackerEntry(ctx, id, day, value, now); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.SetTrackerStatus(ctx, id, tc.status); err != nil {
				t.Fatal(err)
			}
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/grid@organizer mood"})
			today := now.In(a.Loc())
			want, err := moodgrid.Render(today.Year(), today, tc.want)
			if err != nil {
				t.Fatal(err)
			}
			if len(send.photos) != 1 || send.photos[0].chatID != 42 || !bytes.Equal(send.photos[0].data, want) ||
				!sameStrings(send.sent, []string{tc.caption}) {
				t.Fatalf("wrong grid or destination: photos=%d, messages=%q", len(send.photos), send.sent)
			}
			// A failed upload produces a retry message and leaves the history intact.
			send.failSends = 1
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/grid mood"})
			if len(send.photos) != 1 || len(send.sent) != 2 || send.sent[1] != a.label("grid_failed", nil) {
				t.Fatalf("upload failure: photos=%d, messages=%q", len(send.photos), send.sent)
			}
			entries, err := st.TrackerEntries(ctx, id, "2024-01-01", "2027-01-01")
			tracker, trackerErr := st.Tracker(ctx, id)
			if err != nil || len(entries) != 4 || trackerErr != nil || tracker.Status != tc.status {
				t.Fatalf("viewing changed history or settings: %v, %+v, %v, %v", entries, tracker, err, trackerErr)
			}
		})
	}
}

func TestGridEmptyUsageAndStoreFailure(t *testing.T) {
	ctx := context.Background()
	now := at(t, "2026-11-24 12:00")
	for _, lang := range []string{"en", "ru"} {
		t.Run(lang, func(t *testing.T) {
			a, send, st := newTestApp(t, nil, func(c *config.Config) { c.Lang = lang })
			for _, command := range []string{"/grid", "/grid workout", "/grid mood 2025"} {
				a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: command})
			}
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/grid mood"})
			if _, err := st.TrackerForPreset(ctx, moodPreset); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("grid created a tracker: %v", err)
			}
			id, err := st.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
			if err != nil {
				t.Fatal(err)
			}
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/grid mood"})
			if err := st.SaveTrackerEntry(ctx, id, "2025-12-31", 4, now); err != nil {
				t.Fatal(err)
			}
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/grid mood"})
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/grid mood"})
			usage, empty := a.label("grid_usage", nil), a.label("grid_empty", 2026)
			want := []string{usage, usage, usage, empty, empty, empty, a.label("grid_failed", nil)}
			if len(send.photos) != 0 || !sameStrings(send.sent, want) {
				t.Fatalf("messages = %q, photos = %d", send.sent, len(send.photos))
			}
		})
	}
}
