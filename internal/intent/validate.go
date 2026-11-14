package intent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Errors of Validate, one per answer the app gives: whatever is wrong with an intent is
// asked again, never shown as a failure (SPEC §7).
var (
	ErrUnsupported = errors.New("not supported yet") // update and delete come later
	ErrPast        = errors.New("in the past")
	ErrInvalid     = errors.New("invalid intent") // wrapped with the reason, for the log
)

// maxEventMinutes bounds a timed event: a longer one is more likely a parser's mistake,
// and events of several days come as all-day ones.
const maxEventMinutes = 24 * 60

// anchors are the task times a parser may name instead of a clock; config.yaml sets them.
var anchors = []string{"morning", "afternoon", "evening", "eod"}

// Validate checks what a parser handed over before the app acts on it. The model may
// return anything its schema allows, so nothing is trusted. now must be in the user's
// zone: which day is today depends on it.
func (in Intent) Validate(now time.Time) error {
	switch in.Action {
	case "create":
		return in.validateCreate(now)
	case "note":
		if strings.TrimSpace(in.Text) == "" {
			return invalid("note without text")
		}
		return nil
	case "clarify":
		return nil // the parser itself says it did not understand
	case "update", "delete":
		return ErrUnsupported
	}
	return invalid("unknown action %q", in.Action)
}

// validateCreate allows what is missing, since the app asks about it, but nothing wrong.
func (in Intent) validateCreate(now time.Time) error {
	if in.Kind != "event" && in.Kind != "task" {
		return invalid("unknown kind %q", in.Kind)
	}
	if strings.TrimSpace(in.Title) == "" {
		return invalid("no title")
	}
	if in.Start != nil && in.When != "" {
		return invalid("both start and when")
	}
	if in.Start != nil && !in.Start.After(now) {
		return ErrPast
	}
	switch {
	case in.Kind == "event" && (in.DurationMin < 0 || in.DurationMin > maxEventMinutes):
		return invalid("duration %d min", in.DurationMin)
	case in.Kind == "task" && slices.Contains(anchors, in.When):
		return nil // an anchor is not a date
	}
	return checkDate(in.When, now)
}

// checkDate accepts no date, or a real one from today on.
func checkDate(when string, now time.Time) error {
	if when == "" {
		return nil
	}
	day, err := time.ParseInLocation(time.DateOnly, when, now.Location())
	if err != nil {
		return invalid("when %q", when)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if day.Before(today) {
		return ErrPast
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}
