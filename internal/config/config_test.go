package config

import (
	"os"
	"strings"
	"testing"
)

const valid = `
tz: Europe/Warsaw
lang: ru
anchors:
  morning: "09:00"
  afternoon: "14:00"
  evening: "18:00"
  eod: "21:00"
quiet_hours:
  from: "00:00"
  to: "07:00"
reminders:
  event_offsets_min: [60, 15]
  evening_at: "20:00"
  cleanup: true
digest:
  at: "07:30"
`

func TestParseValid(t *testing.T) {
	c, err := parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if c.Lang != "ru" || c.Digest.At != "07:30" || c.Reminders.EveningAt != "20:00" || !c.Reminders.Cleanup {
		t.Fatalf("parsed %+v", c)
	}
	if len(c.Reminders.EventOffsetsMin) != 2 || c.Reminders.EventOffsetsMin[1] != 15 {
		t.Fatalf("offsets = %v", c.Reminders.EventOffsetsMin)
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name, from, to, wantErr string
	}{
		{"unknown key from an old config", `  cleanup: true`, "  cleanup: true\n  day_before_at: \"20:00\"", "day_before_at"},
		{"missing digest time", `  at: "07:30"`, ``, "digest.at"},
		{"bad clock", `  at: "07:30"`, `  at: "24:00"`, "digest.at"},
		{"unknown zone", `tz: Europe/Warsaw`, `tz: Europe/Atlantis`, "tz"},
		{"unsupported language", `lang: ru`, `lang: es`, "lang"},
		{"missing language", `lang: ru`, ``, "lang"},
		{"non-positive offset", `[60, 15]`, `[60, 0]`, "event_offsets_min"},
		{"missing anchor", `  eod: "21:00"`, ``, "anchors.eod"},
		{"digest after evening", `  at: "07:30"`, `  at: "21:00"`, "must be before"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(valid, c.from) {
				t.Fatalf("fixture does not contain %q", c.from)
			}
			_, err := parse([]byte(strings.Replace(valid, c.from, c.to, 1)))
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want mention of %q", err, c.wantErr)
			}
		})
	}
}

func TestRepoConfigIsValid(t *testing.T) {
	c, err := parseFile(t, "../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if c.Digest.At == "" {
		t.Fatal("config.yaml has no digest.at")
	}
}

func parseFile(t *testing.T, path string) (*Config, error) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parse(b)
}
