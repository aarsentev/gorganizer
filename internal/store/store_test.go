package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	for range 2 {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		var version int
		if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != 1 {
			t.Fatalf("user_version = %d, want 1", version)
		}
		var mode string
		if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" {
			t.Fatalf("journal_mode = %q, want wal", mode)
		}
		s.Close()
	}
}

func TestTZ(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	if tz, err := s.TZ(ctx); err != nil || tz != "" {
		t.Fatalf("initial TZ = %q, %v; want empty", tz, err)
	}
	for _, want := range []string{"Europe/Warsaw", "America/Toronto"} {
		if err := s.SetTZ(ctx, want); err != nil {
			t.Fatal(err)
		}
		if got, err := s.TZ(ctx); err != nil || got != want {
			t.Fatalf("TZ = %q, %v; want %q", got, err, want)
		}
	}
}

func TestTryFire(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)

	steps := []struct {
		job, day string
		want     bool
	}{
		{"digest", "2026-09-29", true},
		{"digest", "2026-09-29", false},
		{"checkin", "2026-09-29", true},
		{"digest", "2026-09-30", true},
	}
	for _, st := range steps {
		got, err := s.TryFire(ctx, st.job, st.day, now)
		if err != nil {
			t.Fatal(err)
		}
		if got != st.want {
			t.Fatalf("TryFire(%s, %s) = %v, want %v", st.job, st.day, got, st.want)
		}
	}
}
