package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

// fakeSender records what the bot would do in Telegram, in call order.
type fakeSender struct {
	sent      []string
	keyboards [][][]tg.Button // keyboard of each sent message, nil for plain ones
	edits     []edit
	deleted   []int64
	ops       []string // "send:<id>", "edit:<id>" and "delete:<id>"
	failSends int      // the next N sends fail, as if Telegram were down
}

type edit struct {
	messageID int64
	text      string
	keyboard  [][]tg.Button
}

func (f *fakeSender) Send(ctx context.Context, chatID int64, text string) (int64, error) {
	return f.SendButtons(ctx, chatID, text, nil)
}

func (f *fakeSender) SendButtons(_ context.Context, _ int64, text string, keyboard [][]tg.Button) (int64, error) {
	if f.failSends > 0 {
		f.failSends--
		return 0, errors.New("telegram is down")
	}
	f.sent = append(f.sent, text)
	f.keyboards = append(f.keyboards, keyboard)
	id := int64(len(f.sent))
	f.ops = append(f.ops, fmt.Sprintf("send:%d", id))
	return id, nil
}

func (f *fakeSender) Edit(_ context.Context, _, messageID int64, text string, keyboard [][]tg.Button) error {
	f.edits = append(f.edits, edit{messageID, text, keyboard})
	f.ops = append(f.ops, fmt.Sprintf("edit:%d", messageID))
	return nil
}

func (f *fakeSender) Delete(_ context.Context, _, messageID int64) error {
	f.deleted = append(f.deleted, messageID)
	f.ops = append(f.ops, fmt.Sprintf("delete:%d", messageID))
	return nil
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func testConfig() *config.Config {
	c := &config.Config{TZ: "Europe/Warsaw", Lang: "ru", ChatID: 42}
	c.QuietHours.From, c.QuietHours.To = "00:00", "07:00"
	c.Reminders.EventOffsetsMin = []int{60}
	c.Reminders.EveningAt = "20:00"
	c.Digest.At = "07:30"
	c.Anchors = map[string]string{"morning": "09:00", "afternoon": "14:00", "evening": "18:00", "eod": "21:00"}
	return c
}

var warsaw, _ = time.LoadLocation("Europe/Warsaw")

// at parses "2006-01-02 15:04" as Warsaw local time.
func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02 15:04", s, warsaw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// newTestApp builds an app on an in-memory store; mutate adjusts the config.
func newTestApp(t *testing.T, cal Calendar, mutate func(*config.Config)) (*App, *fakeSender, *store.Store) {
	t.Helper()
	cfg := testConfig()
	if mutate != nil {
		mutate(cfg)
	}
	st := openStore(t)
	send := &fakeSender{}
	a, err := New(context.Background(), st, send, cal, cfg, discard)
	if err != nil {
		t.Fatal(err)
	}
	return a, send, st
}

func put(t *testing.T, st *store.Store, events ...store.Event) {
	t.Helper()
	if err := st.UpsertEvents(context.Background(), events); err != nil {
		t.Fatal(err)
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestTZSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)

	a, err := New(ctx, st, &fakeSender{}, nil, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Loc().String(); got != "Europe/Warsaw" {
		t.Fatalf("First run Loc = %s, want config default", got)
	}
	if err := a.SetTZ(ctx, "America/Toronto"); err != nil {
		t.Fatal(err)
	}

	// Restart with the same database: state wins over config.
	a, err = New(ctx, st, &fakeSender{}, nil, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Loc().String(); got != "America/Toronto" {
		t.Fatalf("After restart Loc = %s, want America/Toronto", got)
	}
}

func TestSetTZRejectsUnknownZone(t *testing.T) {
	ctx := context.Background()
	a, err := New(ctx, openStore(t), &fakeSender{}, nil, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetTZ(ctx, "Mars/Olympus"); err == nil {
		t.Fatal("SetTZ accepted an unknown zone")
	}
	if got := a.Loc().String(); got != "Europe/Warsaw" {
		t.Fatalf("Loc = %s after failed SetTZ, want unchanged", got)
	}
}

func TestHandleText(t *testing.T) {
	ctx := context.Background()
	send := &fakeSender{}
	a, err := New(ctx, openStore(t), send, nil, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 7, 5, 0, 0, time.UTC) // 09:05 in Warsaw

	a.HandleText(ctx, now, 1, "hello")
	a.HandleText(ctx, now, 1, "/tz")

	want := []string{"hello", "Europe/Warsaw, сейчас 09:05"}
	if len(send.sent) != len(want) {
		t.Fatalf("sent %q, want %q", send.sent, want)
	}
	for i := range want {
		if send.sent[i] != want[i] {
			t.Fatalf("sent[%d] = %q, want %q", i, send.sent[i], want[i])
		}
	}
}

func TestLangSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)

	a, err := New(ctx, st, &fakeSender{}, nil, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if a.Lang() != "ru" {
		t.Fatalf("first run Lang = %s, want config default", a.Lang())
	}
	if err := a.SetLang(ctx, "es"); err == nil {
		t.Fatal("SetLang accepted an unsupported language")
	}
	if err := a.SetLang(ctx, "pl"); err != nil {
		t.Fatal(err)
	}

	a, err = New(ctx, st, &fakeSender{}, nil, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if a.Lang() != "pl" {
		t.Fatalf("after restart Lang = %s, want pl", a.Lang())
	}
}
