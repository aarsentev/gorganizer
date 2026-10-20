package app

import (
	"regexp"
	"strconv"
	"time"
)

// Time helpers shared by the features. A clock is "H:MM", as in config.yaml and in
// commands; a day is a local midnight in the user's zone, a.Loc().

// clockRe is a time of day: 9:00 or 09:00.
var clockRe = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

// midnight is the start of t's day, in t's zone.
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// atClock is the given "H:MM" on the local day; time.Date handles DST gaps.
func atClock(day time.Time, clock string) time.Time {
	m := clockRe.FindStringSubmatch(clock)
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	return time.Date(day.Year(), day.Month(), day.Day(), h, min, 0, 0, day.Location())
}

// nextAt is the clock time today, or tomorrow if that has passed or was asked for.
func (a *App) nextAt(now time.Time, clock string, tomorrow bool) time.Time {
	local := now.In(a.Loc())
	day := midnight(local)
	if tomorrow {
		day = day.AddDate(0, 0, 1)
	}
	t := atClock(day, clock)
	if !tomorrow && !t.After(local) {
		t = atClock(day.AddDate(0, 0, 1), clock)
	}
	return t
}

// clockMinutes turns "HH:MM" (validated by config) into minutes since midnight.
func clockMinutes(s string) int {
	t, _ := time.Parse("15:04", s)
	return t.Hour()*60 + t.Minute()
}

func minuteOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}

// whenView lets templates say "today at 18:00" / "tomorrow at 18:00" / "Tue, Sep 29 at 18:00".
type whenView struct {
	T               time.Time
	Today, Tomorrow bool
}

func (a *App) when(t, now time.Time) whenView {
	local, today := t.In(a.Loc()), midnight(now.In(a.Loc()))
	day := midnight(local)
	return whenView{T: local, Today: day.Equal(today), Tomorrow: day.Equal(today.AddDate(0, 0, 1))}
}
