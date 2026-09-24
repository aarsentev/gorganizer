package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db *sql.DB
}

// Open opens the database at path (":memory:" for tests) and applies pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// One connection: SQLite has a single writer, and ":memory:" is per-connection.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// migrate applies migrations/NNNN_*.sql with NNNN > PRAGMA user_version,
// each in its own transaction together with the version bump.
func (s *Store) migrate(ctx context.Context) error {
	var current int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: bad version prefix", name)
		}
		if version <= current {
			continue
		}
		if version != current+1 {
			return fmt.Errorf("migration %s: expected version %d", name, current+1)
		}

		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		// An empty file would still bump user_version and shadow the real migration.
		if strings.TrimSpace(string(body)) == "" {
			return fmt.Errorf("migration %s is empty", name)
		}
		if err := s.apply(ctx, version, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		current = version
	}
	return nil
}

func (s *Store) apply(ctx context.Context, version int, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

// TryFire records that a daily job ran on the given local day.
// It returns true only for the first call per (job, day).
func (s *Store) TryFire(ctx context.Context, job, day string, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO jobs_fired (job, day, fired_utc) VALUES (?, ?, ?)",
		job, day, now.Unix())
	if err != nil {
		return false, fmt.Errorf("try fire %s %s: %w", job, day, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("try fire %s %s: %w", job, day, err)
	}
	return n == 1, nil
}
