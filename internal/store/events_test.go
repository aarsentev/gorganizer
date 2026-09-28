package store

import (
	"context"
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func ids(events []Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.ID)
	}
	return out
}

func equal(a, b []string) bool {
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

func TestUpsertAndQueryTimed(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	base := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)

	if err := s.UpsertEvents(ctx, []Event{
		{ID: "seminar", Title: "Семинар", Start: base, End: base.Add(time.Hour), Location: "ауд. 5"},
		{ID: "doctor", Title: "Врач", Start: base.Add(2 * time.Hour)},
		{ID: "late", Title: "Поздно", Start: base.Add(10 * time.Hour)},
		{ID: "birthday", Title: "ДР", Start: date(2026, 9, 29), End: date(2026, 9, 30), AllDay: true},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.TimedEventsBetween(ctx, base, base.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !equal(ids(got), []string{"seminar", "doctor"}) {
		t.Fatalf("timed = %v, want [seminar doctor]", ids(got))
	}
	if e := got[0]; e.Title != "Семинар" || e.Location != "ауд. 5" || !e.End.Equal(base.Add(time.Hour)) {
		t.Fatalf("seminar = %+v", e)
	}
	if !got[1].End.IsZero() {
		t.Fatalf("doctor End = %v, want zero", got[1].End)
	}

	// Reschedule, rename and delete: upsert updates in place, no duplicates.
	if err := s.UpsertEvents(ctx, []Event{
		{ID: "seminar", Title: "Семинар (перенос)", Start: base.Add(time.Hour)},
		{ID: "doctor", Title: "Врач", Start: base.Add(2 * time.Hour), Deleted: true},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = s.TimedEventsBetween(ctx, base, base.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !equal(ids(got), []string{"seminar"}) || got[0].Title != "Семинар (перенос)" || !got[0].Start.Equal(base.Add(time.Hour)) {
		t.Fatalf("after update = %+v", got)
	}

	if err := s.ClearEvents(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.TimedEventsBetween(ctx, base, base.Add(24*time.Hour)); len(got) != 0 {
		t.Fatalf("after clear = %v", ids(got))
	}
}

func TestAllDayEventsOn(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	if err := s.UpsertEvents(ctx, []Event{
		{ID: "birthday", Title: "ДР", Start: date(2026, 9, 29), End: date(2026, 9, 30), AllDay: true},
		{ID: "trip", Title: "Поездка", Start: date(2026, 9, 28), End: date(2026, 10, 1), AllDay: true},
		{ID: "noend", Title: "Без конца", Start: date(2026, 9, 29), AllDay: true},
		{ID: "gone", Title: "Удалено", Start: date(2026, 9, 29), End: date(2026, 9, 30), AllDay: true, Deleted: true},
		{ID: "timed", Title: "Не весь день", Start: date(2026, 9, 29).Add(8 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	// The zone of the argument does not matter, only its calendar date.
	toronto, _ := time.LoadLocation("America/Toronto")
	cases := []struct {
		day  time.Time
		want []string
	}{
		{time.Date(2026, 9, 28, 23, 0, 0, 0, toronto), []string{"trip"}},
		{time.Date(2026, 9, 29, 23, 30, 0, 0, toronto), []string{"trip", "noend", "birthday"}},
		{date(2026, 9, 30), []string{"trip"}},
		{date(2026, 10, 1), nil},
	}
	for _, c := range cases {
		got, err := s.AllDayEventsOn(ctx, c.day)
		if err != nil {
			t.Fatal(err)
		}
		if !equal(ids(got), c.want) {
			t.Errorf("AllDayEventsOn(%s) = %v, want %v", c.day.Format("2006-01-02"), ids(got), c.want)
		}
	}
}

func TestSent(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	start := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	now := start.Add(-time.Hour)

	if sent, _ := s.WasSent(ctx, "seminar", start, "60m"); sent {
		t.Fatal("WasSent before MarkSent")
	}
	if err := s.MarkSent(ctx, "seminar", start, "60m", 101, now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkSent(ctx, "seminar", start, "60m", 999, now); err != nil {
		t.Fatal("second MarkSent must be a no-op, got", err)
	}
	if sent, _ := s.WasSent(ctx, "seminar", start, "60m"); !sent {
		t.Fatal("WasSent after MarkSent = false")
	}
	moved := start.Add(4 * time.Hour)
	if sent, _ := s.WasSent(ctx, "seminar", moved, "60m"); sent {
		t.Fatal("rescheduled event counted as already reminded")
	}
	if err := s.MarkSent(ctx, "seminar", moved, "60m", 102, moved.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := s.ReminderMessages(ctx, start.Add(-24*time.Hour), start.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 101 || got[1] != 102 {
		t.Fatalf("ReminderMessages = %v, want [101 102]", got)
	}

	if err := s.ForgetReminderMessage(ctx, 101); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ReminderMessages(ctx, start.Add(-24*time.Hour), start.Add(24*time.Hour))
	if len(got) != 1 || got[0] != 102 {
		t.Fatalf("after forget = %v, want [102]", got)
	}
	if sent, _ := s.WasSent(ctx, "seminar", start, "60m"); !sent {
		t.Fatal("forgetting the message must keep the sent mark")
	}
}
