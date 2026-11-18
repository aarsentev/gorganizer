package store

import (
	"context"
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
	Kind    string
	Unit    string
	Status  string
	Created time.Time
}

func (s *Store) CreateTracker(ctx context.Context, name, kind, unit string, now time.Time) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("create tracker: name is required")
	}
	switch kind {
	case TrackerBoolean, TrackerNumber, TrackerRating:
	default:
		return 0, fmt.Errorf("create tracker: unknown kind %q", kind)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO trackers (name, kind, unit, created_utc) VALUES (?, ?, ?, ?)",
		name, kind, strings.TrimSpace(unit), now.Unix())
	if err != nil {
		return 0, fmt.Errorf("create tracker: %w", err)
	}
	return res.LastInsertId()
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
		"SELECT id, name, kind, unit, status, created_utc FROM trackers "+where, args...)
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
		if err := rows.Scan(&t.ID, &t.Name, &t.Kind, &t.Unit, &t.Status, &created); err != nil {
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
