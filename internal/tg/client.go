package tg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Message is an incoming text message from the allowed chat.
type Message struct {
	ChatID int64
	Text   string
}

type Handler func(ctx context.Context, m Message)

type Client struct {
	bot     *bot.Bot
	allowed int64
	log     *slog.Logger
	handler Handler
}

// New validates the token with getMe. Only messages from allowedChatID reach the handler.
func New(token string, allowedChatID int64, log *slog.Logger) (*Client, error) {
	c := &Client{allowed: allowedChatID, log: log}
	b, err := bot.New(token,
		bot.WithDefaultHandler(c.onUpdate),
		bot.WithErrorsHandler(func(err error) { log.Error("telegram", "err", err) }),
		// One worker and synchronous handlers: updates are processed strictly in order,
		// so a reply to a question never overtakes the question's own message.
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	c.bot = b
	return c, nil
}

// Run polls for updates until ctx is cancelled.
func (c *Client) Run(ctx context.Context, h Handler) error {
	c.handler = h
	c.bot.Start(ctx)
	return nil
}

func (c *Client) Send(ctx context.Context, chatID int64, text string) error {
	_, err := c.bot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	if err != nil {
		return fmt.Errorf("send to %d: %w", chatID, err)
	}
	return nil
}

func (c *Client) onUpdate(ctx context.Context, _ *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil {
		return
	}
	if m.Chat.ID != c.allowed {
		username := ""
		if m.From != nil {
			username = m.From.Username
		}
		c.log.Warn("Message from unknown chat", "chat_id", m.Chat.ID, "username", username)
		return
	}
	if m.Text == "" {
		return
	}
	c.handler(ctx, Message{ChatID: m.Chat.ID, Text: m.Text})
}
