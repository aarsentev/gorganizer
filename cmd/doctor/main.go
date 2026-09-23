package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	_ "time/tzdata"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func main() {
	ctx := context.Background()
	sa := option.WithAuthCredentialsFile(option.ServiceAccount, env("GOOGLE_SA_FILE"))
	calID := env("CALENDAR_ID")

	cal, err := calendar.NewService(ctx, sa)
	if err != nil {
		log.Fatal("calendar service: ", err)
	}

	now := time.Now()
	list, err := cal.Events.List(calID).
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
	created, err := cal.Events.Insert(calID, probe).Do()
	if err != nil {
		log.Fatal("Write calendar (403 — need to extend rights): ", err)
	}
	if err := cal.Events.Delete(calID, created.Id).Do(); err != nil {
		log.Fatal("Delete probe: ", err)
	}
	fmt.Println("2. Calendar write ok")

	resp, err := http.PostForm(
		"https://api.telegram.org/bot"+env("TELEGRAM_TOKEN")+"/sendMessage",
		url.Values{"chat_id": {env("TELEGRAM_CHAT_ID")}, "text": {"doctor ok"}})
	if err != nil {
		log.Fatal("Telegram: ", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatal("Telegram status: ", resp.Status)
	}
	fmt.Println("3. Telegram ok")

	tz := envOr("TZ", "Europe/Warsaw")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Fatal("tz: ", err)
	}
	fmt.Printf("4. TimeZone ok: %s, now %s\n", tz, time.Now().In(loc).Format("15:04"))

	fmt.Printf("5. ENV ok: go %s\n", os.Getenv("GOVERSION"))
}

func env(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatal("missing env: ", k)
	}
	return v
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
