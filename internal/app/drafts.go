package app

import (
	"context"
	"encoding/json"
	"errors"
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

// updateDraft stores the draft after an answer, with the field the next question is about.
func (a *App) updateDraft(ctx context.Context, id int64, in intent.Intent, asking string, now time.Time) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("encode draft: %w", err)
	}
	return a.store.UpdatePending(ctx, id, raw, asking, now)
}

// draft loads the draft a button refers to; store.ErrNotFound when it is gone.
func (a *App) draft(ctx context.Context, id int64) (store.Pending, intent.Intent, error) {
	p, err := a.store.Pending(ctx, id)
	if err != nil {
		return store.Pending{}, intent.Intent{}, err
	}
	in, err := decodeDraft(p)
	return p, in, err
}

func decodeDraft(p store.Pending) (intent.Intent, error) {
	var in intent.Intent
	if err := json.Unmarshal(p.IntentRaw, &in); err != nil {
		return intent.Intent{}, fmt.Errorf("decode draft %d: %w", p.ID, err)
	}
	return in, nil
}

// draftTimeout closes a draft whose question has waited this long without an answer,
// so that a forgotten question does not take the next message as its answer.
const draftTimeout = 10 * time.Minute

// expireDrafts runs on every tick. It sends nothing: the question stays in the chat,
// and its buttons answer "no longer relevant".
func (a *App) expireDrafts(ctx context.Context, now time.Time) {
	n, err := a.store.ExpirePending(ctx, now.Add(-draftTimeout))
	if err != nil {
		a.log.Error("expire drafts", "err", err)
		return
	}
	if n > 0 {
		a.log.Info("drafts expired", "count", n)
	}
}

// handleCancel closes the chat's draft, a question or a card alike. The draft's message
// stays as it is, since the draft does not keep its id; its buttons go stale.
func (a *App) handleCancel(ctx context.Context, chatID int64) {
	p, err := a.store.PendingFor(ctx, chatID)
	if errors.Is(err, store.ErrNotFound) {
		a.replyBlock(ctx, chatID, "cancel_nothing", nil)
		return
	}
	if err != nil {
		a.log.Error("cancel", "err", err)
		return
	}
	in, err := decodeDraft(p)
	if err != nil {
		a.log.Error("cancel", "err", err)
		return
	}
	if err := a.store.DeletePending(ctx, p.ID); err != nil {
		a.log.Error("cancel", "err", err)
		return
	}
	a.replyBlock(ctx, chatID, "event_cancelled", in)
}
