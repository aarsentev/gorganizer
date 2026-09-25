package app

import (
	"context"
	"fmt"
	"time"
)

// HandleText is called by tg for every message from the allowed chat.
func (a *App) HandleText(ctx context.Context, now time.Time, chatID int64, text string) {
	var reply string
	switch text {
	case "/tz":
		loc := a.Loc()
		reply = fmt.Sprintf("%s, Now %s", loc, now.In(loc).Format("15:04"))
	default:
		reply = text
	}
	a.reply(ctx, chatID, reply)
}

func (a *App) reply(ctx context.Context, chatID int64, text string) {
	if err := a.send.Send(ctx, chatID, text); err != nil {
		a.log.Error("reply", "chat_id", chatID, "err", err)
	}
}
