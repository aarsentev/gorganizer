package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTrackerEntryCorrections(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	id := createTracker(t, s, "Reading", TrackerNumber, "pages", now)
	other := createTracker(t, s, "Workout", TrackerBoolean, "", now)
	day := "2026-11-18"
	if err := s.SaveTrackerEntry(ctx, id, day, 10, now); err != nil {
		t.Fatal(err)
	}
	// A second tracker and the next day must remain independent of the correction.
	if err := s.SaveTrackerEntry(ctx, other, day, 1, now); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTrackerEntry(ctx, id, "2026-11-19", 15, now.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	later := now.Add(2 * time.Hour)
	if err := s.SaveTrackerEntry(ctx, id, day, 20, later); err != nil {
		t.Fatal(err)
	}
	// Repeated Telegram delivery or another tap on the same answer is a no-op.
	if err := s.SaveTrackerEntry(ctx, id, day, 20, later.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	entries, err := s.TrackerEntries(ctx, id, "2026-01-01", "2027-01-01")
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries after correction = %+v, %v; want two days", entries, err)
	}
	want := TrackerEntry{TrackerID: id, Day: day, Value: 20, Created: now, Updated: later}
	if entries[0] != want || entries[1].Day != "2026-11-19" || entries[1].Value != 15 {
		t.Fatalf("entries = %+v; want first %+v and unchanged second day", entries, want)
	}
	entry, err := s.TrackerEntry(ctx, other, day)
	if err != nil || entry.Value != 1 {
		t.Fatalf("other tracker changed: %+v, %v", entry, err)
	}
}

func TestTrackerEntryMissingIsNotZero(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	for _, kind := range []string{TrackerBoolean, TrackerNumber} {
		t.Run(kind, func(t *testing.T) {
			id := createTracker(t, s, kind, kind, "", now)
			if _, err := s.TrackerEntry(ctx, id, "2026-11-18"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("unanswered day: %v, want ErrNotFound", err)
			}
			if err := s.SaveTrackerEntry(ctx, id, "2026-11-18", 0, now); err != nil {
				t.Fatal(err)
			}
			entry, err := s.TrackerEntry(ctx, id, "2026-11-18")
			if err != nil || entry.Value != 0 {
				t.Fatalf("explicit zero = %+v, %v", entry, err)
			}
			entries, err := s.TrackerEntries(ctx, id, "2026-01-01", "2027-01-01")
			if err != nil || len(entries) != 1 || entries[0].Day != "2026-11-18" {
				t.Fatalf("grid must include only answered days: %+v, %v", entries, err)
			}
		})
	}
}

func TestTrackerEntryValueValidation(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		kind    string
		valid   []int
		invalid []int
	}{
		{TrackerBoolean, []int{0, 1}, []int{-1, 2}},
		{TrackerNumber, []int{0, 250}, []int{-1}},
		{TrackerRating, []int{1, 5}, []int{-1, 0, 6}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			id := createTracker(t, s, tc.kind, tc.kind, "", now)
			for _, value := range tc.valid {
				if err := s.SaveTrackerEntry(ctx, id, "2026-11-18", value, now); err != nil {
					t.Fatalf("valid value %d: %v", value, err)
				}
			}
			for _, value := range tc.invalid {
				if err := s.SaveTrackerEntry(ctx, id, "2026-11-18", value, now); err == nil {
					t.Fatalf("invalid value %d accepted", value)
				}
			}
			entry, err := s.TrackerEntry(ctx, id, "2026-11-18")
			if err != nil || entry.Value != tc.valid[len(tc.valid)-1] {
				t.Fatalf("invalid answer overwrote saved value: %+v, %v", entry, err)
			}
		})
	}
	if err := s.SaveTrackerEntry(ctx, 999, "2026-11-18", 1, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("save for missing tracker: %v, want ErrNotFound", err)
	}
}

func TestTrackerEntryDatesAndYearBoundaries(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2028, 12, 31, 13, 0, 0, 0, time.UTC)
	id := createTracker(t, s, "Reading", TrackerNumber, "pages", now)
	for _, day := range []string{"2029-01-01", "2028-12-31", "2027-12-31", "2028-02-29", "2028-01-01"} {
		if err := s.SaveTrackerEntry(ctx, id, day, 10, now); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.TrackerEntries(ctx, id, "2028-01-01", "2029-01-01")
	if err != nil {
		t.Fatal(err)
	}
	var days []string
	for _, entry := range entries {
		days = append(days, entry.Day)
	}
	if want := []string{"2028-01-01", "2028-02-29", "2028-12-31"}; !reflect.DeepEqual(days, want) {
		t.Fatalf("year's days = %v, want %v", days, want)
	}
	for _, day := range []string{"", "2026-02-29", "2028-02-30", "2028-13-01", "2028-00-01", "2028-1-01", "2028-01-1", "0000-01-01", "2028-01-01T12:00:00Z"} {
		if err := s.SaveTrackerEntry(ctx, id, day, 1, now); err == nil {
			t.Fatalf("saved invalid date %q", day)
		}
		if _, err := s.TrackerEntry(ctx, id, day); err == nil {
			t.Fatalf("read invalid date %q", day)
		}
	}
	for _, bounds := range [][2]string{
		{"bad", "2029-01-01"}, {"2028-01-01", "bad"},
		{"2028-01-01", "2028-01-01"}, {"2029-01-01", "2028-01-01"},
	} {
		if _, err := s.TrackerEntries(ctx, id, bounds[0], bounds[1]); err == nil {
			t.Fatalf("accepted invalid range %v", bounds)
		}
	}
}

func TestTrackerHistorySurvivesTimezoneChangeAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "trackers.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.SetTZ(ctx, "Asia/Irkutsk"); err != nil {
		t.Fatal(err)
	}
	local := time.FixedZone("Irkutsk", 8*60*60)
	question := time.Date(2026, 12, 31, 21, 0, 0, 0, local)
	id := createTracker(t, s, "Mood", TrackerRating, "", question)
	// A reply after midnight in a new year still answers the preceding evening.
	answer := question.Add(4 * time.Hour)
	day := question.Format(time.DateOnly)
	if err := s.SaveTrackerEntry(ctx, id, day, 4, answer); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTZ(ctx, "America/Los_Angeles"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTrackerStatus(ctx, id, TrackerPaused); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s = reopened
	tracker, err := s.Tracker(ctx, id)
	if err != nil || tracker.Status != TrackerPaused || !tracker.Created.Equal(question) {
		t.Fatalf("tracker after restart = %+v, %v", tracker, err)
	}
	entries, err := s.TrackerEntries(ctx, id, "2026-01-01", "2027-01-01")
	if err != nil || len(entries) != 1 || entries[0].Day != "2026-12-31" || !entries[0].Created.Equal(answer) {
		t.Fatalf("history moved after timezone change/restart: %+v, %v", entries, err)
	}
	if _, err := s.TrackerEntry(ctx, id, "2027-01-01"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("late answer appeared on submission date: %v", err)
	}
	// A correction made later in the new timezone also stays on the chosen day.
	corrected := answer.Add(48 * time.Hour).In(time.FixedZone("Pacific", -8*60*60))
	if err := s.SaveTrackerEntry(ctx, id, day, 5, corrected); err != nil {
		t.Fatal(err)
	}
	entry, err := s.TrackerEntry(ctx, id, day)
	if err != nil || entry.Value != 5 || !entry.Created.Equal(answer) || !entry.Updated.Equal(corrected) {
		t.Fatalf("backdated correction = %+v, %v", entry, err)
	}
}
