package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPendingLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	first, err := s.CreatePending(ctx, 42, []byte(`{"title":"Doctor"}`), "duration", now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.PendingFor(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != first || p.ChatID != 42 || string(p.IntentRaw) != `{"title":"Doctor"}` ||
		p.Asking != "duration" || !p.Updated.Equal(now) {
		t.Fatalf("pending = %+v", p)
	}

	// An answer: same id, new content, the timeout restarts; "" means the card is shown.
	later := now.Add(3 * time.Minute)
	if err := s.UpdatePending(ctx, first, []byte(`{"title":"Doctor","duration_min":60}`), "", later); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Pending(ctx, first); string(p.IntentRaw) != `{"title":"Doctor","duration_min":60}` ||
		p.Asking != "" || !p.Updated.Equal(later) {
		t.Fatalf("after update = %+v", p)
	}

	// A new draft replaces the old one and never reuses its id,
	// even though the table held only that one row.
	second, err := s.CreatePending(ctx, 42, []byte(`{"title":"Seminar"}`), "start", later)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("new draft got the old id %d: old buttons would act on it", first)
	}
	if _, err := s.Pending(ctx, first); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old draft err = %v, want ErrNotFound", err)
	}
	if err := s.UpdatePending(ctx, first, []byte(`{}`), "", later); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of the old draft err = %v, want ErrNotFound", err)
	}

	if err := s.DeletePending(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PendingFor(ctx, 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}

func TestExpirePending(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	stale, _ := s.CreatePending(ctx, 1, []byte(`{}`), "start", now.Add(-11*time.Minute))
	fresh, _ := s.CreatePending(ctx, 2, []byte(`{}`), "start", now.Add(-9*time.Minute))

	n, err := s.ExpirePending(ctx, now.Add(-10*time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("expired %d, %v; want 1", n, err)
	}
	if _, err := s.Pending(ctx, stale); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale draft err = %v, want ErrNotFound", err)
	}
	if _, err := s.Pending(ctx, fresh); err != nil {
		t.Fatalf("fresh draft: %v", err)
	}
}
