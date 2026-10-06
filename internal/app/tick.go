package app

import (
	"context"
	"time"
)

// Every calls f right away and then every d until ctx is done.
// It replaces a scheduler package: all logic lives in the functions it calls.
func Every(ctx context.Context, d time.Duration, f func(context.Context, time.Time)) error {
	f(ctx, time.Now())
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-t.C:
			f(ctx, now)
		}
	}
}

// Tick runs every minute and reads only the local cache, never Google.
// It is idempotent: a missed tick is harmless, the next one catches up.
func (a *App) Tick(ctx context.Context, now time.Time) {
	// Quiet hours only block sending. Everything due meanwhile goes out on the first tick after.
	if a.quiet(now) {
		return
	}
	// Without a calendar the event cache is never synced: it can only be stale.
	if a.cal != nil {
		a.eventReminders(ctx, now)
	}
	// Daily jobs first: the day review takes in tasks due right now, and a separate ⏰ would repeat it.
	a.dailyJobs(ctx, now)
	a.taskReminders(ctx, now)
}

func (a *App) quiet(now time.Time) bool {
	from, to := clockMinutes(a.cfg.QuietHours.From), clockMinutes(a.cfg.QuietHours.To)
	m := minuteOfDay(now.In(a.Loc()))
	if from <= to {
		return m >= from && m < to
	}
	return m >= from || m < to // wraps over midnight, e.g. 23:00–08:00
}

// clockMinutes turns "HH:MM" (validated by config) into minutes since midnight.
func clockMinutes(s string) int {
	t, _ := time.Parse("15:04", s)
	return t.Hour()*60 + t.Minute()
}

func minuteOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
