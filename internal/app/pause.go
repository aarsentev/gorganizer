package app

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxPause keeps a typo like /pause 3000d from silencing the bot for years.
const maxPause = 365 * 24 * time.Hour

// pauseRe is "3d" or "12h"; Russian д and ч work too. Four digits at most:
// a longer number would overflow time.Duration before the maxPause check.
var pauseRe = regexp.MustCompile(`^(\d{1,4})([dhдч])$`)

// handlePause sets, lifts or shows the pause. While paused, Tick sends nothing;
// commands and buttons keep working.
func (a *App) handlePause(ctx context.Context, now time.Time, chatID int64, args string) {
	switch {
	case args == "":
		// Only show the state below.
	case strings.EqualFold(args, "off"):
		if err := a.store.SetPausedUntil(ctx, time.Time{}); err != nil {
			a.log.Error("lift pause", "err", err)
			return
		}
	default:
		d, ok := parsePause(args)
		if !ok {
			a.replyBlock(ctx, chatID, "pause_usage", nil)
			return
		}
		if err := a.store.SetPausedUntil(ctx, now.Add(d)); err != nil {
			a.log.Error("set pause", "err", err)
			return
		}
	}

	until, err := a.store.PausedUntil(ctx)
	if err != nil {
		a.log.Error("pause", "err", err)
		return
	}
	if !now.Before(until) {
		a.replyBlock(ctx, chatID, "pause_off", nil)
		return
	}
	a.replyBlock(ctx, chatID, "pause_on", until.In(a.Loc()))
}

// parsePause turns "3d" or "12h" into a duration of at most maxPause.
func parsePause(s string) (time.Duration, bool) {
	m := pauseRe.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1]) // the regexp lets only digits through
	unit := time.Hour
	if m[2] == "d" || m[2] == "д" {
		unit = 24 * time.Hour
	}
	d := time.Duration(n) * unit
	if d == 0 || d > maxPause {
		return 0, false
	}
	return d, true
}

// paused reports whether a pause is on. If the state cannot be read, the bot
// rather speaks than stays silent by mistake.
func (a *App) paused(ctx context.Context, now time.Time) bool {
	until, err := a.store.PausedUntil(ctx)
	if err != nil {
		a.log.Error("pause", "err", err)
		return false
	}
	return now.Before(until)
}
