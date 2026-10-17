package app

import (
	"context"
	"errors"
	"testing"

	"go-organizer/internal/intent"
	"go-organizer/internal/store"
)

func TestDraftRoundTrip(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newTestApp(t, nil, nil)
	start := at(t, "2026-09-30 09:00")
	in := intent.Intent{Action: "create", Kind: "event", Title: "Приём у врача", Start: &start, DurationMin: 60, Location: "клиника"}

	id, err := a.newDraft(ctx, 42, in, "location", at(t, "2026-09-29 10:00"))
	if err != nil {
		t.Fatal(err)
	}
	p, got, err := a.draft(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Asking != "location" || got.Title != in.Title || !got.Start.Equal(start) ||
		got.DurationMin != 60 || got.Location != "клиника" || got.Kind != "event" {
		t.Fatalf("draft = %+v, %+v", p, got)
	}

	if _, _, err := a.draft(ctx, id+1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing draft err = %v, want ErrNotFound", err)
	}
}
