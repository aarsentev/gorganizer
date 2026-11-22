package app

import (
	"context"
	"errors"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

const moodPreset = "mood"

func (a *App) handleTrackers(ctx context.Context, chatID int64) {
	text, keyboard, err := a.renderMoodSettings(ctx)
	if err == nil {
		_, err = a.send.SendButtons(ctx, chatID, text, keyboard)
	}
	if err != nil {
		a.log.Error("mood settings", "err", err)
		a.replyBlock(ctx, chatID, "trackers_failed", nil)
	}
}

// First enable is idempotent. Later Enable/Pause buttons address the existing
// tracker, so an old first-enable button cannot undo a subsequent pause.
func (a *App) onTracker(ctx context.Context, now time.Time, cb tg.Callback, id int64, action string) error {
	switch {
	case id == 0 && action == "enable-mood":
		if _, err := a.store.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now); err != nil {
			return err
		}
	case id > 0 && (action == "enable" || action == "pause"):
		t, err := a.store.Tracker(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			return errStale
		}
		if err != nil {
			return err
		}
		if t.Preset != moodPreset || t.Kind != store.TrackerRating || (action == "pause" && t.Status == store.TrackerArchived) {
			return errStale
		}
		status := store.TrackerActive
		if action == "pause" {
			status = store.TrackerPaused
		}
		if status != t.Status {
			if err := a.store.SetTrackerStatus(ctx, id, status); err != nil {
				return err
			}
		}
	default:
		// Retired picker and archive controls must not change saved settings.
		return errStale
	}
	text, keyboard, err := a.renderMoodSettings(ctx)
	if err != nil {
		return err
	}
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
}

func (a *App) renderMoodSettings(ctx context.Context) (string, [][]tg.Button, error) {
	t, err := a.store.TrackerForPreset(ctx, moodPreset)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", nil, err
	}
	enabled := t.Status == store.TrackerActive
	label, action := "btn_tracker_enable", "enable"
	if t.ID == 0 {
		action = "enable-mood"
	} else if enabled {
		label, action = "btn_tracker_pause", "pause"
	}
	text, err := a.render("mood_settings", struct {
		Exists, Enabled bool
		Clock, Zone     string
	}{t.ID != 0, enabled, a.cfg.Anchors["eod"], a.Loc().String()})
	keyboard := [][]tg.Button{{{Text: a.label(label, nil), Data: buttonData("trk", t.ID, action)}}}
	return text, keyboard, err
}
