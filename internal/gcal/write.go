package gcal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
)

// NewEvent is a timed event to create.
type NewEvent struct {
	// ID is chosen by the caller (5–1024 characters of 0-9 and a-v) so that a repeated
	// request creates nothing new: if a response was lost, pressing "Create" again
	// must not make a second event.
	ID       string
	Title    string
	Start    time.Time
	End      time.Time
	Location string
	Zone     string // IANA name: the event keeps it, a recurring one stays in it after a move (SPEC §9)
}

// Create inserts the event and returns it in the shape of the local cache, so it can
// be reminded of right away instead of after the next sync.
func (c *Client) Create(ctx context.Context, e NewEvent) (Event, error) {
	item := &calendar.Event{
		Id:       e.ID,
		Summary:  e.Title,
		Location: e.Location,
		Start:    &calendar.EventDateTime{DateTime: e.Start.Format(time.RFC3339), TimeZone: e.Zone},
		End:      &calendar.EventDateTime{DateTime: e.End.Format(time.RFC3339), TimeZone: e.Zone},
	}
	_, err := c.svc.Events.Insert(c.calendarID, item).Context(ctx).Do()
	var apiErr *googleapi.Error
	// 409: an event with this id exists, the earlier request did reach Google.
	if err != nil && !(errors.As(err, &apiErr) && apiErr.Code == http.StatusConflict) {
		return Event{}, fmt.Errorf("insert event: %w", err)
	}
	// Built from what was sent, not parsed from the reply: on 409 there is no event in it.
	// The next sync brings Google's own version anyway.
	return Event{
		ID:       e.ID,
		Title:    e.Title,
		Start:    e.Start.UTC(),
		End:      e.End.UTC(),
		Location: e.Location,
	}, nil
}
