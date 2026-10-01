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

// Send returns the Telegram id of the new message, needed to delete it later.
func (c *Client) Send(ctx context.Context, chatID int64, text string) (int64, error) {
	m, err := c.bot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	if err != nil {
		return 0, fmt.Errorf("send to %d: %w", chatID, err)
	}
	return int64(m.ID), nil
}

// Delete removes a message the bot sent. Telegram allows it only within 48 hours.
func (c *Client) Delete(ctx context.Context, chatID, messageID int64) error {
	_, err := c.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: int(messageID)})
	if err != nil {
		return fmt.Errorf("delete %d in %d: %w", messageID, chatID, err)
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
