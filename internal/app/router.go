package app

import (
	"context"
	"strings"
	"time"

	"go-organizer/internal/tg"
)

// HandleText is called by tg for every message from the allowed chat.
func (a *App) HandleText(ctx context.Context, now time.Time, m tg.Message) {
	cmd, args := splitCommand(m.Text)
	switch cmd {
	case "":
		if args != "" {
			a.askKind(ctx, m)
		}
	case "/task":
		a.handleTask(ctx, now, m.ChatID, args)
	case "/note":
		a.handleNote(ctx, now, m.ChatID, args)
	case "/inbox":
		a.handleInbox(ctx, m.ChatID)
	case "/tz":
		a.handleTZ(ctx, now, m.ChatID, args)
	default:
		a.replyBlock(ctx, m.ChatID, "unknown_command", cmd)
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
