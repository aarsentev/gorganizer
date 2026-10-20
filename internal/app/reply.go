package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go-organizer/internal/tg"
)

// Sending and editing messages, and the data of their buttons: shared by every feature.

func (a *App) reply(ctx context.Context, chatID int64, text string) {
	if _, err := a.send.Send(ctx, chatID, text); err != nil {
		a.log.Error("reply", "chat_id", chatID, "err", err)
	}
}

func (a *App) replyBlock(ctx context.Context, chatID int64, block string, data any) {
	text, err := a.render(block, data)
	if err != nil {
		a.log.Error("render", "err", err)
		return
	}
	a.reply(ctx, chatID, text)
}

func (a *App) editBlock(ctx context.Context, cb tg.Callback, block string, data any, keyboard [][]tg.Button) error {
	text, err := a.render(block, data)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}

// buttonData builds "<kind>:<id>:<action>": Telegram cuts button data at 64 bytes,
// so a button carries only a reference, never the content.
func buttonData(kind string, id int64, action string) string {
	return fmt.Sprintf("%s:%d:%s", kind, id, action)
}

// parseData splits data built by buttonData.
func parseData(data string) (kind string, id int64, action string, ok bool) {
	parts := strings.SplitN(data, ":", 3)
	if len(parts) != 3 {
		return "", 0, "", false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, "", false
	}
	return parts[0], id, parts[2], true
}
