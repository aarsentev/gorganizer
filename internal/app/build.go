package app

import (
	"context"
	"time"
)

// checkTimeout keeps a hanging Google from holding up the start of the bot.
const checkTimeout = 10 * time.Second

type buildView struct {
	Version     string
	Built       time.Time // in the user's zone; zero if the build time could not be read
	Schema      int       // the last applied migration
	DBErr       string
	NoCalendar  bool
	CalendarErr string
}

type statusView struct {
	Version string
	Built   time.Time // in the user's zone; zero if the build time could not be read
	TZ      tzView
	Paused  time.Time // the end of the pause in the user's zone; zero if there is none
}

// SetBuild tells the app which commit it runs and when that was built.
func (a *App) SetBuild(version, built string) {
	a.version, a.built = version, built
}

// builtAt is the build time in the user's zone, zero if it could not be read: go run has none.
func (a *App) builtAt() time.Time {
	// make image writes the time as 2026-10-24T10:20Z.
	t, err := time.Parse("2006-01-02T15:04Z07:00", a.built)
	if err != nil {
		return time.Time{}
	}
	return t.In(a.Loc())
}

// handleStatus answers what runs and how it is set: the version, the zone, the pause.
func (a *App) handleStatus(ctx context.Context, now time.Time, chatID int64) {
	until, err := a.store.PausedUntil(ctx)
	if err != nil {
		a.log.Error("status", "err", err)
		return
	}
	loc := a.Loc()
	view := statusView{Version: a.version, Built: a.builtAt(), TZ: tzView{Zone: loc.String(), Now: now.In(loc)}}
	if now.Before(until) {
		view.Paused = until.In(loc)
	}
	a.replyBlock(ctx, chatID, "status", view)
}

// AnnounceBuild tells the chat that a new build is running and whether its parts work,
// like doctor but reading only. A restart of the same build stays silent: the build
// time it started with last is kept in state.
func (a *App) AnnounceBuild(ctx context.Context) {
	last, err := a.store.LastBuild(ctx)
	if err != nil {
		a.log.Error("announce build", "err", err)
		return
	}
	if last == a.built {
		return
	}

	view := buildView{Version: a.version, Built: a.builtAt()}
	if view.Schema, err = a.store.SchemaVersion(ctx); err != nil {
		view.DBErr = shorten(err.Error(), 100)
	}
	if a.cal == nil {
		view.NoCalendar = true
	} else {
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		if err := a.cal.Check(checkCtx); err != nil {
			view.CalendarErr = shorten(err.Error(), 100)
		}
		cancel()
	}

	text, err := a.render("build", view)
	if err != nil {
		a.log.Error("render", "err", err)
		return
	}
	// Not remembered if not sent: the next start tries again.
	if _, err := a.send.Send(ctx, a.cfg.ChatID, text); err != nil {
		a.log.Error("announce build", "err", err)
		return
	}
	if err := a.store.SetLastBuild(ctx, a.built); err != nil {
		a.log.Error("announce build", "err", err)
	}
}
