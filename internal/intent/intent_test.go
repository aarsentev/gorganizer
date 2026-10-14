package intent

import (
	"encoding/json"
	"testing"
	"time"
)

// The JSON names are the contract with stored drafts and, at stage 5, with the model:
// renaming a Go field must not change them.
func TestJSONForm(t *testing.T) {
	start := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
	in := Intent{Action: "create", Kind: "event", Title: "Doctor", Start: &start, DurationMin: 60, Location: "Clinic"}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"action":"create","kind":"event","title":"Doctor","start":"2026-09-29T07:00:00Z","duration_min":60,"location":"Clinic"}`
	if string(b) != want {
		t.Fatalf("json = %s\nwant   %s", b, want)
	}

	var out Intent
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Title != in.Title || !out.Start.Equal(start) || out.DurationMin != 60 || out.Location != "Clinic" {
		t.Fatalf("round trip = %+v", out)
	}

	// Unknown so far: no start, no time; only the date is known.
	partial, _ := json.Marshal(Intent{Action: "create", Kind: "event", Title: "Doctor", When: "2026-09-29"})
	if string(partial) != `{"action":"create","kind":"event","title":"Doctor","when":"2026-09-29"}` {
		t.Fatalf("partial json = %s", partial)
	}
}
