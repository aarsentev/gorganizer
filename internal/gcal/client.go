package gcal

import (
	"context"
	"fmt"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type Client struct {
	svc        *calendar.Service
	calendarID string
}

// New Client limited to calendar events. It does not touch the network.
func New(ctx context.Context, saFile, calendarID string) (*Client, error) {
	svc, err := calendar.NewService(ctx,
		option.WithAuthCredentialsFile(option.ServiceAccount, saFile),
		option.WithScopes(calendar.CalendarEventsScope))
	if err != nil {
		return nil, fmt.Errorf("calendar: %w", err)
	}
	return &Client{svc: svc, calendarID: calendarID}, nil
}

// Check reads one event: the key, the sharing and the network all work. It writes nothing,
// unlike doctor, so it can run on every deploy.
func (c *Client) Check(ctx context.Context) error {
	if _, err := c.svc.Events.List(c.calendarID).MaxResults(1).Context(ctx).Do(); err != nil {
		return fmt.Errorf("read calendar: %w", err)
	}
	return nil
}
