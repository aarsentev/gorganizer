package app

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"go-organizer/internal/config"
)

func TestPluralRules(t *testing.T) {
	cases := map[string]map[int]int{
		"ru": {0: 2, 1: 0, 2: 1, 4: 1, 5: 2, 11: 2, 12: 2, 14: 2, 21: 0, 22: 1, 25: 2, 101: 0, 111: 2},
		"pl": {0: 2, 1: 0, 2: 1, 5: 2, 12: 2, 21: 2, 22: 1, 25: 2},
		"en": {0: 1, 1: 0, 2: 1},
		"de": {0: 1, 1: 0, 2: 1},
		"fr": {0: 0, 1: 0, 2: 1},
	}
	for _, lang := range config.Languages {
		rule, ok := pluralRules[lang]
		if !ok {
			t.Errorf("no plural rule for %s", lang)
			continue
		}
		for n, want := range cases[lang] {
			if got := rule(n); got != want {
				t.Errorf("%s plural(%d) = %d, want %d", lang, n, got, want)
			}
		}
	}
}

type dayEvents struct {
	Day    time.Time
	Events []eventView
}

type reminder struct {
	Event   eventView
	Minutes int
}

// renderSamples is the same data for every language; each language test lists its expected texts.
func renderSamples() map[string]struct {
	block string
	data  any
} {
	day := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC) // Tuesday
	at := func(h, m int) time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }

	seminar := eventView{Title: "Seminar", Start: at(10, 0), End: at(11, 30), Location: "Room 5"}
	doctor := eventView{Title: "Doctor", Start: at(18, 0)}
	birthday := eventView{Title: "Birthday", Start: day, AllDay: true}
	untitled := eventView{Start: at(10, 0)}

	type sample = struct {
		block string
		data  any
	}
	return map[string]sample{
		"tz": {"tz", struct {
			Zone string
			Now  time.Time
		}{"Europe/Warsaw", at(9, 5)}},
		"reminder with location":    {"reminder", reminder{seminar, 60}},
		"reminder without location": {"reminder", reminder{doctor, 60}},
		"untitled reminder":         {"reminder", reminder{untitled, 15}},
		"evening":                   {"evening", dayEvents{day, []eventView{birthday, seminar, doctor}}},
		"evening empty":             {"evening_empty", dayEvents{Day: day}},
		"digest, one event":         {"digest", dayEvents{day, []eventView{seminar}}},
		"digest, three events":      {"digest", dayEvents{day, []eventView{birthday, seminar, doctor}}},
		"digest, five events":       {"digest", dayEvents{day, []eventView{doctor, doctor, doctor, doctor, doctor}}},
		"digest empty":              {"digest_empty", dayEvents{Day: day}},
	}
}

func checkRender(t *testing.T, lang string, want map[string]string) {
	t.Helper()
	tx, err := loadTexts()
	if err != nil {
		t.Fatal(err)
	}
	samples := renderSamples()
	if len(want) != len(samples) {
		t.Fatalf("%s: %d expectations for %d samples", lang, len(want), len(samples))
	}
	for name, s := range samples {
		t.Run(name, func(t *testing.T) {
			got, err := tx.render(lang, s.block, s.data)
			if err != nil {
				t.Fatal(err)
			}
			if got != want[name] {
				t.Fatalf("got\n%s\nwant\n%s", got, want[name])
			}
		})
	}
}

const fiveDoctors = "\n• 18:00 Doctor\n• 18:00 Doctor\n• 18:00 Doctor\n• 18:00 Doctor\n• 18:00 Doctor"

func TestRenderRu(t *testing.T) {
	checkRender(t, "ru", map[string]string{
		"tz":                        "Europe/Warsaw, сейчас 09:05",
		"reminder with location":    "⏰ Seminar через 60 мин — 10:00, Room 5",
		"reminder without location": "⏰ Doctor через 60 мин — 18:00",
		"untitled reminder":         "⏰ (без названия) через 15 мин — 10:00",
		"evening":                   "📅 Завтра, вт, 29 сен:\n• весь день Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"evening empty":             "📅 На завтра ничего нет, отдыхайте",
		"digest, one event":         "☀️ Сегодня, вт, 29 сен — 1 событие:\n• 10:00–11:30 Seminar, Room 5",
		"digest, three events":      "☀️ Сегодня, вт, 29 сен — 3 события:\n• весь день Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"digest, five events":       "☀️ Сегодня, вт, 29 сен — 5 событий:" + fiveDoctors,
		"digest empty":              "☀️ Сегодня, вт, 29 сен, событий нет",
	})
}

func TestRenderEn(t *testing.T) {
	checkRender(t, "en", map[string]string{
		"tz":                        "Europe/Warsaw, now 09:05",
		"reminder with location":    "⏰ Seminar in 60 min — 10:00, Room 5",
		"reminder without location": "⏰ Doctor in 60 min — 18:00",
		"untitled reminder":         "⏰ (untitled) in 15 min — 10:00",
		"evening":                   "📅 Tomorrow, Tue, Sep 29:\n• all day Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"evening empty":             "📅 Nothing planned for tomorrow, enjoy your rest",
		"digest, one event":         "☀️ Today, Tue, Sep 29 — 1 event:\n• 10:00–11:30 Seminar, Room 5",
		"digest, three events":      "☀️ Today, Tue, Sep 29 — 3 events:\n• all day Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"digest, five events":       "☀️ Today, Tue, Sep 29 — 5 events:" + fiveDoctors,
		"digest empty":              "☀️ Today, Tue, Sep 29, no events",
	})
}

func TestMissingLanguageFallsBackToBase(t *testing.T) {
	tx, err := loadTexts()
	if err != nil {
		t.Fatal(err)
	}
	got, err := tx.render("fr", "evening_empty", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "📅 На завтра ничего нет, отдыхайте" {
		t.Fatalf("fallback = %q", got)
	}
}

var defineRe = regexp.MustCompile(`{{-?\s*define\s+"([^"]+)"`)

func blocks(t *testing.T, file string) []string {
	t.Helper()
	b, err := fs.ReadFile(messageFiles, file)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range defineRe.FindAllStringSubmatch(string(b), -1) {
		names = append(names, m[1])
	}
	return names
}

// TestLanguageFiles guards the groundwork for translations: a language file may only
// define blocks that exist in the base language and needs date names of its own.
func TestLanguageFiles(t *testing.T) {
	base := blocks(t, "messages/"+baseLang+".tmpl")
	files, _ := fs.Glob(messageFiles, "messages/*.tmpl")
	for _, f := range files {
		lang := strings.TrimSuffix(strings.TrimPrefix(f, "messages/"), ".tmpl")
		if !slices.Contains(config.Languages, lang) {
			t.Errorf("%s: %s is not in config.Languages", f, lang)
			continue
		}
		if _, ok := locales[lang]; !ok {
			t.Errorf("%s: no locale (weekday and month names) for %s", f, lang)
		}
		have := blocks(t, f)
		for _, name := range have {
			if !slices.Contains(base, name) {
				t.Errorf("%s: block %q does not exist in %s", f, name, baseLang)
			}
		}
		for _, name := range base {
			if !slices.Contains(have, name) {
				t.Logf("%s: %q not translated, falls back to %s", f, name, baseLang)
			}
		}
	}
	for _, lang := range config.Languages {
		if !slices.Contains(files, "messages/"+lang+".tmpl") {
			t.Logf("%s: no messages file yet, everything falls back to %s", lang, baseLang)
		}
	}
}
