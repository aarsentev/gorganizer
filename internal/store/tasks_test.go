package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func taskIDs(tasks []Task) []int64 {
	var out []int64
	for _, t := range tasks {
		out = append(out, t.ID)
	}
	return out
}

func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

	part, err := s.CreateTask(ctx, "Pick up the part", now.Add(6*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	call, _ := s.CreateTask(ctx, "Call the bank", now.Add(time.Hour), now)
	later, _ := s.CreateTask(ctx, "Renew passport", now.Add(48*time.Hour), now)

	got, err := s.Task(ctx, part)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Pick up the part" || !got.Remind.Equal(now.Add(6*time.Hour)) || got.Status != TaskOpen || got.Reminded {
		t.Fatalf("task = %+v", got)
	}

	if due, _ := s.DueTasks(ctx, now.Add(time.Hour)); !sameIDs(taskIDs(due), []int64{call}) {
		t.Fatalf("due at +1h = %v, want [%d]", taskIDs(due), call)
	}
	if err := s.MarkTaskReminded(ctx, call); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.DueTasks(ctx, now.Add(6*time.Hour)); !sameIDs(taskIDs(due), []int64{part}) {
		t.Fatalf("due at +6h = %v, a reminded task must not come back", taskIDs(due))
	}

	// "In an hour": a new time arms the reminder again.
	if err := s.RescheduleTask(ctx, call, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.DueTasks(ctx, now.Add(2*time.Hour)); !sameIDs(taskIDs(due), []int64{call}) {
		t.Fatalf("due after reschedule = %v", taskIDs(due))
	}

	if open, _ := s.OpenTasksBefore(ctx, now.Add(24*time.Hour)); !sameIDs(taskIDs(open), []int64{call, part}) {
		t.Fatalf("open today = %v, want [%d %d] without %d", taskIDs(open), call, part, later)
	}
	if err := s.SetTaskStatus(ctx, part, TaskDone); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskStatus(ctx, call, TaskCancelled); err != nil {
		t.Fatal(err)
	}
	if open, _ := s.OpenTasksBefore(ctx, now.Add(24*time.Hour)); len(open) != 0 {
		t.Fatalf("open after closing = %v", taskIDs(open))
	}
	if due, _ := s.DueTasks(ctx, now.Add(72*time.Hour)); !sameIDs(taskIDs(due), []int64{later}) {
		t.Fatalf("closed tasks must never be due, got %v", taskIDs(due))
	}
}

func TestMissingTask(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if _, err := s.Task(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Task(99) err = %v, want ErrNotFound", err)
	}
	if err := s.SetTaskStatus(ctx, 99, TaskDone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetTaskStatus(99) err = %v, want ErrNotFound", err)
	}
}
