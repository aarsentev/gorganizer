package store

import (
	"context"
	"database/sql"
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
		if version != 6 {
			t.Fatalf("user_version = %d, want 6", version)
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

func TestStateAccessors(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	if token, _ := s.SyncToken(ctx); token != "" {
		t.Fatalf("initial SyncToken = %q", token)
	}
	if err := s.SetSyncToken(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if token, _ := s.SyncToken(ctx); token != "abc" {
		t.Fatalf("SyncToken = %q, want abc", token)
	}
	if err := s.SetSyncToken(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if token, _ := s.SyncToken(ctx); token != "" {
		t.Fatalf("SyncToken after reset = %q", token)
	}

	if zone, _ := s.CalendarTZ(ctx); zone != "" {
		t.Fatalf("initial CalendarTZ = %q", zone)
	}
	if err := s.SetCalendarTZ(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	if zone, _ := s.CalendarTZ(ctx); zone != "Asia/Singapore" {
		t.Fatalf("CalendarTZ = %q, want Asia/Singapore", zone)
	}
	if tz, _ := s.TZ(ctx); tz != "" {
		t.Fatalf("SetCalendarTZ changed TZ to %q", tz)
	}

	if err := s.SetLang(ctx, "ru"); err != nil {
		t.Fatal(err)
	}
	if lang, _ := s.Lang(ctx); lang != "ru" {
		t.Fatalf("Lang = %q, want ru", lang)
	}

	if id, err := s.EveningMessageID(ctx); err != nil || id != 0 {
		t.Fatalf("initial EveningMessageID = %d, %v", id, err)
	}
	if err := s.SetEveningMessageID(ctx, 4242); err != nil {
		t.Fatal(err)
	}
	if id, _ := s.EveningMessageID(ctx); id != 4242 {
		t.Fatalf("EveningMessageID = %d, want 4242", id)
	}
	if err := s.SetEveningMessageID(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if id, _ := s.EveningMessageID(ctx); id != 0 {
		t.Fatalf("EveningMessageID after clear = %d", id)
	}

	if until, err := s.PausedUntil(ctx); err != nil || !until.IsZero() {
		t.Fatalf("initial PausedUntil = %v, %v", until, err)
	}
	end := time.Date(2026, 10, 2, 12, 5, 0, 0, time.UTC)
	if err := s.SetPausedUntil(ctx, end); err != nil {
		t.Fatal(err)
	}
	if until, _ := s.PausedUntil(ctx); !until.Equal(end) {
		t.Fatalf("PausedUntil = %v, want %v", until, end)
	}
	if err := s.SetPausedUntil(ctx, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if until, _ := s.PausedUntil(ctx); !until.IsZero() {
		t.Fatalf("PausedUntil after lifting = %v", until)
	}

	if built, err := s.LastBuild(ctx); err != nil || built != "" {
		t.Fatalf("initial LastBuild = %q, %v", built, err)
	}
	if err := s.SetLastBuild(ctx, "2026-10-24T10:20Z"); err != nil {
		t.Fatal(err)
	}
	if built, _ := s.LastBuild(ctx); built != "2026-10-24T10:20Z" {
		t.Fatalf("LastBuild = %q", built)
	}

	if version, err := s.SchemaVersion(ctx); err != nil || version != 6 {
		t.Fatalf("SchemaVersion = %d, %v; want 6", version, err)
	}
}

func TestMigrateTrackersFromVersion4(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	legacy := &Store{db: db}
	for i, name := range []string{"0001_init.sql", "0002_sent_message_id.sql", "0003_tasks.sql", "0004_pending.sql"} {
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := legacy.apply(ctx, i+1, string(body)); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	task, err := legacy.CreateTask(ctx, "Existing task", now, now)
	if err != nil {
		t.Fatal(err)
	}
	note, err := legacy.CreateNote(ctx, "Existing note", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.SetTZ(ctx, "Asia/Irkutsk"); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if version, err := s.SchemaVersion(ctx); err != nil || version != 6 {
		t.Fatalf("upgraded version = %d, %v; want 6", version, err)
	}
	if got, err := s.Task(ctx, task); err != nil || got.Title != "Existing task" {
		t.Fatalf("task after upgrade = %+v, %v", got, err)
	}
	if got, err := s.Note(ctx, note); err != nil || got.Text != "Existing note" {
		t.Fatalf("note after upgrade = %+v, %v", got, err)
	}
	if tz, err := s.TZ(ctx); err != nil || tz != "Asia/Irkutsk" {
		t.Fatalf("timezone after upgrade = %q, %v", tz, err)
	}
	trackers, err := s.Trackers(ctx)
	if err != nil || len(trackers) != 0 {
		t.Fatalf("upgrade must not enable trackers automatically: %+v, %v", trackers, err)
	}
	id := createTracker(t, s, "Mood", TrackerRating, "", now)
	if err := s.SaveTrackerEntry(ctx, id, "2026-11-18", 4, now); err != nil {
		t.Fatalf("save after upgrade: %v", err)
	}
}

func TestTrackerEntryForeignKey(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	// Check SQLite itself enforces the relationship, even outside the store API.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tracker_entries (tracker_id, day, value, created_utc, updated_utc)
		VALUES (999, '2026-11-18', 1, 0, 0)`); err == nil {
		t.Fatal("database accepted an entry without a tracker")
	}
	now := time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)
	id := createTracker(t, s, "Mood", TrackerRating, "", now)
	if err := s.SaveTrackerEntry(ctx, id, "2026-11-18", 4, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM trackers WHERE id = ?", id); err == nil {
		t.Fatal("database allowed deletion of a tracker with history")
	}
}

func TestMigrateTrackerPresetsFromVersion5(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "presets.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	legacy := &Store{db: db}
	for i, name := range []string{"0001_init.sql", "0002_sent_message_id.sql", "0003_tasks.sql", "0004_pending.sql", "0005_trackers.sql"} {
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := legacy.apply(ctx, i+1, string(body)); err != nil {
			t.Fatal(err)
		}
	}
	// Version 5 had no preset column. Preserve the old tracker and its answers as-is.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO trackers (id, name, kind, status, created_utc) VALUES (1, 'Mood', 'rating', 'paused', 100);
		INSERT INTO tracker_entries (tracker_id, day, value, created_utc, updated_utc)
		VALUES (1, '2026-11-19', 4, 100, 100)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	got, err := s.Tracker(ctx, 1)
	if err != nil || got.Preset != "" || got.Name != "Mood" || got.Status != TrackerPaused {
		t.Fatalf("old tracker after migration = %+v, %v", got, err)
	}
	entry, err := s.TrackerEntry(ctx, 1, "2026-11-19")
	if err != nil || entry.Value != 4 {
		t.Fatalf("old history after migration = %+v, %v", entry, err)
	}
	id, err := s.CreatePresetTracker(ctx, "mood", "Mood", TrackerRating, "", time.Unix(200, 0))
	if err != nil || id == 1 {
		t.Fatalf("new preset = %d, %v; must not claim an unrelated old tracker", id, err)
	}
}

func TestUnfire(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 9, 29, 5, 30, 0, 0, time.UTC)

	if ok, _ := s.TryFire(ctx, "digest", "2026-09-29", now); !ok {
		t.Fatal("first TryFire = false")
	}
	if err := s.Unfire(ctx, "digest", "2026-09-29"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.TryFire(ctx, "digest", "2026-09-29", now); !ok {
		t.Fatal("TryFire after Unfire = false, the job must be able to run again")
	}
}
