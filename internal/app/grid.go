package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-organizer/internal/moodgrid"
	"go-organizer/internal/store"
)

func (a *App) handleGrid(ctx context.Context, now time.Time, chatID int64, args string) {
	if args != "" && !strings.EqualFold(args, moodPreset) {
		a.replyBlock(ctx, chatID, "grid_usage", nil)
		return
	}
	if err := a.sendMoodGrid(ctx, now.In(a.Loc()), chatID); err != nil {
		a.log.Error("mood grid", "err", err)
		a.replyBlock(ctx, chatID, "grid_failed", nil)
	}
}

func (a *App) sendMoodGrid(ctx context.Context, today time.Time, chatID int64) error {
	t, err := a.store.TrackerForPreset(ctx, moodPreset)
	if errors.Is(err, store.ErrNotFound) {
		a.replyBlock(ctx, chatID, "grid_empty", today.Year())
		return nil
	}
	if err != nil {
		return err
	}
	// Dates belong to the original check-ins. A timezone change can leave a
	// saved answer ahead of today's local date; show it once that date arrives.
	first := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, today.Location())
	entries, err := a.store.TrackerEntries(ctx, t.ID, first.Format(time.DateOnly), today.AddDate(0, 0, 1).Format(time.DateOnly))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		a.replyBlock(ctx, chatID, "grid_empty", today.Year())
		return nil
	}
	ratings := make(map[string]int, len(entries))
	for _, entry := range entries {
		ratings[entry.Day] = entry.Value
	}
	data, err := moodgrid.Render(today.Year(), today, ratings)
	if err != nil {
		return err
	}
	caption, err := a.render("grid_caption", today.Year())
	if err != nil {
		return err
	}
	_, err = a.send.SendPNG(ctx, chatID, data, caption)
	return err
}
