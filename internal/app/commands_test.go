package app

import (
	"context"
	"testing"

	"go-organizer/internal/tg"
)

func TestHelpAndMenu(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/start"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/help"})

	help := "/add — Событие: /add 30.09 9:00-10:00 Приём\n" +
		"/task — Задача: /task забрать запчасть вечером\n" +
		"/note — Заметка: мысль, факт, ссылка\n" +
		"/inbox — Ящик заметок\n" +
		"/tz — Часовой пояс: /tz toronto\n" +
		"/pause — Пауза: /pause 3d, /pause off\n" +
		"/lang — Язык: /lang en\n\n" +
		"Просто текст — спрошу, задача это или заметка."
	if !sameStrings(send.sent, []string{help, help}) {
		t.Fatalf("sent %q, want the help twice", send.sent)
	}

	a.PublishCommands(ctx)
	if len(send.menus) != 1 || len(send.menus[0]) != len(menu) ||
		send.menus[0][0] != (tg.Command{Name: "add", Description: "Событие: /add 30.09 9:00-10:00 Приём"}) {
		t.Fatalf("menus = %v", send.menus)
	}
}

func TestLang(t *testing.T) {
	ctx := context.Background()
	a, send, _ := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")

	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/lang"})
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/lang fr"}) // planned, but no messages file yet
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/lang EN"})

	want := []string{
		"Язык: ru. Доступно: en, ru",
		"Нет языка «fr». Доступно: en, ru",
		"Language: en. Available: en, ru",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent %q, want %q", send.sent, want)
	}
	if a.Lang() != "en" {
		t.Fatalf("Lang = %s", a.Lang())
	}
	// The menu follows the language, and only an actual switch republishes it.
	if len(send.menus) != 1 || send.menus[0][0].Description != "Event: /add 30.09 9:00-10:00 Doctor" {
		t.Fatalf("menus = %v", send.menus)
	}
}
