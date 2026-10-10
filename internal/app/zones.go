package app

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// zoneRegions are the IANA areas a bare city name is looked up in, most likely first.
var zoneRegions = []string{
	"Europe", "America", "Asia", "Africa", "Australia", "Pacific", "Atlantic", "Indian", "Antarctica", "Arctic",
}

type tzView struct {
	Zone string
	Now  time.Time
}

// handleTZ shows the zone, or with an argument switches it and answers with the time
// there: a typo in the city shows up right away, not as a reminder at the wrong hour.
func (a *App) handleTZ(ctx context.Context, now time.Time, chatID int64, args string) {
	block := "tz_current"
	if args != "" {
		name, ok := resolveZone(args)
		if !ok {
			a.replyBlock(ctx, chatID, "tz_unknown", args)
			return
		}
		if err := a.SetTZ(ctx, name); err != nil {
			a.log.Error("set tz", "err", err)
			return
		}
		block = "tz"
	}
	loc := a.Loc()
	a.replyBlock(ctx, chatID, block, tzView{Zone: loc.String(), Now: now.In(loc)})
}

// resolveZone finds an IANA zone by its name ("America/Toronto") or by a city
// in it ("toronto", "new york"). Russian names come with the model at stage 5.
func resolveZone(input string) (string, bool) {
	// "Local" is the server's zone, "" is UTC: neither is a place the user is in.
	if input == "" || strings.EqualFold(input, "local") {
		return "", false
	}
	name := titleWords(input)
	var candidates []string
	if !strings.Contains(name, "/") {
		// Regions first: "Singapore" alone is an old alias, "Asia/Singapore" the real name.
		for _, region := range zoneRegions {
			candidates = append(candidates, region+"/"+name)
		}
	}
	// As typed last: names like UTC or Etc/GMT-8 do not follow the word capitals.
	candidates = append(candidates, name, input)
	for _, candidate := range candidates {
		if _, err := time.LoadLocation(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// titleWords spells a name the IANA way: every word capitalized, words joined by "_",
// so "new york" becomes "New_York" and "america/toronto" becomes "America/Toronto".
func titleWords(s string) string {
	segments := strings.Split(s, "/")
	for i, segment := range segments {
		words := strings.FieldsFunc(segment, func(r rune) bool { return r == ' ' || r == '_' })
		for j, word := range words {
			r, size := utf8.DecodeRuneInString(word)
			words[j] = string(unicode.ToUpper(r)) + strings.ToLower(word[size:])
		}
		segments[i] = strings.Join(words, "_")
	}
	return strings.Join(segments, "/")
}
