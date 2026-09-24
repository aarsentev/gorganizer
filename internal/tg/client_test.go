package tg

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestWhitelist(t *testing.T) {
	var got []Message
	c := &Client{
		allowed: 42,
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		handler: func(_ context.Context, m Message) { got = append(got, m) },
	}

	updates := []*models.Update{
		{Message: &models.Message{Chat: models.Chat{ID: 42}, Text: "hi"}},
		{Message: &models.Message{Chat: models.Chat{ID: 7}, Text: "intruder"}},
		{Message: &models.Message{Chat: models.Chat{ID: 42}}}, // sticker, no text
		{}, // not a message
	}
	for _, u := range updates {
		c.onUpdate(context.Background(), nil, u)
	}

	if len(got) != 1 || got[0].Text != "hi" {
		t.Fatalf("handled %+v, want only the allowed text message", got)
	}
}
