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

// AnnounceBuild tells the chat that a new build is running and whether its parts work,
// like doctor but reading only. A restart of the same build stays silent: the build
// time it started with last is kept in state.
func (a *App) AnnounceBuild(ctx context.Context, version, built string) {
	last, err := a.store.LastBuild(ctx)
	if err != nil {
		a.log.Error("announce build", "err", err)
		return
	}
	if last == built {
		return
	}

	view := buildView{Version: version}
	// make image writes the time as 2026-10-24T10:20Z.
	if t, err := time.Parse("2006-01-02T15:04Z07:00", built); err == nil {
		view.Built = t.In(a.Loc())
	}
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
	if err := a.store.SetLastBuild(ctx, built); err != nil {
		a.log.Error("announce build", "err", err)
	}
}
