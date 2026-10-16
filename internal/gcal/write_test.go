package gcal

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
)

func newEvent(t *testing.T) NewEvent {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	return NewEvent{
		ID:       "org7a1760000000",
		Title:    "Doctor",
		Start:    time.Date(2026, 9, 29, 9, 0, 0, 0, loc),
		End:      time.Date(2026, 9, 29, 10, 0, 0, 0, loc),
		Location: "Clinic",
		Zone:     "Asia/Singapore",
	}
}

func TestCreate(t *testing.T) {
	var method, path string
	var sent calendar.Event
	c := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
		}
		writeJSON(t, w, calendar.Event{Id: sent.Id, Summary: sent.Summary, Start: sent.Start, End: sent.End})
	})

	got, err := c.Create(context.Background(), newEvent(t))
	if err != nil {
		t.Fatal(err)
	}

	if method != http.MethodPost || path != "/calendars/primary/events" {
		t.Fatalf("request = %s %s, want POST to the calendar's events", method, path)
	}
	if sent.Id != "org7a1760000000" || sent.Summary != "Doctor" || sent.Location != "Clinic" ||
		sent.Start.DateTime != "2026-09-29T09:00:00+08:00" || sent.Start.TimeZone != "Asia/Singapore" ||
		sent.End.DateTime != "2026-09-29T10:00:00+08:00" || sent.End.TimeZone != "Asia/Singapore" {
		t.Fatalf("sent %+v start %+v end %+v", sent, sent.Start, sent.End)
	}
	want := Event{
		ID: "org7a1760000000", Title: "Doctor", Location: "Clinic",
		Start: time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 9, 29, 2, 0, 0, 0, time.UTC),
	}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// The first request reached Google but its response was lost: the repeat gets 409,
// and that is the same success, not a second event and not an error.
func TestCreateRepeated(t *testing.T) {
	c := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":{"code":409,"message":"The requested identifier already exists."}}`))
	})

	got, err := c.Create(context.Background(), newEvent(t))
	if err != nil {
		t.Fatalf("repeated create: %v", err)
	}
	if got.ID != "org7a1760000000" || got.Title != "Doctor" {
		t.Fatalf("got %+v", got)
	}
}

func TestCreateFails(t *testing.T) {
	c := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if _, err := c.Create(context.Background(), newEvent(t)); err == nil {
		t.Fatal("a refused insert must be an error: the draft stays and the user can retry")
	}
}
