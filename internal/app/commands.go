package app

import (
	"context"
	"slices"
	"strings"

	"go-organizer/internal/tg"
)

// menu is the Telegram command menu, in order. Descriptions are the cmd_<name>
// blocks of the messages, so the menu changes language with the bot.
var menu = []string{"task", "note", "inbox", "tz", "lang"}

type langView struct {
	Lang      string
	Available []string
}

// PublishCommands sets the command menu in the current language. A failure is only
// logged: the bot works without the menu, the commands can still be typed.
func (a *App) PublishCommands(ctx context.Context) {
	if err := a.send.SetCommands(ctx, a.commands()); err != nil {
		a.log.Warn("publish commands", "err", err)
	}
}

func (a *App) commands() []tg.Command {
	out := make([]tg.Command, len(menu))
	for i, name := range menu {
		out[i] = tg.Command{Name: name, Description: a.label("cmd_"+name, nil)}
	}
	return out
}

// handleHelp lists the same commands as the menu. Telegram sends /start
// when the bot is opened for the first time.
func (a *App) handleHelp(ctx context.Context, chatID int64) {
	a.replyBlock(ctx, chatID, "help", a.commands())
}

// handleLang shows the language or switches it, and the menu with it.
func (a *App) handleLang(ctx context.Context, chatID int64, args string) {
	view := langView{Available: a.texts.languages()}
	if args != "" {
		lang := strings.ToLower(args)
		if !slices.Contains(view.Available, lang) {
			view.Lang = args
			a.replyBlock(ctx, chatID, "lang_unknown", view)
			return
		}
		if err := a.SetLang(ctx, lang); err != nil {
			a.log.Error("set lang", "err", err)
			return
		}
		a.PublishCommands(ctx)
	}
	view.Lang = a.Lang()
	a.replyBlock(ctx, chatID, "lang", view)
}
