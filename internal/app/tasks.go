package app

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// Words "/task" understands at the end of the text. Strict syntax: free text is stage 5.
var (
	anchorWords = map[string]string{
		"morning": "morning", "утром": "morning", "утро": "morning",
		"afternoon": "afternoon", "днём": "afternoon", "днем": "afternoon",
		"evening": "evening", "вечером": "evening", "вечер": "evening",
		"eod": "eod",
	}
	tomorrowWords = map[string]bool{"tomorrow": true, "завтра": true}
	clockRe       = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)
)

var errNoTitle = errors.New("no title")

// parseTask splits "/task" arguments into a title and a reminder time.
// Recognized tail: [tomorrow] [anchor | HH:MM] in either order; default is evening,
// so the reminder does not land on the day review at eod.
// A time that has already passed today moves to tomorrow.
func (a *App) parseTask(args string, now time.Time) (string, time.Time, error) {
	words := strings.Fields(args)
	var (
		clock    string
		tomorrow bool
	)
tail:
	for range 2 {
		if len(words) == 0 {
			break
		}
		last := strings.ToLower(words[len(words)-1])
		switch {
		case clock == "" && anchorWords[last] != "":
			clock = a.cfg.Anchors[anchorWords[last]]
		case clock == "" && clockRe.MatchString(last):
			clock = last
		case !tomorrow && tomorrowWords[last]:
			tomorrow = true
		default:
			break tail
		}
		words = words[:len(words)-1]
	}
	if len(words) == 0 {
		return "", time.Time{}, errNoTitle
	}
	if clock == "" {
		clock = a.cfg.Anchors["evening"]
	}

	return capitalize(strings.Join(words, " ")), a.nextAt(now, clock, tomorrow), nil
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

// atClock is the given "H:MM" on the local day; time.Date handles DST gaps.
func atClock(day time.Time, clock string) time.Time {
	m := clockRe.FindStringSubmatch(clock)
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	return time.Date(day.Year(), day.Month(), day.Day(), h, min, 0, 0, day.Location())
}

func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func (a *App) handleTask(ctx context.Context, now time.Time, chatID int64, args string) {
	title, remind, err := a.parseTask(args, now)
	if err != nil {
		a.replyBlock(ctx, chatID, "task_usage", nil)
		return
	}
	id, err := a.store.CreateTask(ctx, title, remind, now)
	if err != nil {
		a.log.Error("create task", "err", err)
		return
	}
	t, err := a.store.Task(ctx, id)
	if err != nil {
		a.log.Error("create task", "err", err)
		return
	}
	text, err := a.render("task_card", a.cardView(t, now))
	if err != nil {
		a.log.Error("render", "err", err)
		return
	}
	if _, err := a.send.SendButtons(ctx, chatID, text, a.cardKeyboard(id)); err != nil {
		a.log.Error("send task card", "err", err)
	}
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

type taskView struct {
	Title   string
	When    whenView
	Checkin time.Time
}

func (a *App) cardView(t store.Task, now time.Time) taskView {
	checkin := atClock(midnight(t.Remind.In(a.Loc())), a.cfg.Anchors["eod"])
	return taskView{Title: t.Title, When: a.when(t.Remind, now), Checkin: checkin}
}

func (a *App) cardKeyboard(id int64) [][]tg.Button {
	return [][]tg.Button{{
		{Text: a.label("btn_ok", nil), Data: buttonData("card", id, "ok")},
		{Text: a.label("btn_time", nil), Data: buttonData("card", id, "time")},
		{Text: a.label("btn_cancel", nil), Data: buttonData("card", id, "cancel")},
	}}
}

func (a *App) timeKeyboard(id int64) [][]tg.Button {
	at := func(anchor string) tg.Button {
		return tg.Button{Text: a.label("btn_"+anchor, nil), Data: buttonData("card", id, "at-"+anchor)}
	}
	return [][]tg.Button{
		{at("morning"), at("afternoon")},
		{at("evening"), at("eod")},
		{{Text: a.label("btn_tomorrow", nil), Data: buttonData("card", id, "tomorrow")}},
	}
}

// taskReminders sends "⏰ title" with buttons for every task whose time has come.
func (a *App) taskReminders(ctx context.Context, now time.Time) {
	tasks, err := a.store.DueTasks(ctx, now)
	if err != nil {
		a.log.Error("task reminders", "err", err)
		return
	}
	for _, t := range tasks {
		text, err := a.render("task_reminder", t)
		if err != nil {
			a.log.Error("render", "err", err)
			continue
		}
		if _, err := a.send.SendButtons(ctx, a.cfg.ChatID, text, a.reminderKeyboard(t.ID, now)); err != nil {
			a.log.Error("task reminder", "task_id", t.ID, "err", err)
			continue // not marked, the next tick retries
		}
		if err := a.store.MarkTaskReminded(ctx, t.ID, t.Remind); err != nil {
			a.log.Error("task reminder", "task_id", t.ID, "err", err)
		}
	}
}

// reminderKeyboard offers "at the end of the day" only while that time is still ahead.
func (a *App) reminderKeyboard(id int64, now time.Time) [][]tg.Button {
	later := tg.Button{Text: a.label("btn_tomorrow", nil), Data: buttonData("rem", id, "tomorrow")}
	eod := atClock(midnight(now.In(a.Loc())), a.cfg.Anchors["eod"])
	if eod.After(now) {
		later = tg.Button{Text: a.label("btn_at", eod), Data: buttonData("rem", id, "eod")}
	}
	return [][]tg.Button{{
		{Text: a.label("btn_done", nil), Data: buttonData("rem", id, "done")},
		{Text: a.label("btn_hour", nil), Data: buttonData("rem", id, "hour")},
		later,
	}}
}

// sameTimeTomorrow keeps the clock time of the task and moves it to the day after now.
func (a *App) sameTimeTomorrow(t store.Task, now time.Time) time.Time {
	tomorrow := midnight(now.In(a.Loc())).AddDate(0, 0, 1)
	return atClock(tomorrow, t.Remind.In(a.Loc()).Format("15:04"))
}
