package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// WasSent reports whether the reminder of this kind was sent for the event
// with this start. A rescheduled event has a new start and gets reminded again.
func (s *Store) WasSent(ctx context.Context, eventID string, start time.Time, kind string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM sent WHERE event_id = ? AND start_utc = ? AND kind = ?",
		eventID, start.Unix(), kind).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("was sent %s: %w", eventID, err)
	}
	return true, nil
}

func (s *Store) MarkSent(ctx context.Context, eventID string, start time.Time, kind string, messageID int64, now time.Time) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO sent (event_id, start_utc, kind, sent_utc, message_id) VALUES (?, ?, ?, ?, ?)",
		eventID, start.Unix(), kind, now.Unix(), messageID)
	if err != nil {
		return fmt.Errorf("mark sent %s: %w", eventID, err)
	}
	return nil
}

// ReminderMessages returns Telegram ids of reminders sent in [from, to)
// that were not cleaned up yet.
func (s *Store) ReminderMessages(ctx context.Context, from, to time.Time) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT message_id FROM sent WHERE message_id IS NOT NULL AND sent_utc >= ? AND sent_utc < ? ORDER BY sent_utc",
		from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("reminder messages: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reminder messages: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ForgetReminderMessage drops the id after the message was deleted (or could not be),
// so cleanup does not retry it forever. The sent mark itself stays.
func (s *Store) ForgetReminderMessage(ctx context.Context, messageID int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE sent SET message_id = NULL WHERE message_id = ?", messageID); err != nil {
		return fmt.Errorf("forget message %d: %w", messageID, err)
	}
	return nil
}
