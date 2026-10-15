package app

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// What /add understands at either end of the title, in any order: a date, and a time
// or a time range. Only forms without a language: words like "tomorrow", "Monday" or
// units like "1h" would need a dictionary per language, they come with the model at stage 5.
var (
	dateRe  = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})(?:\.(\d{4}|\d{2}))?$`)
	rangeRe = regexp.MustCompile(`^(\d{1,2}:\d{2})[-–—](\d{1,2}:\d{2})$`)
)

var (
	errBadDate  = errors.New("no such date")
	errBadRange = errors.New("no such time range")
	errNotWhen  = errors.New("not a date or a time")
)

// said is what was said about when and how long; empty fields were not said.
type said struct {
	date     string // 29.09[.2026]
	clock    string // H:MM, the start
	duration int    // minutes, from a range like 9:00-10:30
}

// parseAdd splits /add arguments into the title, the place after " @" and what was
// said about when and how long. A bare number is part of the title: "2 ящика пива".
func parseAdd(args string) (title, location string, s said, err error) {
	rest, location, _ := strings.Cut(" "+args, " @")
	location = strings.TrimSpace(location)
	words := strings.Fields(rest)
	from, to := 0, len(words)
	for from < to {
		ok, err := s.take(words[from])
		if err != nil {
			return "", "", said{}, err
		}
		if !ok {
			break
		}
		from++
	}
	for from < to {
		ok, err := s.take(words[to-1])
		if err != nil {
			return "", "", said{}, err
		}
		if !ok {
			break
		}
		to--
	}
	if from == to {
		return "", "", said{}, errNoTitle
	}
	return capitalize(strings.Join(words[from:to], " ")), location, s, nil
}

// parseWhen reads an answer to the bot's question that holds nothing but
// a date and a time or a range, in any combination: "30.09 9:00", "10:30".
func parseWhen(text string) (said, error) {
	var s said
	for _, word := range strings.Fields(text) {
		ok, err := s.take(word)
		if err != nil {
			return said{}, err
		}
		if !ok {
			return said{}, errNotWhen
		}
	}
	if s == (said{}) {
		return said{}, errNotWhen
	}
	return s, nil
}

// take files the word as a date, or a time or a range, not said yet. A second date
// or time is not taken: it stays in the title.
func (s *said) take(w string) (bool, error) {
	switch {
	case s.date == "" && dateRe.MatchString(w):
		if !validDate(dateRe.FindStringSubmatch(w)) {
			return false, errBadDate
		}
		s.date = w
	case s.clock == "" && clockRe.MatchString(w):
		s.clock = w
	case s.clock == "" && rangeRe.MatchString(w):
		m := rangeRe.FindStringSubmatch(w)
		minutes := rangeMinutes(m[1], m[2])
		if minutes == 0 {
			return false, errBadRange
		}
		s.clock, s.duration = m[1], minutes
	default:
		return false, nil
	}
	return true, nil
}

// rangeMinutes is how long from start to end, 0 if either is not a time or they are equal.
// An end before the start is on the next day: 23:00-7:00 is 8 hours.
func rangeMinutes(start, end string) int {
	if !clockRe.MatchString(start) || !clockRe.MatchString(end) {
		return 0
	}
	const day = 24 * 60
	return (clockMinutes(end) - clockMinutes(start) + day) % day
}

// validDate checks a matched 29.09[.2026]. Without a year 29.02 is fine:
// some year has it, the nearest such year is picked later.
func validDate(m []string) bool {
	d, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	y := 2024 // a leap year
	if m[3] != "" {
		y = fullYear(m[3])
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	return t.Day() == d && int(t.Month()) == mo
}

func fullYear(s string) int {
	y, _ := strconv.Atoi(s)
	if y < 100 {
		y += 2000
	}
	return y
}

// moment turns what was said into the event's start, or, when only the date is
// known, into that date (2026-09-29) so the time can be asked. A time alone is today,
// or tomorrow if it has passed, as in /task. A start in the past is returned as is:
// the dialog asks again.
func (a *App) moment(s said, now time.Time) (*time.Time, string) {
	if s.date == "" {
		if s.clock == "" {
			return nil, ""
		}
		start := a.nextAt(now, s.clock, false)
		return &start, ""
	}
	day := a.resolveDay(s.date, now)
	if s.clock == "" {
		return nil, day.Format(time.DateOnly)
	}
	start := atClock(day, s.clock)
	return &start, ""
}

// resolveDay is the local midnight of 29.09[.2026]. Without a year it is
// the nearest such date from today on.
func (a *App) resolveDay(date string, now time.Time) time.Time {
	today := midnight(now.In(a.Loc()))
	m := dateRe.FindStringSubmatch(date)
	d, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	if m[3] != "" {
		return time.Date(fullYear(m[3]), time.Month(mo), d, 0, 0, 0, 0, today.Location())
	}
	for y := today.Year(); ; y++ {
		// t.Day() differs when the year has no such date: 29.02 rolls over to March.
		t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, today.Location())
		if t.Day() == d && !t.Before(today) {
			return t
		}
	}
}
