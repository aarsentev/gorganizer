package intent

import (
	"errors"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	zone := time.FixedZone("UTC+2", 2*60*60)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, zone)
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)
	event := func(change func(*Intent)) Intent {
		in := Intent{Action: "create", Kind: "event", Title: "Doctor"}
		change(&in)
		return in
	}
	task := func(change func(*Intent)) Intent {
		in := Intent{Action: "create", Kind: "task", Title: "Call mom"}
		change(&in)
		return in
	}

	cases := []struct {
		name string
		in   Intent
		want error
	}{
		{"complete event", event(func(in *Intent) { in.Start, in.DurationMin = &later, 60 }), nil},
		{"title only: the app asks the rest", event(func(*Intent) {}), nil},
		{"date of today", event(func(in *Intent) { in.When = "2026-09-29" }), nil},
		{"date of yesterday", event(func(in *Intent) { in.When = "2026-09-28" }), ErrPast},
		{"date not in ISO form", event(func(in *Intent) { in.When = "30.09" }), ErrInvalid},
		{"no such date", event(func(in *Intent) { in.When = "2026-02-30" }), ErrInvalid},
		{"start in the past", event(func(in *Intent) { in.Start = &earlier }), ErrPast},
		{"start and date both", event(func(in *Intent) { in.Start, in.When = &later, "2026-09-30" }), ErrInvalid},
		{"negative duration", event(func(in *Intent) { in.Start, in.DurationMin = &later, -5 }), ErrInvalid},
		{"a whole day", event(func(in *Intent) { in.Start, in.DurationMin = &later, 24*60 }), nil},
		{"longer than a day", event(func(in *Intent) { in.Start, in.DurationMin = &later, 25*60 }), ErrInvalid},
		{"blank title", event(func(in *Intent) { in.Title = "  " }), ErrInvalid},
		{"unknown kind", event(func(in *Intent) { in.Kind = "meeting" }), ErrInvalid},
		{"task at an anchor", task(func(in *Intent) { in.When = "evening" }), nil},
		{"task at an unknown anchor", task(func(in *Intent) { in.When = "tonight" }), ErrInvalid},
		{"task on a date", task(func(in *Intent) { in.When = "2026-09-30" }), nil},
		{"task at a time", task(func(in *Intent) { in.Start = &later }), nil},
		{"task with a duration", task(func(in *Intent) { in.DurationMin = 15 }), nil},
		{"note", Intent{Action: "note", Text: "Tyre size 205/55 R16"}, nil},
		{"empty note", Intent{Action: "note"}, ErrInvalid},
		{"clarify", Intent{Action: "clarify"}, nil},
		{"update", Intent{Action: "update"}, ErrUnsupported},
		{"delete", Intent{Action: "delete"}, ErrUnsupported},
		{"no action", Intent{}, ErrInvalid},
	}
	for _, c := range cases {
		err := c.in.Validate(now)
		if (c.want == nil && err != nil) || !errors.Is(err, c.want) {
			t.Errorf("%s: Validate() = %v, want %v", c.name, err, c.want)
		}
	}
}

// Which day is today depends on the zone: at 01:00 on the 29th in UTC+2 it is still the
// 28th in UTC. A date of the 28th is past for the user and would pass in UTC.
func TestValidateTodayInUserZone(t *testing.T) {
	zone := time.FixedZone("UTC+2", 2*60*60)
	now := time.Date(2026, 9, 29, 1, 0, 0, 0, zone)
	in := Intent{Action: "create", Kind: "event", Title: "Doctor", When: "2026-09-28"}
	if err := in.Validate(now); !errors.Is(err, ErrPast) {
		t.Fatalf("yesterday in the user's zone: %v, want ErrPast", err)
	}
	if err := in.Validate(now.UTC()); err != nil {
		t.Fatalf("the same moment in UTC: %v, want today there", err)
	}
}

func TestInvalidKeepsTheReason(t *testing.T) {
	later := time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)
	in := Intent{Action: "create", Kind: "event", Title: "Doctor", Start: &later, DurationMin: 1500}
	err := in.Validate(later.Add(-time.Hour))
	if err == nil || err.Error() != "invalid intent: duration 1500 min" {
		t.Fatalf("err = %v", err)
	}
}
