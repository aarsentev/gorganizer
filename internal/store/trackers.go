package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TrackerBoolean = "boolean" // 0 = no, 1 = yes
	TrackerNumber  = "number"  // nonnegative whole numbers, e.g. pages read
	TrackerRating  = "rating"  // 1 through 5

	TrackerActive   = "active"
	TrackerPaused   = "paused"
	TrackerArchived = "archived"
)

// Tracker defines what a daily answer means. Status controls whether to ask;
// pausing or archiving never removes answers or prevents historical corrections.
type Tracker struct {
	ID      int64
	Name    string
	Preset  string // stable preset key, empty for an ordinary tracker
	Kind    string
	Unit    string
	Status  string
	Created time.Time
}

func (s *Store) CreateTracker(ctx context.Context, name, kind, unit string, now time.Time) (int64, error) {
	name = strings.TrimSpace(name)
	if err := validateTracker(name, kind); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO trackers (name, kind, unit, created_utc) VALUES (?, ?, ?, ?)",
		name, kind, strings.TrimSpace(unit), now.Unix())
	if err != nil {
		return 0, fmt.Errorf("create tracker: %w", err)
	}
	return res.LastInsertId()
}

// CreatePresetTracker returns the existing id on a repeated enable, preserving
// its status and history. Resuming a paused/archived tracker is a separate action.
func (s *Store) CreatePresetTracker(ctx context.Context, preset, name, kind, unit string, now time.Time) (int64, error) {
	preset, name = strings.TrimSpace(preset), strings.TrimSpace(name)
	if preset == "" {
		return 0, fmt.Errorf("create tracker: preset is required")
	}
	if err := validateTracker(name, kind); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO trackers (preset, name, kind, unit, created_utc) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(preset) DO NOTHING RETURNING id`,
		preset, name, kind, strings.TrimSpace(unit), now.Unix()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		t, err := s.TrackerForPreset(ctx, preset)
		return t.ID, err
	}
	if err != nil {
		return 0, fmt.Errorf("create preset tracker: %w", err)
	}
	return id, nil
}

func validateTracker(name, kind string) error {
	if name == "" {
		return fmt.Errorf("create tracker: name is required")
	}
	switch kind {
	case TrackerBoolean, TrackerNumber, TrackerRating:
	default:
		return fmt.Errorf("create tracker: unknown kind %q", kind)
	}
	return nil
}

func (s *Store) Tracker(ctx context.Context, id int64) (Tracker, error) {
	trackers, err := s.queryTrackers(ctx, "WHERE id = ?", id)
	if err != nil {
		return Tracker{}, err
	}
	if len(trackers) == 0 {
		return Tracker{}, fmt.Errorf("tracker %d: %w", id, ErrNotFound)
	}
	return trackers[0], nil
}

func (s *Store) TrackerForPreset(ctx context.Context, preset string) (Tracker, error) {
	trackers, err := s.queryTrackers(ctx, "WHERE preset = ?", preset)
	if err != nil {
		return Tracker{}, err
	}
	if len(trackers) == 0 {
		return Tracker{}, fmt.Errorf("tracker preset %q: %w", preset, ErrNotFound)
	}
	return trackers[0], nil
}

// Trackers includes paused and archived trackers so their history stays accessible.
func (s *Store) Trackers(ctx context.Context) ([]Tracker, error) {
	return s.queryTrackers(ctx, "ORDER BY id")
}

func (s *Store) ActiveTrackers(ctx context.Context) ([]Tracker, error) {
	return s.queryTrackers(ctx, "WHERE status = 'active' ORDER BY id")
}

func (s *Store) SetTrackerStatus(ctx context.Context, id int64, status string) error {
	switch status {
	case TrackerActive, TrackerPaused, TrackerArchived:
	default:
		return fmt.Errorf("update tracker %d: unknown status %q", id, status)
	}
	res, err := s.db.ExecContext(ctx, "UPDATE trackers SET status = ? WHERE id = ?", status, id)
	if err != nil {
		return fmt.Errorf("update tracker %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update tracker %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("tracker %d: %w", id, ErrNotFound)
	}
	return nil
}

func (s *Store) queryTrackers(ctx context.Context, where string, args ...any) ([]Tracker, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, COALESCE(preset, ''), kind, unit, status, created_utc FROM trackers "+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query trackers: %w", err)
	}
	defer rows.Close()

	var trackers []Tracker
	for rows.Next() {
		var (
			t       Tracker
			created int64
		)
		if err := rows.Scan(&t.ID, &t.Name, &t.Preset, &t.Kind, &t.Unit, &t.Status, &created); err != nil {
			return nil, fmt.Errorf("scan tracker: %w", err)
		}
		t.Created = time.Unix(created, 0).UTC()
		trackers = append(trackers, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query trackers: %w", err)
	}
	return trackers, nil
}
