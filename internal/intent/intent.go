// Package intent is what the user asked for, in the form a parser hands to the app:
// the strict-syntax commands now, the model at stage 5. The JSON form is the contract:
// drafts are stored in it, and the model will answer in it.
package intent

import "time"

// Intent is flat: one struct checked by code, not a tagged union (SPEC §7).
// Fields of SPEC §7 are added when a stage needs them.
type Intent struct {
	Action      string     `json:"action"`         // create|update|delete|note|clarify
	Kind        string     `json:"kind,omitempty"` // event|task
	Title       string     `json:"title,omitempty"`
	Start       *time.Time `json:"start,omitempty"`
	When        string     `json:"when,omitempty"` // for an event: a known date (2026-09-29) whose time is still asked
	DurationMin int        `json:"duration_min,omitempty"`
	Location    string     `json:"location,omitempty"`
}
