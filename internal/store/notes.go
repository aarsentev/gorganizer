package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Note is a quick capture without a time. A handled note leaves the inbox but stays in the table.
type Note struct {
	ID   int64
	Text string
	At   time.Time
	Done bool
}

func (s *Store) CreateNote(ctx context.Context, text string, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO notes (text, at_utc) VALUES (?, ?)", text, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("create note: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) Note(ctx context.Context, id int64) (Note, error) {
	var (
		n  Note
		at int64
	)
	err := s.db.QueryRowContext(ctx, "SELECT id, text, at_utc, done FROM notes WHERE id = ?", id).
		Scan(&n.ID, &n.Text, &at, &n.Done)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, fmt.Errorf("note %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return Note{}, fmt.Errorf("get note %d: %w", id, err)
	}
	n.At = time.Unix(at, 0).UTC()
	return n, nil
}

// OpenNoteIDs returns the ids of notes not handled yet, oldest first.
// The inbox pages through them; ids grow with time, so the order is the order of capture.
func (s *Store) OpenNoteIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM notes WHERE done = 0 ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("open notes: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("open notes: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetNoteDone marks a note as handled, or with done = false brings it back to the inbox.
func (s *Store) SetNoteDone(ctx context.Context, id int64, done bool) error {
	res, err := s.db.ExecContext(ctx, "UPDATE notes SET done = ? WHERE id = ?", done, id)
	if err != nil {
		return fmt.Errorf("update note %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("note %d: %w", id, ErrNotFound)
	}
	return nil
}
