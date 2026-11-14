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
	Text        string     `json:"text,omitempty"` // a note's text
}

// What a question can be about. Missing returns one, and the draft keeps it as asking,
// so that the answer is read as that field.
const (
	AskWhen = "when" // neither the date nor the time: "when?"
	AskTime = "time" // only the date, in When: "at what time?"
	AskEnd  = "end"  // the end of an event: "until when?"
)

// Missing is the first field the user still has to be asked about, "" when nothing is.
// Code decides it, not the parser: the model leaves out what it did not hear (SPEC §7).
func (in Intent) Missing() string {
	if in.Kind != "event" {
		return ""
	}
	switch {
	case in.Start == nil && in.When == "":
		return AskWhen
	case in.Start == nil:
		return AskTime
	case in.DurationMin == 0:
		return AskEnd
	}
	return ""
}
