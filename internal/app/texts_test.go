package app

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"go-organizer/internal/config"
	"go-organizer/internal/store"
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

func TestGMT(t *testing.T) {
	zone := func(name string) *time.Location {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatal(err)
		}
		return loc
	}
	summer := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"Europe/Warsaw":   "GMT+2",
		"America/Toronto": "GMT-4",
		"Asia/Kolkata":    "GMT+5:30",
		"UTC":             "GMT+0",
	}
	for name, want := range cases {
		if got := gmt(summer.In(zone(name))); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
	// Winter time: the offset belongs to the moment, not to the zone.
	if got := gmt(time.Date(2026, 1, 1, 12, 0, 0, 0, zone("America/Toronto"))); got != "GMT-5" {
		t.Errorf("Toronto in winter: %s, want GMT-5", got)
	}
}

type dayEvents struct {
	Day        time.Time
	Events     []eventView
	Tasks      []store.Task
	NoCalendar bool
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

	tasks := []store.Task{{Title: "Pick up the part"}, {Title: "Call the bank"}}
	today := whenView{T: at(18, 0), Today: true}
	tomorrow := whenView{T: at(18, 0).AddDate(0, 0, 1), Tomorrow: true}
	later := whenView{T: at(9, 0).AddDate(0, 0, 3)}
	card := func(w whenView) taskView { return taskView{Title: "Pick up the part", When: w, Checkin: at(21, 0)} }
	checkin := struct{ Tasks []checkinLine }{[]checkinLine{
		{N: 1, Title: "Pick up the part", State: "open"},
		{N: 2, Title: "Call the bank", State: "done"},
		{N: 3, Title: "Renew passport", State: "moved", When: tomorrow},
		{N: 4, Title: "Old idea", State: "cancelled"},
	}}

	inbox := inboxView{Open: 6, Page: 2, Pages: 3, Notes: []noteLine{
		{N: 1, Text: "Buy a bulb"},
		{N: 2, Text: "Call the bank", Done: true},
	}}

	type sample = struct {
		block string
		data  any
	}
	return map[string]sample{
		"tz":                        {"tz", tzView{"Europe/Warsaw", time.Date(2026, 9, 29, 9, 5, 0, 0, warsaw)}},
		"tz current":                {"tz_current", tzView{"Europe/Warsaw", time.Date(2026, 9, 29, 9, 5, 0, 0, warsaw)}},
		"unknown zone":              {"tz_unknown", "Atlantis"},
		"reminder with location":    {"reminder", reminder{seminar, 60}},
		"reminder without location": {"reminder", reminder{doctor, 60}},
		"untitled reminder":         {"reminder", reminder{untitled, 15}},
		"evening":                   {"evening", dayEvents{Day: day, Events: []eventView{birthday, seminar, doctor}}},
		"evening empty":             {"evening_empty", dayEvents{Day: day}},
		"digest, one event":         {"digest", dayEvents{Day: day, Events: []eventView{seminar}}},
		"digest, three events":      {"digest", dayEvents{Day: day, Events: []eventView{birthday, seminar, doctor}}},
		"digest, five events":       {"digest", dayEvents{Day: day, Events: []eventView{doctor, doctor, doctor, doctor, doctor}}},
		"digest empty":              {"digest", dayEvents{Day: day}},
		"digest with tasks":         {"digest", dayEvents{Day: day, Events: []eventView{seminar}, Tasks: tasks}},
		"digest, only tasks":        {"digest", dayEvents{Day: day, Tasks: tasks[:1]}},
		"digest, no calendar":       {"digest", dayEvents{Day: day, NoCalendar: true}},
		"no calendar, tasks":        {"digest", dayEvents{Day: day, Tasks: tasks[:1], NoCalendar: true}},
		"task card today":           {"task_card", card(today)},
		"task card tomorrow":        {"task_card", card(tomorrow)},
		"task moved to a date":      {"task_moved", taskView{Title: "Pick up the part", When: later}},
		"day review":                {"checkin", checkin},
		"review button":             {"btn_chk_tomorrow", 3},
		"review state button":       {"btn_chk_state", checkin.Tasks[2]},
		"inbox page":                {"inbox", inbox},
		"inbox, one page":           {"inbox", inboxView{Open: 1, Page: 1, Pages: 1, Notes: inbox.Notes[:1]}},
		"closed note button":        {"btn_note", inbox.Notes[1]},
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
		"tz":                        "Europe/Warsaw (GMT+2), сейчас 09:05",
		"tz current":                "Europe/Warsaw (GMT+2), сейчас 09:05\nСменить: /tz toronto или /tz America/Toronto",
		"unknown zone":              "Не нашёл зону «Atlantis». Пример: /tz toronto или /tz America/Toronto",
		"reminder with location":    "⏰ Seminar через 60 мин — 10:00, Room 5",
		"reminder without location": "⏰ Doctor через 60 мин — 18:00",
		"untitled reminder":         "⏰ (без названия) через 15 мин — 10:00",
		"evening":                   "📅 Завтра, вт, 29 сен:\n• весь день Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"evening empty":             "📅 На завтра ничего нет, отдыхайте",
		"digest, one event":         "☀️ Сегодня, вт, 29 сен — 1 событие:\n• 10:00–11:30 Seminar, Room 5",
		"digest, three events":      "☀️ Сегодня, вт, 29 сен — 3 события:\n• весь день Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"digest, five events":       "☀️ Сегодня, вт, 29 сен — 5 событий:" + fiveDoctors,
		"digest empty":              "☀️ Сегодня, вт, 29 сен, событий нет",
		"digest with tasks":         "☀️ Сегодня, вт, 29 сен — 1 событие:\n• 10:00–11:30 Seminar, Room 5\n\nЗадачи:\n☐ Pick up the part\n☐ Call the bank",
		"digest, only tasks":        "☀️ Сегодня, вт, 29 сен, событий нет\n\nЗадачи:\n☐ Pick up the part",
		"digest, no calendar":       "☀️ Сегодня, вт, 29 сен, задач нет",
		"no calendar, tasks":        "☀️ Сегодня, вт, 29 сен\nЗадачи:\n☐ Pick up the part",
		"task card today":           "☐ Pick up the part\nНапомню сегодня в 18:00, итог дня в 21:00",
		"task card tomorrow":        "☐ Pick up the part\nНапомню завтра в 18:00, итог дня в 21:00",
		"task moved to a date":      "⏰ Pick up the part → пт, 2 окт в 09:00",
		"day review":                "🌙 Итог дня:\n1. ☐ Pick up the part\n2. ☑ Call the bank\n3. → Renew passport (завтра в 18:00)\n4. ✕ Old idea",
		"review button":             "3 → завтра",
		"review state button":       "3 →",
		"inbox page":                "📥 Ящик: 6 · стр. 2/3\n1. ☐ Buy a bulb\n2. ☑ Call the bank",
		"inbox, one page":           "📥 Ящик: 1\n1. ☐ Buy a bulb",
		"closed note button":        "2 ☑",
	})
}

func TestRenderEn(t *testing.T) {
	checkRender(t, "en", map[string]string{
		"tz":                        "Europe/Warsaw (GMT+2), now 09:05",
		"tz current":                "Europe/Warsaw (GMT+2), now 09:05\nChange it: /tz toronto or /tz America/Toronto",
		"unknown zone":              "No zone \"Atlantis\". Example: /tz toronto or /tz America/Toronto",
		"reminder with location":    "⏰ Seminar in 60 min — 10:00, Room 5",
		"reminder without location": "⏰ Doctor in 60 min — 18:00",
		"untitled reminder":         "⏰ (untitled) in 15 min — 10:00",
		"evening":                   "📅 Tomorrow, Tue, Sep 29:\n• all day Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"evening empty":             "📅 Nothing planned for tomorrow, enjoy your rest",
		"digest, one event":         "☀️ Today, Tue, Sep 29 — 1 event:\n• 10:00–11:30 Seminar, Room 5",
		"digest, three events":      "☀️ Today, Tue, Sep 29 — 3 events:\n• all day Birthday\n• 10:00–11:30 Seminar, Room 5\n• 18:00 Doctor",
		"digest, five events":       "☀️ Today, Tue, Sep 29 — 5 events:" + fiveDoctors,
		"digest empty":              "☀️ Today, Tue, Sep 29, no events",
		"digest with tasks":         "☀️ Today, Tue, Sep 29 — 1 event:\n• 10:00–11:30 Seminar, Room 5\n\nTasks:\n☐ Pick up the part\n☐ Call the bank",
		"digest, only tasks":        "☀️ Today, Tue, Sep 29, no events\n\nTasks:\n☐ Pick up the part",
		"digest, no calendar":       "☀️ Today, Tue, Sep 29, no tasks",
		"no calendar, tasks":        "☀️ Today, Tue, Sep 29\nTasks:\n☐ Pick up the part",
		"task card today":           "☐ Pick up the part\nReminder today at 18:00, day review at 21:00",
		"task card tomorrow":        "☐ Pick up the part\nReminder tomorrow at 18:00, day review at 21:00",
		"task moved to a date":      "⏰ Pick up the part → Fri, Oct 2 at 09:00",
		"day review":                "🌙 Day review:\n1. ☐ Pick up the part\n2. ☑ Call the bank\n3. → Renew passport (tomorrow at 18:00)\n4. ✕ Old idea",
		"review button":             "3 → tomorrow",
		"review state button":       "3 →",
		"inbox page":                "📥 Inbox: 6 · page 2/3\n1. ☐ Buy a bulb\n2. ☑ Call the bank",
		"inbox, one page":           "📥 Inbox: 1\n1. ☐ Buy a bulb",
		"closed note button":        "2 ☑",
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

// TestReadyLanguagesAreComplete: ru and en are finished translations, a new block
// must be added to both.
func TestReadyLanguagesAreComplete(t *testing.T) {
	base := blocks(t, "messages/"+baseLang+".tmpl")
	for _, lang := range []string{"en"} {
		have := blocks(t, "messages/"+lang+".tmpl")
		for _, name := range base {
			if !slices.Contains(have, name) {
				t.Errorf("%s: block %q is missing", lang, name)
			}
		}
	}
}
