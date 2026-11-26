// Package moodgrid draws a calendar of daily mood ratings, without database or Telegram access.
package moodgrid

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	gridLeft = 96
	gridTop  = 140
	step     = 22
	cell     = 18
)

var (
	background = color.RGBA{250, 248, 244, 255}
	ink        = color.RGBA{45, 57, 55, 255}
	muted      = color.RGBA{103, 115, 111, 255}
	missing    = color.RGBA{214, 220, 216, 255}
	future     = color.RGBA{204, 210, 206, 255}
	moods      = [5]color.RGBA{
		{174, 80, 91, 255}, {203, 143, 68, 255}, {129, 147, 173, 255},
		{72, 158, 152, 255}, {32, 112, 93, 255},
	}
)

// Render returns a PNG with English labels and Monday-first weeks. Ratings are
// keyed by YYYY-MM-DD, with values 1–5. Today uses the caller's local calendar.
// Missing days have no rating; future days are outlined. Padding is left blank.
func Render(year int, today time.Time, ratings map[string]int) ([]byte, error) {
	if year < 1 || year > 9998 || today.IsZero() {
		return nil, fmt.Errorf("mood grid: invalid year or current date")
	}
	todayDay := today.Format(time.DateOnly)
	for day, value := range ratings {
		d, err := time.Parse(time.DateOnly, day)
		if err != nil || d.Format(time.DateOnly) != day || d.Year() != year || day > todayDay || value < 1 || value > 5 {
			return nil, fmt.Errorf("mood grid: invalid rating %q = %d", day, value)
		}
	}
	first := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(1, 0, -1)
	offset := (int(first.Weekday()) + 6) % 7
	weeks := (offset + last.YearDay() + 6) / 7 // Some leap years occupy 54 columns.
	img := image.NewRGBA(image.Rect(0, 0, gridLeft+weeks*step+40, 430))
	box := func(r image.Rectangle, c color.RGBA) {
		draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
	}
	box(img.Bounds(), background)
	ttf, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{Size: 18, DPI: 72})
	if err != nil {
		return nil, err
	}
	defer face.Close()
	title, err := opentype.NewFace(ttf, &opentype.FaceOptions{Size: 34, DPI: 72})
	if err != nil {
		return nil, err
	}
	defer title.Close()
	text := func(x, y int, s string, f font.Face, c color.RGBA) {
		d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	text(40, 54, fmt.Sprintf("Mood · %d", year), title, ink)
	text(40, 86, fmt.Sprintf("%d days recorded · As of %s", len(ratings), today.Format("02 Jan 2006")), face, muted)
	for row, name := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		text(40, gridTop+row*step+15, name, face, muted)
	}
	for d := first; d.Year() == year; d = d.AddDate(0, 0, 1) {
		index := offset + d.YearDay() - 1
		x, y := gridLeft+(index/7)*step, gridTop+(index%7)*step
		if d.Day() == 1 {
			text(x, gridTop-16, d.Format("Jan"), face, muted)
		}
		r := image.Rect(x, y, x+cell, y+cell)
		day := d.Format(time.DateOnly)
		switch {
		case day > todayDay:
			box(r, future)
			box(r.Inset(1), background)
		case ratings[day] != 0:
			box(r, moods[ratings[day]-1])
		default:
			box(r, missing)
		}
	}
	for i, label := range []string{"1  Very low", "2  Low", "3  Okay", "4  Good", "5  Great"} {
		x := 40 + i*245
		box(image.Rect(x, 330, x+cell, 330+cell), moods[i])
		text(x+28, 345, label, face, ink)
	}
	box(image.Rect(40, 382, 40+cell, 382+cell), missing)
	text(68, 397, "No entry", face, muted)
	r := image.Rect(250, 382, 250+cell, 382+cell)
	box(r, future)
	box(r.Inset(1), background)
	text(278, 397, "Future", face, muted)
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("encode mood grid: %w", err)
	}
	return out.Bytes(), nil
}
