package app

import (
	"context"
	"time"
)

const (
	jobDigest  = "digest"
	jobEvening = "evening"
)

// dailyJobs: "local time >= X and not done today", never "exactly X", so a restart
// does not lose a job and a flight west does not repeat it. The digest window ends
// at the evening time: a morning digest at 23:00 after a long downtime is useless.
func (a *App) dailyJobs(ctx context.Context, now time.Time) {
	local := now.In(a.Loc())
	m := minuteOfDay(local)
	digestAt := clockMinutes(a.cfg.Digest.At)
	eveningAt := clockMinutes(a.cfg.Reminders.EveningAt)

	if m >= digestAt && m < eveningAt {
		a.fire(ctx, jobDigest, local, now, a.digest)
	}
	if m >= eveningAt {
		a.fire(ctx, jobEvening, local, now, a.evening)
	}
}

func (a *App) fire(ctx context.Context, job string, local, now time.Time, run func(context.Context, time.Time) error) {
	day := local.Format(time.DateOnly)
	ok, err := a.store.TryFire(ctx, job, day, now)
	if err != nil {
		a.log.Error("daily job", "job", job, "err", err)
		return
	}
	if !ok {
		return
	}
	if err := run(ctx, local); err != nil {
		a.log.Error("daily job", "job", job, "err", err)
		// Let the next tick try again instead of losing the job for today.
		if err := a.store.Unfire(ctx, job, day); err != nil {
			a.log.Error("daily job", "job", job, "err", err)
		}
	}
}

// evening sends tomorrow's events. Its id is kept so the morning digest can delete it.
func (a *App) evening(ctx context.Context, local time.Time) error {
	tomorrow := midnight(local).AddDate(0, 0, 1)
	id, err := a.sendDay(ctx, tomorrow, "evening", "evening_empty")
	if err != nil {
		return err
	}
	// The morning never ran (downtime), so yesterday's evening message is still there.
	a.deleteEvening(ctx)
	return a.store.SetEveningMessageID(ctx, id)
}

// digest sends today's events, then removes the evening message it replaces.
// Send first: if sending fails, last evening's message stays.
func (a *App) digest(ctx context.Context, local time.Time) error {
	today := midnight(local)
	if _, err := a.sendDay(ctx, today, "digest", "digest_empty"); err != nil {
		return err
	}
	a.deleteEvening(ctx)
	if a.cfg.Reminders.Cleanup {
		a.cleanupReminders(ctx, today)
	}
	return nil
}

func (a *App) sendDay(ctx context.Context, day time.Time, block, emptyBlock string) (int64, error) {
	events, err := a.dayEvents(ctx, day)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		block = emptyBlock
	}
	text, err := a.render(block, struct {
		Day    time.Time
		Events []eventView
	}{day, events})
	if err != nil {
		return 0, err
	}
	return a.send.Send(ctx, a.cfg.ChatID, text)
}

// dayEvents lists all-day events first, then timed ones by start, for a local day.
func (a *App) dayEvents(ctx context.Context, day time.Time) ([]eventView, error) {
	allDay, err := a.store.AllDayEventsOn(ctx, day)
	if err != nil {
		return nil, err
	}
	// AddDate keeps local midnight across DST changes, unlike adding 24 hours.
	timed, err := a.store.TimedEventsBetween(ctx, day, day.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	var out []eventView
	for _, e := range append(allDay, timed...) {
		out = append(out, a.view(e))
	}
	return out, nil
}

// deleteEvening errors are only logged: the message may be gone already
// or older than Telegram's 48-hour limit.
func (a *App) deleteEvening(ctx context.Context) {
	id, err := a.store.EveningMessageID(ctx)
	if err != nil || id == 0 {
		return
	}
	if err := a.send.Delete(ctx, a.cfg.ChatID, id); err != nil {
		a.log.Warn("delete evening message", "message_id", id, "err", err)
	}
	if err := a.store.SetEveningMessageID(ctx, 0); err != nil {
		a.log.Error("delete evening message", "err", err)
	}
}

// cleanupReminders deletes yesterday's "in N min" reminders.
func (a *App) cleanupReminders(ctx context.Context, today time.Time) {
	ids, err := a.store.ReminderMessages(ctx, today.AddDate(0, 0, -1), today)
	if err != nil {
		a.log.Error("cleanup reminders", "err", err)
		return
	}
	for _, id := range ids {
		if err := a.send.Delete(ctx, a.cfg.ChatID, id); err != nil {
			a.log.Warn("cleanup reminder", "message_id", id, "err", err)
		}
		if err := a.store.ForgetReminderMessage(ctx, id); err != nil {
			a.log.Error("cleanup reminders", "err", err)
		}
	}
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
