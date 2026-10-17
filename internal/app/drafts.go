package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go-organizer/internal/intent"
	"go-organizer/internal/store"
)

// A draft is the intent of a dialog in progress, kept in store.Pending between
// messages as JSON: store does not know the intent package.

// newDraft starts the chat's draft, replacing an older one, and returns its id for the buttons.
func (a *App) newDraft(ctx context.Context, chatID int64, in intent.Intent, asking string, now time.Time) (int64, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return 0, fmt.Errorf("encode draft: %w", err)
	}
	return a.store.CreatePending(ctx, chatID, raw, asking, now)
}

// draft loads the draft a button refers to; store.ErrNotFound when it is gone.
func (a *App) draft(ctx context.Context, id int64) (store.Pending, intent.Intent, error) {
	p, err := a.store.Pending(ctx, id)
	if err != nil {
		return store.Pending{}, intent.Intent{}, err
	}
	var in intent.Intent
	if err := json.Unmarshal(p.IntentRaw, &in); err != nil {
		return store.Pending{}, intent.Intent{}, fmt.Errorf("decode draft %d: %w", id, err)
	}
	return p, in, nil
}
