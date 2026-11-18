package store

import (
	"context"
	"fmt"
	"time"
)

// TrackerEntry answers a particular calendar day, not the day it was submitted.
// Day is YYYY-MM-DD in the question's local calendar; timezone changes never move it.
// No row means unanswered. A stored zero is an explicit answer for boolean/number trackers.
type TrackerEntry struct {
	TrackerID int64
	Day       string
	Value     int
	Created   time.Time
	Updated   time.Time
}

// SaveTrackerEntry inserts or corrects the day's answer. Repeating the same answer
// leaves the row unchanged, including its timestamps. The caller supplies the day
// explicitly so a late reply or a backdated correction keeps its intended date.
func (s *Store) SaveTrackerEntry(ctx context.Context, trackerID int64, day string, value int, now time.Time) error {
	if err := validTrackerDay(day); err != nil {
		return err
	}
	t, err := s.Tracker(ctx, trackerID)
	if err != nil {
		return err
	}
	valid := false
	switch t.Kind {
	case TrackerBoolean:
		valid = value == 0 || value == 1
	case TrackerNumber:
		valid = value >= 0
	case TrackerRating:
		valid = value >= 1 && value <= 5
	}
	if !valid {
		return fmt.Errorf("tracker %d: value %d is invalid for %s", trackerID, value, t.Kind)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tracker_entries (tracker_id, day, value, created_utc, updated_utc)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(tracker_id, day) DO UPDATE SET value = excluded.value, updated_utc = excluded.updated_utc
		WHERE tracker_entries.value != excluded.value`,
		trackerID, day, value, now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("save tracker %d entry for %s: %w", trackerID, day, err)
	}
	return nil
}

func (s *Store) TrackerEntry(ctx context.Context, trackerID int64, day string) (TrackerEntry, error) {
	if err := validTrackerDay(day); err != nil {
		return TrackerEntry{}, err
	}
	entries, err := s.queryTrackerEntries(ctx, "WHERE tracker_id = ? AND day = ?", trackerID, day)
	if err != nil {
		return TrackerEntry{}, err
	}
	if len(entries) == 0 {
		return TrackerEntry{}, fmt.Errorf("tracker %d entry for %s: %w", trackerID, day, ErrNotFound)
	}
	return entries[0], nil
}

// TrackerEntries returns only recorded answers, ordered by day in [from, until).
// Both bounds are YYYY-MM-DD dates. For a yearly grid use Jan 1 through next Jan 1.
// Missing dates are left out rather than filled with zeroes.
func (s *Store) TrackerEntries(ctx context.Context, trackerID int64, from, until string) ([]TrackerEntry, error) {
	if err := validTrackerDay(from); err != nil {
		return nil, err
	}
	if err := validTrackerDay(until); err != nil {
		return nil, err
	}
	if from >= until {
		return nil, fmt.Errorf("tracker entries: from must be before until")
	}
	return s.queryTrackerEntries(ctx,
		"WHERE tracker_id = ? AND day >= ? AND day < ? ORDER BY day", trackerID, from, until)
}

func validTrackerDay(day string) error {
	d, err := time.Parse(time.DateOnly, day)
	if err != nil || d.Year() < 1 || d.Format(time.DateOnly) != day {
		return fmt.Errorf("tracker entry: invalid date %q, want YYYY-MM-DD", day)
	}
	return nil
}

func (s *Store) queryTrackerEntries(ctx context.Context, where string, args ...any) ([]TrackerEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT tracker_id, day, value, created_utc, updated_utc FROM tracker_entries "+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query tracker entries: %w", err)
	}
	defer rows.Close()

	var entries []TrackerEntry
	for rows.Next() {
		var (
			e                TrackerEntry
			created, updated int64
		)
		if err := rows.Scan(&e.TrackerID, &e.Day, &e.Value, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan tracker entry: %w", err)
		}
		e.Created = time.Unix(created, 0).UTC()
		e.Updated = time.Unix(updated, 0).UTC()
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query tracker entries: %w", err)
	}
	return entries, nil
}
