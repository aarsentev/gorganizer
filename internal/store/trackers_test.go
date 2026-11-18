package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func createTracker(t *testing.T, s *Store, name, kind, unit string, now time.Time) int64 {
	t.Helper()
	id, err := s.CreateTracker(context.Background(), name, kind, unit, now)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func trackerIDs(trackers []Tracker) []int64 {
	var ids []int64
	for _, tracker := range trackers {
		ids = append(ids, tracker.ID)
	}
	return ids
}

func TestTrackerLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	mood := createTracker(t, s, "Mood", TrackerRating, "", now)
	workout := createTracker(t, s, "Workout", TrackerBoolean, "", now)
	reading := createTracker(t, s, " Reading ", TrackerNumber, " pages ", now)

	got, err := s.Tracker(ctx, reading)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != reading || got.Name != "Reading" || got.Kind != TrackerNumber || got.Unit != "pages" ||
		got.Status != TrackerActive || !got.Created.Equal(now) {
		t.Fatalf("reading = %+v", got)
	}
	if err := s.SaveTrackerEntry(ctx, mood, "2026-11-18", 3, now); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{TrackerPaused, TrackerActive, TrackerArchived} {
		if err := s.SetTrackerStatus(ctx, mood, status); err != nil {
			t.Fatal(err)
		}
		got, err := s.Tracker(ctx, mood)
		if err != nil || got.Status != status {
			t.Fatalf("tracker after %s = %+v, %v", status, got, err)
		}
		want := []int64{workout, reading}
		if status == TrackerActive {
			want = []int64{mood, workout, reading}
		}
		active, err := s.ActiveTrackers(ctx)
		if err != nil || !sameIDs(trackerIDs(active), want) {
			t.Fatalf("active after %s = %v, %v; want %v", status, trackerIDs(active), err, want)
		}
		entry, err := s.TrackerEntry(ctx, mood, "2026-11-18")
		if err != nil || entry.Value != 3 {
			t.Fatalf("history after %s = %+v, %v", status, entry, err)
		}
	}
	all, err := s.Trackers(ctx)
	if err != nil || !sameIDs(trackerIDs(all), []int64{mood, workout, reading}) {
		t.Fatalf("all trackers = %v, %v; archived history must remain accessible", trackerIDs(all), err)
	}
	if err := s.SaveTrackerEntry(ctx, mood, "2026-11-18", 4, now.Add(time.Hour)); err != nil {
		t.Fatalf("correct archived history: %v", err)
	}
	entry, err := s.TrackerEntry(ctx, mood, "2026-11-18")
	if err != nil || entry.Value != 4 {
		t.Fatalf("corrected history = %+v, %v", entry, err)
	}
}

func TestTrackerValidation(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ name, kind string }{
		{"", TrackerRating}, {" \t\n", TrackerBoolean}, {"Mood", "unknown"},
	} {
		if _, err := s.CreateTracker(ctx, tc.name, tc.kind, "", now); err == nil {
			t.Fatalf("CreateTracker(%q, %q) succeeded", tc.name, tc.kind)
		}
	}
	all, err := s.Trackers(ctx)
	if err != nil || len(all) != 0 {
		t.Fatalf("invalid definitions created trackers: %v, %v", all, err)
	}
	id := createTracker(t, s, "Mood", TrackerRating, "", now)
	if err := s.SetTrackerStatus(ctx, id, "deleted"); err == nil {
		t.Fatal("invalid status accepted")
	}
	got, err := s.Tracker(ctx, id)
	if err != nil || got.Status != TrackerActive {
		t.Fatalf("invalid status changed tracker: %+v, %v", got, err)
	}
	if _, err := s.Tracker(ctx, id+1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tracker: %v, want ErrNotFound", err)
	}
	if err := s.SetTrackerStatus(ctx, id+1, TrackerPaused); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing tracker: %v, want ErrNotFound", err)
	}
}
