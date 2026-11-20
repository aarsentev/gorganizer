package app

import (
	"context"
	"errors"
	"time"

	"go-organizer/internal/store"
)

// Mood uses the evening schedule but has its own daily delivery mark: retrying a
// failed prompt must not resend the task review (or the other way around).
func (a *App) moodCheckin(ctx context.Context, local time.Time) ([]any, error) {
	t, err := a.store.TrackerForPreset(ctx, moodPreset)
	if errors.Is(err, store.ErrNotFound) {
		return []any{"trackers", 0}, nil
	}
	if err != nil {
		return nil, err
	}
	if t.Status != store.TrackerActive {
		return []any{"trackers", 0}, nil
	}
	// The visible date stays with the question when it is read on a later day.
	text, err := a.render("mood_checkin", local.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	if _, err := a.send.Send(ctx, a.cfg.ChatID, text); err != nil {
		return nil, err
	}
	return []any{"tracker_id", t.ID}, nil
}
