package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Pending is the draft of a dialog. The intent is kept as raw JSON: store does not
// know the intent package, app decodes it.
type Pending struct {
	ID        int64
	ChatID    int64
	IntentRaw []byte
	Asking    string // field the waiting question is about, "" while the card is shown
	Updated   time.Time
}

// CreatePending starts a new draft for the chat and drops the previous one, if any.
// The new draft gets a new id, so buttons under the old one stop working.
func (s *Store) CreatePending(ctx context.Context, chatID int64, intentRaw []byte, asking string, now time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("create pending: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM pending WHERE chat_id = ?", chatID); err != nil {
		return 0, fmt.Errorf("create pending: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO pending (chat_id, intent_raw, asking, updated_utc) VALUES (?, ?, NULLIF(?, ''), ?)",
		chatID, string(intentRaw), asking, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("create pending: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create pending: %w", err)
	}
	return id, tx.Commit()
}

// UpdatePending stores the draft after an answer; the id stays, the timeout restarts.
func (s *Store) UpdatePending(ctx context.Context, id int64, intentRaw []byte, asking string, now time.Time) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE pending SET intent_raw = ?, asking = NULLIF(?, ''), updated_utc = ? WHERE id = ?",
		string(intentRaw), asking, now.Unix(), id)
	if err != nil {
		return fmt.Errorf("update pending %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("pending %d: %w", id, ErrNotFound)
	}
	return nil
}

// Pending returns the draft a button refers to.
func (s *Store) Pending(ctx context.Context, id int64) (Pending, error) {
	return s.queryPending(ctx, "WHERE id = ?", id)
}

// PendingFor returns the chat's open draft: a plain message may be the answer to its question.
func (s *Store) PendingFor(ctx context.Context, chatID int64) (Pending, error) {
	return s.queryPending(ctx, "WHERE chat_id = ?", chatID)
}

func (s *Store) DeletePending(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM pending WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete pending %d: %w", id, err)
	}
	return nil
}

// ExpirePending drops drafts not touched since before and returns how many.
func (s *Store) ExpirePending(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM pending WHERE updated_utc < ?", before.Unix())
	if err != nil {
		return 0, fmt.Errorf("expire pending: %w", err)
	}
	return res.RowsAffected()
}

func (s *Store) queryPending(ctx context.Context, where string, args ...any) (Pending, error) {
	var (
		p       Pending
		raw     string
		updated int64
	)
	err := s.db.QueryRowContext(ctx,
		"SELECT id, chat_id, intent_raw, COALESCE(asking, ''), updated_utc FROM pending "+where, args...).
		Scan(&p.ID, &p.ChatID, &raw, &p.Asking, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Pending{}, fmt.Errorf("pending: %w", ErrNotFound)
	}
	if err != nil {
		return Pending{}, fmt.Errorf("get pending: %w", err)
	}
	p.IntentRaw = []byte(raw)
	p.Updated = time.Unix(updated, 0).UTC()
	return p, nil
}
