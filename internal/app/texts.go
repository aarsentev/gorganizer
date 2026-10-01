package app

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
	"time"
)

//go:embed messages/*.tmpl
var messageFiles embed.FS

// baseLang has every text. Other languages may define only part of the blocks;
// the rest falls back to baseLang.
const baseLang = "ru"

// locale holds what Go cannot localize on its own: weekday and month names.
type locale struct {
	date func(t time.Time) string
}

var locales = map[string]locale{
	"ru": {date: func(t time.Time) string {
		weekdays := [7]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
		months := [12]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}
		return fmt.Sprintf("%s, %d %s", weekdays[t.Weekday()], t.Day(), months[t.Month()-1])
	}},
	// Go's own names are English, only the order differs from Russian.
	"en": {date: func(t time.Time) string { return t.Format("Mon, Jan 2") }},
}

// pluralRules pick the form index for n. Rules are grammar, not translation,
// so all five languages are here already.
var pluralRules = map[string]func(n int) int{
	"ru": slavicPlural,
	// Polish uses "one" only for exactly 1: 21 wydarzeń, unlike Russian 21 событие.
	"pl": func(n int) int {
		if n == 1 {
			return 0
		}
		if slavicPlural(n) == 1 {
			return 1
		}
		return 2
	},
	"en": func(n int) int { return btoi(n != 1) },
	"de": func(n int) int { return btoi(n != 1) },
	"fr": func(n int) int { return btoi(n > 1) },
}

// slavicPlural: 1 событие, 2 события, 5 событий; 11–14 always take the last form.
func slavicPlural(n int) int {
	n %= 100
	switch {
	case n%10 == 1 && n != 11:
		return 0
	case n%10 >= 2 && n%10 <= 4 && (n < 12 || n > 14):
		return 1
	default:
		return 2
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func funcs(lang string) template.FuncMap {
	loc, ok := locales[lang]
	if !ok {
		loc = locales[baseLang]
	}
	rule := pluralRules[lang]
	return template.FuncMap{
		"date":  loc.date,
		"clock": func(t time.Time) string { return t.Format("15:04") },
		"plural": func(n int, forms ...string) string {
			i := rule(n)
			if i >= len(forms) {
				i = len(forms) - 1
			}
			return forms[i]
		},
	}
}

// texts renders bot messages by block name in the requested language.
type texts struct {
	sets map[string]*template.Template
}

func loadTexts() (*texts, error) {
	base, err := template.New(baseLang).Funcs(funcs(baseLang)).ParseFS(messageFiles, "messages/"+baseLang+".tmpl")
	if err != nil {
		return nil, fmt.Errorf("messages %s: %w", baseLang, err)
	}
	t := &texts{sets: map[string]*template.Template{baseLang: base}}

	files, err := fs.Glob(messageFiles, "messages/*.tmpl")
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		lang := strings.TrimSuffix(strings.TrimPrefix(f, "messages/"), ".tmpl")
		if lang == baseLang {
			continue
		}
		// Clone base so missing blocks fall back to it, then override with this language.
		set, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if _, err := set.Funcs(funcs(lang)).ParseFS(messageFiles, f); err != nil {
			return nil, fmt.Errorf("messages %s: %w", lang, err)
		}
		t.sets[lang] = set
	}
	return t, nil
}

func (t *texts) render(lang, name string, data any) (string, error) {
	set, ok := t.sets[lang]
	if !ok {
		set = t.sets[baseLang]
	}
	var b strings.Builder
	if err := set.ExecuteTemplate(&b, name, data); err != nil {
		return "", fmt.Errorf("render %s/%s: %w", lang, name, err)
	}
	return strings.TrimSpace(b.String()), nil
}

// eventView is an event prepared for a template: times are already in the user's zone,
// all-day events carry only the date.
type eventView struct {
	Title    string
	Start    time.Time
	End      time.Time
	AllDay   bool
	Location string
}
