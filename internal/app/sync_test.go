package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-organizer/internal/gcal"
	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

type calendarReply struct {
	changes gcal.Changes
	err     error
}

// fakeCalendar returns prepared replies in order and records the tokens it was asked with,
// and the events it was asked to create.
type fakeCalendar struct {
	replies   []calendarReply
	tokens    []string
	created   []gcal.NewEvent
	createErr error // the next creates fail while it is set
	checkErr  error
}

func (f *fakeCalendar) Check(context.Context) error {
	return f.checkErr
}

func (f *fakeCalendar) Create(_ context.Context, e gcal.NewEvent) (gcal.Event, error) {
	if f.createErr != nil {
		return gcal.Event{}, f.createErr
	}
	f.created = append(f.created, e)
	return gcal.Event{ID: e.ID, Title: e.Title, Start: e.Start.UTC(), End: e.End.UTC(), Location: e.Location}, nil
}

func (f *fakeCalendar) Changes(_ context.Context, token string, _ time.Time) (gcal.Changes, error) {
	f.tokens = append(f.tokens, token)
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r.changes, r.err
}

func cachedIDs(t *testing.T, st *store.Store) []string {
	t.Helper()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events, err := st.TimedEventsBetween(context.Background(), from, from.AddDate(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range events {
		ids = append(ids, e.ID+"@"+e.Start.In(warsaw).Format("15:04"))
	}
	return ids
}

func syncToken(t *testing.T, st *store.Store) string {
	t.Helper()
	token, err := st.SyncToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSyncFullThenIncremental(t *testing.T) {
	ctx := context.Background()
	now := at(t, "2026-09-29 08:00")
	cal := &fakeCalendar{replies: []calendarReply{
		{changes: gcal.Changes{SyncToken: "t1", Events: []gcal.Event{
			{ID: "seminar", Title: "Seminar", Start: at(t, "2026-09-29 09:00")},
			{ID: "doctor", Title: "Doctor", Start: at(t, "2026-09-29 12:00")},
		}}},
		{changes: gcal.Changes{SyncToken: "t2", Events: []gcal.Event{
			{ID: "seminar", Title: "Seminar", Start: at(t, "2026-09-29 11:00")},
			{ID: "doctor", Deleted: true},
		}}},
	}}
	a, _, st := newTestApp(t, cal, nil)

	a.Sync(ctx, now)
	if got := cachedIDs(t, st); !sameStrings(got, []string{"seminar@09:00", "doctor@12:00"}) {
		t.Fatalf("after full sync cache = %v", got)
	}
	if syncToken(t, st) != "t1" {
		t.Fatalf("token = %q, want t1", syncToken(t, st))
	}

	a.Sync(ctx, now.Add(5*time.Minute))
	if got := cachedIDs(t, st); !sameStrings(got, []string{"seminar@11:00"}) {
		t.Fatalf("after incremental sync cache = %v, want moved seminar and no doctor", got)
	}
	if !sameStrings(cal.tokens, []string{"", "t1"}) || syncToken(t, st) != "t2" {
		t.Fatalf("tokens asked %v, stored %q", cal.tokens, syncToken(t, st))
	}
}

func TestSyncTokenExpiredReplacesCache(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{replies: []calendarReply{
		{err: gcal.ErrSyncTokenExpired},
		{changes: gcal.Changes{SyncToken: "t9", Events: []gcal.Event{
			{ID: "fresh", Title: "Fresh", Start: at(t, "2026-09-29 10:00")},
		}}},
	}}
	a, _, st := newTestApp(t, cal, nil)
	put(t, st, store.Event{ID: "stale", Title: "Stale", Start: at(t, "2026-09-29 09:00")})
	if err := st.SetSyncToken(ctx, "old"); err != nil {
		t.Fatal(err)
	}

	a.Sync(ctx, at(t, "2026-09-29 08:00"))

	if !sameStrings(cal.tokens, []string{"old", ""}) {
		t.Fatalf("tokens asked %v, want old then a full sync", cal.tokens)
	}
	if got := cachedIDs(t, st); !sameStrings(got, []string{"fresh@10:00"}) {
		t.Fatalf("cache = %v, want only the full sync result", got)
	}
	if syncToken(t, st) != "t9" {
		t.Fatalf("token = %q, want t9", syncToken(t, st))
	}
}

func TestSyncTakesCalendarZone(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 7, 5, 0, 0, time.UTC)
	cal := &fakeCalendar{replies: []calendarReply{
		{changes: gcal.Changes{SyncToken: "t1", Zone: "Asia/Singapore"}},
		{changes: gcal.Changes{SyncToken: "t2", Zone: "Asia/Singapore"}},
	}}
	a, send, _ := newTestApp(t, cal, nil)

	// A new install starts in the zone from config and takes the calendar's on the first sync.
	a.Sync(ctx, now)
	a.Sync(ctx, now.Add(5*time.Minute))

	if got := a.Loc().String(); got != "Asia/Singapore" {
		t.Fatalf("Loc = %s, want the calendar's zone", got)
	}
	want := []string{"Часовой пояс взят из Google Календаря: Asia/Singapore (GMT+8), сейчас 15:05"}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent %q, want %q", send.sent, want)
	}
}

func TestSyncLastZoneChangeWins(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 29, 7, 5, 0, 0, time.UTC)
	cal := &fakeCalendar{replies: []calendarReply{
		{changes: gcal.Changes{SyncToken: "t1", Zone: "Europe/Warsaw"}},
		{changes: gcal.Changes{SyncToken: "t2", Zone: "Europe/Warsaw"}},
		{changes: gcal.Changes{SyncToken: "t3"}},
		{changes: gcal.Changes{SyncToken: "t4", Zone: "Asia/Singapore"}},
	}}
	a, send, _ := newTestApp(t, cal, nil)

	a.Sync(ctx, now) // the same zone as the bot: nothing to say
	a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/tz toronto"})
	a.Sync(ctx, now) // the calendar has not changed: /tz stays
	if got := a.Loc().String(); got != "America/Toronto" {
		t.Fatalf("Loc = %s, the sync undid /tz", got)
	}
	a.Sync(ctx, now) // no zone in the answer: nothing changes
	a.Sync(ctx, now) // the zone was changed in Google: the bot follows

	if got := a.Loc().String(); got != "Asia/Singapore" {
		t.Fatalf("Loc = %s, want the calendar's new zone", got)
	}
	want := []string{
		"America/Toronto (GMT-4), сейчас 03:05",
		"Часовой пояс взят из Google Календаря: Asia/Singapore (GMT+8), сейчас 15:05",
	}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent %q, want %q", send.sent, want)
	}
}

func TestSyncErrorKeepsCache(t *testing.T) {
	ctx := context.Background()
	cal := &fakeCalendar{replies: []calendarReply{{err: errors.New("google is down")}}}
	a, _, st := newTestApp(t, cal, nil)
	put(t, st, store.Event{ID: "kept", Title: "Kept", Start: at(t, "2026-09-29 09:00")})
	if err := st.SetSyncToken(ctx, "t1"); err != nil {
		t.Fatal(err)
	}

	a.Sync(ctx, at(t, "2026-09-29 08:00"))

	if got := cachedIDs(t, st); !sameStrings(got, []string{"kept@09:00"}) || syncToken(t, st) != "t1" {
		t.Fatalf("cache = %v, token = %q; want both untouched", got, syncToken(t, st))
	}
}
