package gcal

import (
	"context"
	"net/http"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestCheck(t *testing.T) {
	var query string
	ok := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query().Get("maxResults")
		writeJSON(t, w, calendar.Events{})
	})
	if err := ok.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if query != "1" {
		t.Fatalf("maxResults = %q, want one event read", query)
	}

	refused := fakeCalendar(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if err := refused.Check(context.Background()); err == nil {
		t.Fatal("a refused read must be an error")
	}
}
