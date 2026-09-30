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

// ErrSyncTokenExpired means Google answered 410 Gone: drop the cache and sync from scratch.
var ErrSyncTokenExpired = errors.New("sync token expired")

// Event is a calendar event in the shape the bot needs.
//
// All-day events have only dates: Start and End are UTC midnight of the first day
// and of the day after the last one (Google's exclusive end).
type Event struct {
	ID       string
	Title    string
	Start    time.Time
	End      time.Time
	AllDay   bool
	Location string
	Deleted  bool
}

type Changes struct {
	Events    []Event
	SyncToken string   // pass to the next Changes call
	Skipped   []string // ids of events that could not be parsed
}

// fullSyncLookback limits a full sync: older events are useless for reminders
// and would double the download.
const fullSyncLookback = 24 * time.Hour

// Changes returns events changed since syncToken. An empty syncToken means
// a full sync of events starting from now minus one day.
func (c *Client) Changes(ctx context.Context, syncToken string, now time.Time) (Changes, error) {
	var (
		out       Changes
		pageToken string
	)
	for {
		call := c.svc.Events.List(c.calendarID).
			SingleEvents(true). // every occurrence of a series is its own event
			ShowDeleted(true).  // cancelled occurrences must reach the cache as deleted
			MaxResults(2500).
			Context(ctx)
		if syncToken == "" {
			call = call.TimeMin(now.Add(-fullSyncLookback).Format(time.RFC3339))
		} else {
			call = call.SyncToken(syncToken)
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			var apiErr *googleapi.Error
			if errors.As(err, &apiErr) && apiErr.Code == http.StatusGone {
				return Changes{}, ErrSyncTokenExpired
			}
			return Changes{}, fmt.Errorf("list events: %w", err)
		}

		for _, item := range resp.Items {
			e, err := convert(item)
			if err != nil {
				out.Skipped = append(out.Skipped, item.Id)
				continue
			}
			out.Events = append(out.Events, e)
		}

		if resp.NextPageToken == "" {
			out.SyncToken = resp.NextSyncToken
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
}

func convert(item *calendar.Event) (Event, error) {
	e := Event{
		ID:       item.Id,
		Title:    item.Summary,
		Location: item.Location,
		Deleted:  item.Status == "cancelled",
	}

	start, allDay, err := parseTime(item.Start)
	if err != nil {
		// Cancelled events in an incremental sync often carry only id and status.
		if e.Deleted {
			return e, nil
		}
		return Event{}, fmt.Errorf("event %s start: %w", item.Id, err)
	}
	e.Start, e.AllDay = start, allDay

	if end, _, err := parseTime(item.End); err == nil {
		e.End = end
	}
	return e, nil
}

func parseTime(t *calendar.EventDateTime) (time.Time, bool, error) {
	switch {
	case t == nil:
		return time.Time{}, false, errors.New("missing")
	case t.DateTime != "":
		v, err := time.Parse(time.RFC3339, t.DateTime)
		return v.UTC(), false, err
	case t.Date != "":
		v, err := time.Parse(time.DateOnly, t.Date)
		return v, true, err
	default:
		return time.Time{}, false, errors.New("empty")
	}
}
