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
