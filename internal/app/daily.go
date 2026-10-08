package app

import (
	"context"
	"time"

	"go-organizer/internal/store"
)

const (
	jobDigest  = "digest"
	jobEvening = "evening"
	jobCheckin = "checkin"
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
	// Without a calendar the evening message would say "nothing tomorrow" every day.
	if a.cal != nil && m >= eveningAt {
		a.fire(ctx, jobEvening, local, now, a.evening)
	}
	if m >= clockMinutes(a.cfg.Anchors["eod"]) {
		a.fire(ctx, jobCheckin, local, now, a.checkin)
	}
}

// dailyJob does the job for the local day and returns what it did as log key-value pairs.
type dailyJob func(ctx context.Context, local time.Time) (details []any, err error)

func (a *App) fire(ctx context.Context, job string, local, now time.Time, run dailyJob) {
	day := local.Format(time.DateOnly)
	ok, err := a.store.TryFire(ctx, job, day, now)
	if err != nil {
		a.log.Error("daily job", "job", job, "err", err)
		return
	}
	if !ok {
		return
	}
	details, err := run(ctx, local)
	if err != nil {
		a.log.Error("daily job", "job", job, "err", err)
		// Let the next tick try again instead of losing the job for today.
		if err := a.store.Unfire(ctx, job, day); err != nil {
			a.log.Error("daily job", "job", job, "err", err)
		}
		return
	}
	// One line per job and day, also when there was nothing to send:
	// a silent day review shows up in the log as tasks=0, not as nothing.
	a.log.Info("daily job", append([]any{"job", job, "day", day}, details...)...)
}

// evening sends tomorrow's events. Its id is kept so the morning digest can delete it.
func (a *App) evening(ctx context.Context, local time.Time) ([]any, error) {
	tomorrow := midnight(local).AddDate(0, 0, 1)
	events, err := a.dayEvents(ctx, tomorrow)
	if err != nil {
		return nil, err
	}
	block := "evening"
	if len(events) == 0 {
		block = "evening_empty"
	}
	text, err := a.render(block, struct {
		Day    time.Time
		Events []eventView
	}{tomorrow, events})
	if err != nil {
		return nil, err
	}
	id, err := a.send.Send(ctx, a.cfg.ChatID, text)
	if err != nil {
		return nil, err
	}
	// The morning never ran (downtime), so yesterday's evening message is still there.
	a.deleteEvening(ctx)
	return []any{"events", len(events)}, a.store.SetEveningMessageID(ctx, id)
}

// digest sends today's events and open tasks, then removes the evening message
// it replaces. Send first: if sending fails, last evening's message stays.
func (a *App) digest(ctx context.Context, local time.Time) ([]any, error) {
	today := midnight(local)
	// Without a calendar the events are unknown, not absent: the digest shows tasks only.
	var events []eventView
	if a.cal != nil {
		var err error
		if events, err = a.dayEvents(ctx, today); err != nil {
			return nil, err
		}
	}
	tasks, err := a.store.OpenTasksBefore(ctx, today.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	text, err := a.render("digest", struct {
		Day        time.Time
		Events     []eventView
		Tasks      []store.Task
		NoCalendar bool
	}{today, events, tasks, a.cal == nil})
	if err != nil {
		return nil, err
	}
	if _, err := a.send.Send(ctx, a.cfg.ChatID, text); err != nil {
		return nil, err
	}
	a.deleteEvening(ctx)
	if a.cfg.Reminders.Cleanup {
		a.cleanupReminders(ctx, today)
	}
	return []any{"events", len(events), "tasks", len(tasks)}, nil
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
