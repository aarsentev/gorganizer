package app

import (
	"context"
	"time"
)

// HandleText is called by tg for every message from the allowed chat.
func (a *App) HandleText(ctx context.Context, now time.Time, chatID int64, text string) {
	reply := text
	if text == "/tz" {
		loc := a.Loc()
		var err error
		reply, err = a.render("tz", struct {
			Zone string
			Now  time.Time
		}{loc.String(), now.In(loc)})
		if err != nil {
			a.log.Error("render", "err", err)
			return
		}
	}
	a.reply(ctx, chatID, reply)
}

func (a *App) reply(ctx context.Context, chatID int64, text string) {
	if _, err := a.send.Send(ctx, chatID, text); err != nil {
		a.log.Error("reply", "chat_id", chatID, "err", err)
	}
}
