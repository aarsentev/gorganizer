package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"time"

	_ "time/tzdata"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	_ "modernc.org/sqlite"

	"go-organizer/internal/config"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatal("config: ", err)
	}

	cal, err := calendar.NewService(ctx,
		option.WithAuthCredentialsFile(option.ServiceAccount, cfg.SAFile),
		option.WithScopes(calendar.CalendarEventsScope))
	if err != nil {
		log.Fatal("calendar service: ", err)
	}

	now := time.Now()
	list, err := cal.Events.List(cfg.CalendarID).
		TimeMin(now.Format(time.RFC3339)).
		TimeMax(now.AddDate(0, 0, 7).Format(time.RFC3339)).
		SingleEvents(true).OrderBy("startTime").Do()
	if err != nil {
		log.Fatal("read calendar (404 — вероятно, в CALENDAR_ID email service account'а): ", err)
	}
	for _, e := range list.Items {
		start := e.Start.DateTime
		if start == "" {
			start = e.Start.Date
		}
		fmt.Printf("    %s  %s\n", start, e.Summary)
	}
	fmt.Printf("1. Calendar read ok: %d events\n", len(list.Items))

	probe := &calendar.Event{
		Summary: "Organizer doctor",
		Start:   &calendar.EventDateTime{DateTime: now.Add(time.Hour).Format(time.RFC3339)},
		End:     &calendar.EventDateTime{DateTime: now.Add(2 * time.Hour).Format(time.RFC3339)},
	}
	created, err := cal.Events.Insert(cfg.CalendarID, probe).Do()
	if err != nil {
		log.Fatal("Write calendar (403 — need to extend rights): ", err)
	}
	if err := cal.Events.Delete(cfg.CalendarID, created.Id).Do(); err != nil {
		log.Fatal("Delete probe: ", err)
	}
	fmt.Println("2. Calendar write ok")

	resp, err := http.PostForm(
		"https://api.telegram.org/bot"+cfg.BotToken+"/sendMessage",
		url.Values{"chat_id": {strconv.FormatInt(cfg.ChatID, 10)}, "text": {"doctor ok"}})
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		log.Fatal("Telegram: ", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatal("Telegram status: ", resp.Status)
	}
	fmt.Println("3. Telegram ok")

	loc, err := time.LoadLocation(cfg.TZ)
	if err != nil {
		log.Fatal("tz: ", err)
	}
	fmt.Printf("4. TimeZone ok: %s, now %s\n", cfg.TZ, now.In(loc).Format("15:04"))

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatal("sqlite open: ", err)
	}
	defer db.Close()
	var ver string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&ver); err != nil {
		log.Fatal("sqlite: ", err)
	}
	fmt.Printf("5. SQLite ok: %s %s, %s\n", cfg.DBPath, ver, runtime.Version())
}
