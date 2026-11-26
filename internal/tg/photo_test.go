package tg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
)

type photoHTTPClient func(*http.Request) (*http.Response, error)

func (f photoHTTPClient) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendPNG(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, caption, response string
		id                      int64
		err                     error
	}{
		{"caption", "Mood · 2026", `{"ok":true,"result":{"message_id":777}}`, 777, nil},
		{"no caption", "", `{"ok":true,"result":{"message_id":778}}`, 778, nil},
		{"rejected", "Mood", `{"ok":false,"error_code":400,"description":"Bad Request: invalid photo"}`, 0, bot.ErrorBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := photoHTTPClient(func(r *http.Request) (*http.Response, error) {
				defer r.Body.Close()
				if r.Method != http.MethodPost || r.URL.Path != "/bot123:test/sendPhoto" {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Fatal(err)
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("chat_id") != "42" || r.FormValue("caption") != tc.caption {
					t.Fatalf("unexpected fields: %v", r.MultipartForm.Value)
				}
				file, header, err := r.FormFile("photo")
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				got, err := io.ReadAll(file)
				if err != nil || !bytes.Equal(got, data.Bytes()) || header.Filename != "image.png" {
					t.Fatalf("PNG upload differs: filename=%q, err=%v", header.Filename, err)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})
			b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithHTTPClient(0, transport))
			if err != nil {
				t.Fatal(err)
			}
			c := &Client{bot: b}
			id, err := c.SendPNG(context.Background(), 42, data.Bytes(), tc.caption)
			if id != tc.id || !errors.Is(err, tc.err) {
				t.Fatalf("SendPNG = (%d, %v), want (%d, %v)", id, err, tc.id, tc.err)
			}
		})
	}
}
