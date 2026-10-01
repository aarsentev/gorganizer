package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/store"
)

type fakeSender struct {
	sent    []string
	deleted []int64
}

func (f *fakeSender) Send(_ context.Context, _ int64, text string) (int64, error) {
	f.sent = append(f.sent, text)
	return int64(len(f.sent)), nil
}

func (f *fakeSender) Delete(_ context.Context, _, messageID int64) error {
	f.deleted = append(f.deleted, messageID)
	return nil
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func testConfig() *config.Config {
	return &config.Config{TZ: "Europe/Warsaw", Lang: "ru"}
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

	a, err := New(ctx, st, &fakeSender{}, testConfig(), discard)
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
	a, err = New(ctx, st, &fakeSender{}, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Loc().String(); got != "America/Toronto" {
		t.Fatalf("After restart Loc = %s, want America/Toronto", got)
	}
}

func TestSetTZRejectsUnknownZone(t *testing.T) {
	ctx := context.Background()
	a, err := New(ctx, openStore(t), &fakeSender{}, testConfig(), discard)
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
	a, err := New(ctx, openStore(t), send, testConfig(), discard)
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

	a, err := New(ctx, st, &fakeSender{}, testConfig(), discard)
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

	a, err = New(ctx, st, &fakeSender{}, testConfig(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if a.Lang() != "pl" {
		t.Fatalf("after restart Lang = %s, want pl", a.Lang())
	}
}
