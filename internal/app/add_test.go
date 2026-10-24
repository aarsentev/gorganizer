package app

import (
	"context"
	"errors"
	"testing"

	"go-organizer/internal/store"
	"go-organizer/internal/tg"
)

func TestParseAdd(t *testing.T) {
	cases := []struct {
		args            string
		title, location string
		said            said
	}{
		{"29.09 9:00-10:00 Приём у врача @ клиника на Ленина 12", "Приём у врача", "клиника на Ленина 12", said{"29.09", "9:00", 60}},
		{"приём у врача 30.09 9:00", "Приём у врача", "", said{"30.09", "9:00", 0}},
		{"1.10.2026 10:00–11:30 Пара", "Пара", "", said{"1.10.2026", "10:00", 90}}, // en dash
		{"Ночная смена 23:00-7:00", "Ночная смена", "", said{"", "23:00", 480}},    // ends the next day
		{"Созвон @Zoom", "Созвон", "Zoom", said{}},
		{"Приём у врача", "Приём у врача", "", said{}},
		// A bare number is part of the title.
		{"30.09 9:00 2 ящика пива", "2 ящика пива", "", said{"30.09", "9:00", 0}},
		// Words and units are not understood before the model: they stay in the title.
		{"Приём у врача завтра в 9:00", "Приём у врача завтра в", "", said{"", "9:00", 0}},
		{"30.09 9:00 1ч Пара", "1ч Пара", "", said{"30.09", "9:00", 0}},
		// An e-mail is not a place: the place starts after a space and "@".
		{"Написать a@b.com 30.09", "Написать a@b.com", "", said{"30.09", "", 0}},
		// A second date is not taken: it stays in the title.
		{"30.09 1.10 Встреча", "1.10 Встреча", "", said{"30.09", "", 0}},
	}
	for _, c := range cases {
		title, location, s, err := parseAdd(c.args)
		if err != nil || title != c.title || location != c.location || s != c.said {
			t.Errorf("parseAdd(%q) = %q, %q, %+v, %v\nwant %q, %q, %+v", c.args, title, location, s, err, c.title, c.location, c.said)
		}
	}

	for args, want := range map[string]error{
		"":                  errNoTitle,
		"30.09 9:00-10:00":  errNoTitle,
		"@ клиника":         errNoTitle,
		"31.02 Встреча":     errBadDate,
		"Встреча 13.13":     errBadDate,
		"29.02.2027 Тест":   errBadDate, // 2027 is not a leap year
		"9:00-9:00 Встреча": errBadRange,
		"9:00-25:00 Смена":  errBadRange,
	} {
		if _, _, _, err := parseAdd(args); !errors.Is(err, want) {
			t.Errorf("parseAdd(%q) err = %v, want %v", args, err, want)
		}
	}
}

func TestParseWhen(t *testing.T) {
	cases := map[string]said{
		"30.09 9:00":       {"30.09", "9:00", 0},
		"10:30":            {"", "10:30", 0},
		"1.10 14:00-16:00": {"1.10", "14:00", 120},
	}
	for text, want := range cases {
		if got, err := parseWhen(text); err != nil || got != want {
			t.Errorf("parseWhen(%q) = %+v, %v; want %+v", text, got, err, want)
		}
	}
	for text, want := range map[string]error{
		"клиника на Ленина": errNotWhen,
		"30.09 клиника":     errNotWhen, // the whole answer must be understood
		"завтра":            errNotWhen, // words come with the model
		"45м":               errNotWhen,
		"":                  errNotWhen,
		"30.02":             errBadDate,
	} {
		if _, err := parseWhen(text); !errors.Is(err, want) {
			t.Errorf("parseWhen(%q) err = %v, want %v", text, err, want)
		}
	}
}

func TestRangeMinutes(t *testing.T) {
	cases := map[[2]string]int{
		{"9:00", "10:30"}: 90,
		{"09:00", "9:15"}: 15,
		{"23:00", "7:00"}: 480, // over midnight
		{"23:30", "0:00"}: 30,
		{"9:00", "9:00"}:  0,
		{"9:00", "24:00"}: 0,
		{"9:00", "10:61"}: 0,
	}
	for r, want := range cases {
		if got := rangeMinutes(r[0], r[1]); got != want {
			t.Errorf("rangeMinutes(%s, %s) = %d, want %d", r[0], r[1], got, want)
		}
	}
}

func TestMoment(t *testing.T) {
	a, _, _ := newTestApp(t, nil, nil)
	now := at(t, "2026-09-29 10:00")
	cases := []struct {
		said  said
		start string // "" if no start
		when  string
	}{
		{said{}, "", ""},
		{said{clock: "18:00"}, "2026-09-29 18:00", ""},
		{said{clock: "9:00"}, "2026-09-30 09:00", ""}, // passed today: tomorrow
		{said{date: "30.09"}, "", "2026-09-30"},
		{said{date: "30.09", clock: "9:00"}, "2026-09-30 09:00", ""},
		{said{date: "29.09", clock: "9:00"}, "2026-09-29 09:00", ""}, // in the past: the dialog asks again
		{said{date: "28.09"}, "", "2027-09-28"},                      // passed this year
		{said{date: "29.02"}, "", "2028-02-29"},                      // the nearest leap year
		{said{date: "1.10.2026", clock: "14:00"}, "2026-10-01 14:00", ""},
		{said{date: "1.10.26"}, "", "2026-10-01"},
	}
	for _, c := range cases {
		start, when := a.moment(c.said, now)
		got := ""
		if start != nil {
			got = start.In(warsaw).Format("2006-01-02 15:04")
		}
		if got != c.start || when != c.when {
			t.Errorf("moment(%+v) = %q, %q; want %q, %q", c.said, got, when, c.start, c.when)
		}
	}
}

func TestAddRefusals(t *testing.T) {
	ctx := context.Background()
	a, send, st := newTestApp(t, withCalendar(), nil)
	now := at(t, "2026-09-29 10:00")

	for _, args := range []string{"", "Приём у врача", "30.09 Приём", "9:00 Приём", "29.09 8:00-9:00 Приём", "31.02 9:00-10:00 Приём"} {
		a.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add " + args})
	}
	usage := "Пример: /add 30.09 9:00-10:00 Приём у врача @ клиника\nДата — числом, время — начало и конец, место — после @."
	incomplete := "Пока нужно указать всё — дату, начало и конец: /add 30.09 9:00-10:00 Приём у врача"
	want := []string{usage, incomplete, incomplete, incomplete, "Это время уже прошло: вт, 29 сен, 08:00", usage}
	if !sameStrings(send.sent, want) {
		t.Fatalf("sent\n%q\nwant\n%q", send.sent, want)
	}
	if _, err := st.PendingFor(ctx, 42); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a refused /add left a draft: %v", err)
	}

	// Without a calendar there is nowhere to add to.
	off, offSend, _ := newTestApp(t, nil, nil)
	off.HandleText(ctx, now, tg.Message{ChatID: 42, Text: "/add 30.09 9:00-10:00 Приём"})
	if !sameStrings(offSend.sent, []string{"Календарь не подключён"}) {
		t.Fatalf("without a calendar: %q", offSend.sent)
	}
}
