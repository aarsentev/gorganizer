package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Event is the local copy of a Google Calendar event.
//
// Timed events store absolute instants. All-day events have no instant, only a date:
// Start and End hold UTC midnight of the first day and of the day after the last one
// (Google's exclusive end), so the date never shifts when the user changes zone.
type Event struct {
	ID       string
	Title    string
	Start    time.Time
	End      time.Time // zero if unknown
	AllDay   bool
	Location string
	Deleted  bool
}

// UpsertEvents applies a batch of changes from a sync in one transaction.
func (s *Store) UpsertEvents(ctx context.Context, events []Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("upsert events: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (id, title, start_utc, end_utc, all_day, location, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			title = excluded.title,
			start_utc = excluded.start_utc,
			end_utc = excluded.end_utc,
			all_day = excluded.all_day,
			location = excluded.location,
			deleted = excluded.deleted`)
	if err != nil {
		return fmt.Errorf("upsert events: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		var end sql.NullInt64
		if !e.End.IsZero() {
			end = sql.NullInt64{Int64: e.End.Unix(), Valid: true}
		}
		if _, err := stmt.ExecContext(ctx,
			e.ID, e.Title, e.Start.Unix(), end, e.AllDay, e.Location, e.Deleted); err != nil {
			return fmt.Errorf("upsert event %s: %w", e.ID, err)
		}
	}
	return tx.Commit()
}

// ClearEvents drops the cache before a full resync. Marks in sent are kept on purpose.
func (s *Store) ClearEvents(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM events"); err != nil {
		return fmt.Errorf("clear events: %w", err)
	}
	return nil
}

// TimedEventsBetween returns non-deleted timed events with from <= start < to.
func (s *Store) TimedEventsBetween(ctx context.Context, from, to time.Time) ([]Event, error) {
	return s.queryEvents(ctx, `
		SELECT id, title, start_utc, end_utc, all_day, location, deleted FROM events
		WHERE deleted = 0 AND all_day = 0 AND start_utc >= ? AND start_utc < ?
		ORDER BY start_utc, id`,
		from.Unix(), to.Unix())
}

// AllDayEventsOn returns non-deleted all-day events covering the given calendar date
// (year, month, day are used; the location is ignored).
func (s *Store) AllDayEventsOn(ctx context.Context, date time.Time) ([]Event, error) {
	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).Unix()
	return s.queryEvents(ctx, `
		SELECT id, title, start_utc, end_utc, all_day, location, deleted FROM events
		WHERE deleted = 0 AND all_day = 1 AND start_utc <= ? AND ? < COALESCE(end_utc, start_utc + 86400)
		ORDER BY start_utc, title, id`,
		day, day)
}

func (s *Store) queryEvents(ctx context.Context, query string, args ...any) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var (
			e        Event
			start    int64
			end      sql.NullInt64
			location sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.Title, &start, &end, &e.AllDay, &location, &e.Deleted); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.Start = time.Unix(start, 0).UTC()
		if end.Valid {
			e.End = time.Unix(end.Int64, 0).UTC()
		}
		e.Location = location.String
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	return events, nil
}
