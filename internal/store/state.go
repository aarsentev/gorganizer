package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// Keys of the state table. Callers use the named accessors below, never raw keys.
const (
	keyTZ               = "tz"
	keyLang             = "lang"
	keySyncToken        = "sync_token"
	keyEveningMessageID = "evening_message_id"
)

// TZ returns the stored IANA zone name, or "" if it was never set.
func (s *Store) TZ(ctx context.Context) (string, error) {
	return s.get(ctx, keyTZ)
}

func (s *Store) SetTZ(ctx context.Context, name string) error {
	return s.set(ctx, keyTZ, name)
}

// Lang returns the stored interface language, or "" if it was never set.
func (s *Store) Lang(ctx context.Context) (string, error) {
	return s.get(ctx, keyLang)
}

func (s *Store) SetLang(ctx context.Context, lang string) error {
	return s.set(ctx, keyLang, lang)
}

// SyncToken returns the Google Calendar sync token, or "" before the first full sync.
func (s *Store) SyncToken(ctx context.Context) (string, error) {
	return s.get(ctx, keySyncToken)
}

// SetSyncToken stores the token; "" forces a full sync next time.
func (s *Store) SetSyncToken(ctx context.Context, token string) error {
	if token == "" {
		return s.del(ctx, keySyncToken)
	}
	return s.set(ctx, keySyncToken, token)
}

// EveningMessageID returns the id of the last evening message to delete in the morning, or 0.
func (s *Store) EveningMessageID(ctx context.Context) (int64, error) {
	value, err := s.get(ctx, keyEveningMessageID)
	if err != nil || value == "" {
		return 0, err
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("state %s: %w", keyEveningMessageID, err)
	}
	return id, nil
}

// SetEveningMessageID stores the id; 0 clears it.
func (s *Store) SetEveningMessageID(ctx context.Context, id int64) error {
	if id == 0 {
		return s.del(ctx, keyEveningMessageID)
	}
	return s.set(ctx, keyEveningMessageID, strconv.FormatInt(id, 10))
}

func (s *Store) get(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM state WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get state %s: %w", key, err)
	}
	return value, nil
}

func (s *Store) set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO state (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value",
		key, value)
	if err != nil {
		return fmt.Errorf("set state %s: %w", key, err)
	}
	return nil
}

func (s *Store) del(ctx context.Context, key string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM state WHERE key = ?", key); err != nil {
		return fmt.Errorf("delete state %s: %w", key, err)
	}
	return nil
}
