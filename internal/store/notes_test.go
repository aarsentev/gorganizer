package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoteLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	bulb, err := s.CreateNote(ctx, "buy a light bulb", now)
	if err != nil {
		t.Fatal(err)
	}
	call, _ := s.CreateNote(ctx, "call about the meter", now.Add(time.Minute))
	idea, _ := s.CreateNote(ctx, "idea: CSV export", now.Add(2*time.Minute))

	got, err := s.Note(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "call about the meter" || !got.At.Equal(now.Add(time.Minute)) || got.Done {
		t.Fatalf("note = %+v", got)
	}

	if ids, _ := s.OpenNoteIDs(ctx); !sameIDs(ids, []int64{bulb, call, idea}) {
		t.Fatalf("open = %v, want all three oldest first", ids)
	}

	if err := s.SetNoteDone(ctx, call, true); err != nil {
		t.Fatal(err)
	}
	if ids, _ := s.OpenNoteIDs(ctx); !sameIDs(ids, []int64{bulb, idea}) {
		t.Fatalf("open = %v, a handled note must leave the inbox", ids)
	}
	if got, _ := s.Note(ctx, call); !got.Done {
		t.Fatal("handled note is not marked done")
	}

	// Pressed by mistake: the note comes back to its place.
	if err := s.SetNoteDone(ctx, call, false); err != nil {
		t.Fatal(err)
	}
	if ids, _ := s.OpenNoteIDs(ctx); !sameIDs(ids, []int64{bulb, call, idea}) {
		t.Fatalf("open = %v after undo", ids)
	}
}

func TestMissingNote(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	if _, err := s.Note(ctx, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Note err = %v, want ErrNotFound", err)
	}
	if err := s.SetNoteDone(ctx, 7, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetNoteDone err = %v, want ErrNotFound", err)
	}
	if ids, err := s.OpenNoteIDs(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("open = %v, %v; want empty", ids, err)
	}
}
