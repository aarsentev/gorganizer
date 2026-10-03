package app

import (
	"context"
	"strings"
	"time"
)

// HandleText is called by tg for every message from the allowed chat.
func (a *App) HandleText(ctx context.Context, now time.Time, chatID int64, text string) {
	cmd, args := splitCommand(text)
	switch cmd {
	case "/task":
		a.handleTask(ctx, now, chatID, args)
	case "/tz":
		loc := a.Loc()
		a.replyBlock(ctx, chatID, "tz", struct {
			Zone string
			Now  time.Time
		}{loc.String(), now.In(loc)})
	default:
		// Until notes arrive (stage 3b) plain text is echoed back.
		a.reply(ctx, chatID, text)
	}
}

// splitCommand turns "/task@my_bot buy milk" into ("/task", "buy milk").
// Plain text yields an empty command.
func splitCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", text
	}
	cmd, args, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(args)
}

func (a *App) reply(ctx context.Context, chatID int64, text string) {
	if _, err := a.send.Send(ctx, chatID, text); err != nil {
		a.log.Error("reply", "chat_id", chatID, "err", err)
	}
}
