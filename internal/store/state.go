package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Keys of the state table. Callers use the named accessors below, never raw keys.
const (
	keyTZ = "tz"
)

// TZ returns the stored IANA zone name, or "" if it was never set.
func (s *Store) TZ(ctx context.Context) (string, error) {
	return s.get(ctx, keyTZ)
}

func (s *Store) SetTZ(ctx context.Context, name string) error {
	return s.set(ctx, keyTZ, name)
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
