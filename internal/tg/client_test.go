package tg

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
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

// fakeTelegram answers Bot API calls the way Telegram does and records them.
func fakeTelegram(t *testing.T, calls *[]string, bodies *[]map[string]any) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		*calls = append(*calls, method)
		body := map[string]any{}
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			for k, v := range r.MultipartForm.Value {
				body[k] = v[0]
			}
		}
		*bodies = append(*bodies, body)

		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "sendMessage":
			w.Write([]byte(`{"ok":true,"result":{"message_id":777,"date":0,"chat":{"id":42,"type":"private"}}}`))
		case "deleteMessage":
			if body["message_id"] == "404" {
				w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message to delete not found"}`))
				return
			}
			w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			t.Errorf("unexpected method %s", method)
		}
	}))
	t.Cleanup(srv.Close)

	b, err := bot.New("123:test", bot.WithServerURL(srv.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	return &Client{bot: b, allowed: 42, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestSendReturnsMessageID(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	c := fakeTelegram(t, &calls, &bodies)

	id, err := c.Send(context.Background(), 42, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if id != 777 {
		t.Fatalf("message id = %d, want 777", id)
	}
	if bodies[0]["chat_id"] != "42" || bodies[0]["text"] != "hello" {
		t.Fatalf("sendMessage body = %v", bodies[0])
	}
}

func TestDelete(t *testing.T) {
	var calls []string
	var bodies []map[string]any
	c := fakeTelegram(t, &calls, &bodies)
	ctx := context.Background()

	if err := c.Delete(ctx, 42, 777); err != nil {
		t.Fatal(err)
	}
	if calls[0] != "deleteMessage" || bodies[0]["chat_id"] != "42" || bodies[0]["message_id"] != "777" {
		t.Fatalf("call = %s %v", calls[0], bodies[0])
	}

	if err := c.Delete(ctx, 42, 404); err == nil {
		t.Fatal("deleting a missing message must return an error")
	}
}
