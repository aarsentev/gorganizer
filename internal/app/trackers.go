package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

const moodPreset = "mood"

type trackerView struct {
	store.Tracker
	Clock string
	Zone  string
}

func (a *App) handleTrackers(ctx context.Context, chatID int64) {
	text, keyboard, err := a.renderTrackers(ctx)
	if err == nil {
		_, err = a.send.SendButtons(ctx, chatID, text, keyboard)
	}
	if err != nil {
		a.log.Error("trackers", "err", err)
		a.replyBlock(ctx, chatID, "trackers_failed", nil)
	}
}

// All navigation edits the same message. Preset creation is idempotent in storage,
// including a retry after saving succeeded but the Telegram edit failed.
func (a *App) onTracker(ctx context.Context, now time.Time, cb tg.Callback, id int64, action string) error {
	if id == 0 {
		switch action {
		case "list":
			text, keyboard, err := a.renderTrackers(ctx)
			if err != nil {
				return err
			}
			return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, keyboard)
		case "add":
			return a.editBlock(ctx, cb, "tracker_presets", nil, [][]tg.Button{
				{a.trackerButton("btn_tracker_mood", 0, "mood")},
				{a.trackerButton("btn_tracker_back", 0, "list")},
			})
		case "mood":
			t, err := a.store.TrackerForPreset(ctx, moodPreset)
			if err == nil {
				return a.showTracker(ctx, cb, t)
			}
			if !errors.Is(err, store.ErrNotFound) {
				return err
			}
			return a.editBlock(ctx, cb, "tracker_mood_preview", a.trackerView(store.Tracker{}), [][]tg.Button{{
				a.trackerButton("btn_tracker_enable", 0, "enable-mood"),
				a.trackerButton("btn_tracker_back", 0, "add"),
			}})
		case "enable-mood":
			id, err := a.store.CreatePresetTracker(ctx, moodPreset, "Mood", store.TrackerRating, "", now)
			if err != nil {
				return err
			}
			return a.onTracker(ctx, now, cb, id, "view")
		default:
			return fmt.Errorf("unknown tracker setup action %q", action)
		}
	}

	t, err := a.store.Tracker(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return errStale
	}
	if err != nil {
		return err
	}
	status := t.Status
	switch action {
	case "view":
	case "pause", "resume":
		// An old pause/resume button must not silently bring back an archived tracker.
		if t.Status == store.TrackerArchived {
			return errStale
		}
		status = store.TrackerActive
		if action == "pause" {
			status = store.TrackerPaused
		}
	case "archive":
		status = store.TrackerArchived
	case "restore":
		if t.Status == store.TrackerPaused {
			return errStale
		}
		status = store.TrackerActive
	default:
		return fmt.Errorf("unknown tracker action %q", action)
	}
	if status != t.Status {
		if err := a.store.SetTrackerStatus(ctx, id, status); err != nil {
			return err
		}
		t.Status = status
	}
	return a.showTracker(ctx, cb, t)
}

func (a *App) renderTrackers(ctx context.Context) (string, [][]tg.Button, error) {
	trackers, err := a.store.Trackers(ctx)
	if err != nil {
		return "", nil, err
	}
	var keyboard [][]tg.Button
	for _, t := range trackers {
		keyboard = append(keyboard, []tg.Button{{
			Text: a.label("tracker_summary", t), Data: buttonData("trk", t.ID, "view"),
		}})
	}
	keyboard = append(keyboard, []tg.Button{a.trackerButton("btn_tracker_add", 0, "add")})
	text, err := a.render("trackers", trackers)
	return text, keyboard, err
}

func (a *App) trackerView(t store.Tracker) trackerView {
	return trackerView{Tracker: t, Clock: a.cfg.Anchors["eod"], Zone: a.Loc().String()}
}

func (a *App) showTracker(ctx context.Context, cb tg.Callback, t store.Tracker) error {
	var controls []tg.Button
	switch t.Status {
	case store.TrackerActive:
		controls = append(controls, a.trackerButton("btn_tracker_pause", t.ID, "pause"))
	case store.TrackerPaused:
		controls = append(controls, a.trackerButton("btn_tracker_resume", t.ID, "resume"))
	case store.TrackerArchived:
		controls = append(controls, a.trackerButton("btn_tracker_enable", t.ID, "restore"))
	}
	if t.Status != store.TrackerArchived {
		controls = append(controls, a.trackerButton("btn_tracker_archive", t.ID, "archive"))
	}
	return a.editBlock(ctx, cb, "tracker_detail", a.trackerView(t), [][]tg.Button{
		controls,
		{a.trackerButton("btn_tracker_back", 0, "list")},
	})
}

func (a *App) trackerButton(label string, id int64, action string) tg.Button {
	return tg.Button{Text: a.label(label, nil), Data: buttonData("trk", id, action)}
}
