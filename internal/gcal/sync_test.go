package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestConvert(t *testing.T) {
	utc := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v.UTC()
	}

	cases := []struct {
		name string
		in   *calendar.Event
		want Event
	}{
		{
			name: "timed event with offset is stored in UTC",
			in: &calendar.Event{
				Id: "seminar", Summary: "Seminar", Location: "Room 5",
				Start: &calendar.EventDateTime{DateTime: "2026-09-29T10:00:00+02:00"},
				End:   &calendar.EventDateTime{DateTime: "2026-09-29T11:30:00+02:00"},
			},
			want: Event{
				ID: "seminar", Title: "Seminar", Location: "Room 5",
				Start: utc("2026-09-29T08:00:00Z"), End: utc("2026-09-29T09:30:00Z"),
			},
		},
		{
			name: "all-day event keeps its dates as UTC midnight",
			in: &calendar.Event{
				Id: "trip", Summary: "Trip",
				Start: &calendar.EventDateTime{Date: "2026-09-28"},
				End:   &calendar.EventDateTime{Date: "2026-10-01"},
			},
			want: Event{
				ID: "trip", Title: "Trip", AllDay: true,
				Start: utc("2026-09-28T00:00:00Z"), End: utc("2026-10-01T00:00:00Z"),
			},
		},
		{
			name: "untitled event",
			in: &calendar.Event{
				Id:    "untitled",
				Start: &calendar.EventDateTime{DateTime: "2026-09-29T10:00:00Z"},
			},
			want: Event{ID: "untitled", Start: utc("2026-09-29T10:00:00Z")},
		},
		{
			name: "cancelled occurrence with times",
			in: &calendar.Event{
				Id: "seminar_20261012", Status: "cancelled",
				Start: &calendar.EventDateTime{DateTime: "2026-10-12T10:00:00+02:00"},
			},
			want: Event{ID: "seminar_20261012", Deleted: true, Start: utc("2026-10-12T08:00:00Z")},
		},
		{
			name: "cancelled event with only id and status",
			in:   &calendar.Event{Id: "gone", Status: "cancelled"},
			want: Event{ID: "gone", Deleted: true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := convert(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %+v\nwant %+v", got, c.want)
			}
		})
	}

	if _, err := convert(&calendar.Event{Id: "broken"}); err == nil {
		t.Fatal("active event without start must not convert")
	}
}

// fakeCalendar serves Events.List from a sequence of prepared responses.
func fakeCalendar(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	svc, err := calendar.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/"),
		option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return &Client{svc: svc, calendarID: "primary"}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Error(err)
	}
}

func TestChangesFullSyncWithPages(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	var queries []map[string]string

	c := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		queries = append(queries, map[string]string{
			"timeMin": q.Get("timeMin"), "syncToken": q.Get("syncToken"),
			"pageToken": q.Get("pageToken"), "singleEvents": q.Get("singleEvents"),
			"showDeleted": q.Get("showDeleted"),
		})
		if q.Get("pageToken") == "" {
			writeJSON(t, w, calendar.Events{
				Items: []*calendar.Event{
					{Id: "a", Summary: "A", Start: &calendar.EventDateTime{DateTime: "2026-10-01T09:00:00Z"}},
					{Id: "broken", Summary: "No start"},
				},
				NextPageToken: "page2",
			})
			return
		}
		writeJSON(t, w, calendar.Events{
			Items:         []*calendar.Event{{Id: "b", Status: "cancelled"}},
			NextSyncToken: "token-1",
		})
	})

	got, err := c.Changes(context.Background(), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 2 || got.Events[0].ID != "a" || got.Events[1].ID != "b" || !got.Events[1].Deleted {
		t.Fatalf("events = %+v", got.Events)
	}
	if got.SyncToken != "token-1" {
		t.Fatalf("sync token = %q", got.SyncToken)
	}
	if len(got.Skipped) != 1 || got.Skipped[0] != "broken" {
		t.Fatalf("skipped = %v", got.Skipped)
	}

	if len(queries) != 2 {
		t.Fatalf("requests = %d, want 2 pages", len(queries))
	}
	first := queries[0]
	if first["timeMin"] != "2026-09-29T12:00:00Z" || first["syncToken"] != "" {
		t.Fatalf("full sync query = %v, want timeMin one day back and no sync token", first)
	}
	if first["singleEvents"] != "true" || first["showDeleted"] != "true" {
		t.Fatalf("full sync query = %v, want singleEvents and showDeleted", first)
	}
	if queries[1]["pageToken"] != "page2" {
		t.Fatalf("second request = %v, want pageToken page2", queries[1])
	}
}

func TestChangesIncremental(t *testing.T) {
	var query map[string][]string
	c := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		writeJSON(t, w, calendar.Events{NextSyncToken: "token-2"})
	})

	got, err := c.Changes(context.Background(), "token-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncToken != "token-2" || len(got.Events) != 0 {
		t.Fatalf("got %+v", got)
	}
	if query["syncToken"][0] != "token-1" || len(query["timeMin"]) != 0 {
		t.Fatalf("incremental query = %v, want syncToken and no timeMin", query)
	}
}

func TestChangesSyncTokenExpired(t *testing.T) {
	c := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"error":{"code":410,"message":"Sync token is no longer valid"}}`))
	})

	_, err := c.Changes(context.Background(), "stale", time.Now())
	if !errors.Is(err, ErrSyncTokenExpired) {
		t.Fatalf("err = %v, want ErrSyncTokenExpired", err)
	}
}

func TestChangesOtherErrors(t *testing.T) {
	c := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.Changes(context.Background(), "token", time.Now())
	if err == nil || errors.Is(err, ErrSyncTokenExpired) {
		t.Fatalf("err = %v, want a plain error", err)
	}
}
