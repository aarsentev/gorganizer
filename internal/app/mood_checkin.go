package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
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
	day := local.Format(time.DateOnly)
	text, err := a.render("mood_checkin", day)
	if err != nil {
		return nil, err
	}
	if _, err := a.send.SendButtons(ctx, a.cfg.ChatID, text, a.moodKeyboard(t.ID, day, 0)); err != nil {
		return nil, err
	}
	return []any{"tracker_id", t.ID}, nil
}

func (a *App) moodKeyboard(id int64, day string, selected int) [][]tg.Button {
	var row []tg.Button
	for i, emoji := range []string{"😞", "🙁", "😐", "🙂", "😄"} {
		value := i + 1
		label := a.label("btn_mood_answer", struct {
			Emoji    string
			Value    int
			Selected bool
		}{emoji, value, value == selected})
		row = append(row, tg.Button{Text: label, Data: buttonData("mood", id, day+":"+strconv.Itoa(value))})
	}
	return [][]tg.Button{row}
}

func (a *App) onMoodAnswer(ctx context.Context, now time.Time, cb tg.Callback, id int64, action string) error {
	day, raw, ok := strings.Cut(action, ":")
	value, err := strconv.Atoi(raw)
	if !ok || err != nil || value < 1 || value > 5 {
		return errStale
	}
	t, err := a.store.Tracker(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return errStale
	}
	if err != nil {
		return err
	}
	if t.Preset != moodPreset || t.Kind != store.TrackerRating {
		return errStale
	}
	// The button carries the original day, regardless of when or where it is pressed.
	// The store validates the date and keeps repeated identical answers unchanged.
	if err := a.store.SaveTrackerEntry(ctx, id, day, value, now); err != nil {
		return err
	}
	text, err := a.render("mood_checkin", day)
	if err != nil {
		return err
	}
	text += "\n\n" + a.label("mood_saved", value)
	return a.send.Edit(ctx, cb.ChatID, cb.MessageID, text, a.moodKeyboard(id, day, value))
}
