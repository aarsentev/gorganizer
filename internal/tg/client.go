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

// Button is an inline button. Data must fit Telegram's 64 bytes, so it carries
// only a reference like "task:17:done", never the content itself.
type Button struct {
	Text string
	Data string
}

// Callback is a button press in the allowed chat. Keyboard is the message's
// current buttons, so the handler can redraw them without storing anything.
type Callback struct {
	ChatID    int64
	MessageID int64
	Data      string
	Keyboard  [][]Button
}

type Handlers struct {
	Text     func(ctx context.Context, m Message)
	Callback func(ctx context.Context, cb Callback)
}

type Client struct {
	bot      *bot.Bot
	allowed  int64
	log      *slog.Logger
	handlers Handlers
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
func (c *Client) Run(ctx context.Context, h Handlers) error {
	c.handlers = h
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

// SendButtons sends a message with inline buttons, one slice per row.
func (c *Client) SendButtons(ctx context.Context, chatID int64, text string, keyboard [][]Button) (int64, error) {
	p := &bot.SendMessageParams{ChatID: chatID, Text: text}
	if len(keyboard) > 0 {
		p.ReplyMarkup = markup(keyboard)
	}
	m, err := c.bot.SendMessage(ctx, p)
	if err != nil {
		return 0, fmt.Errorf("send to %d: %w", chatID, err)
	}
	return int64(m.ID), nil
}

// Edit replaces the text and buttons of a sent message; an empty keyboard removes the buttons.
func (c *Client) Edit(ctx context.Context, chatID, messageID int64, text string, keyboard [][]Button) error {
	p := &bot.EditMessageTextParams{ChatID: chatID, MessageID: int(messageID), Text: text}
	if len(keyboard) > 0 {
		p.ReplyMarkup = markup(keyboard)
	}
	if _, err := c.bot.EditMessageText(ctx, p); err != nil {
		return fmt.Errorf("edit %d in %d: %w", messageID, chatID, err)
	}
	return nil
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
	switch {
	case u.Message != nil:
		c.onMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		c.onCallback(ctx, u.CallbackQuery)
	}
}

func (c *Client) onMessage(ctx context.Context, m *models.Message) {
	if m.Chat.ID != c.allowed {
		username := ""
		if m.From != nil {
			username = m.From.Username
		}
		c.log.Warn("Message from unknown chat", "chat_id", m.Chat.ID, "username", username)
		return
	}
	if m.Text == "" || c.handlers.Text == nil {
		return
	}
	c.handlers.Text(ctx, Message{ChatID: m.Chat.ID, Text: m.Text})
}

func (c *Client) onCallback(ctx context.Context, q *models.CallbackQuery) {
	// Buttons can only be pressed under the bot's own messages in a chat that already
	// passed the whitelist, but a forwarded message must not let anyone else in.
	if q.From.ID != c.allowed {
		c.log.Warn("Button press from unknown user", "user_id", q.From.ID, "username", q.From.Username)
		return
	}
	// Answer right away so Telegram stops the spinner on the button.
	if _, err := c.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		c.log.Warn("answer callback", "err", err)
	}

	m := q.Message.Message
	if m == nil || c.handlers.Callback == nil {
		return // the message was deleted or is no longer accessible to the bot
	}
	cb := Callback{ChatID: m.Chat.ID, MessageID: int64(m.ID), Data: q.Data}
	if m.ReplyMarkup != nil {
		for _, row := range m.ReplyMarkup.InlineKeyboard {
			var out []Button
			for _, b := range row {
				out = append(out, Button{Text: b.Text, Data: b.CallbackData})
			}
			cb.Keyboard = append(cb.Keyboard, out)
		}
	}
	c.handlers.Callback(ctx, cb)
}

func markup(keyboard [][]Button) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, len(keyboard))
	for i, row := range keyboard {
		for _, b := range row {
			rows[i] = append(rows[i], models.InlineKeyboardButton{Text: b.Text, CallbackData: b.Data})
		}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
