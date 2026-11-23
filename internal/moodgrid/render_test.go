package moodgrid

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
	"time"
)

func TestCalendarPlacement(t *testing.T) {
	for _, tc := range []struct {
		year, weeks int
		day         string
		col, row    int
	}{
		{2024, 53, "2024-01-01", 0, 0},
		{2024, 53, "2024-02-29", 8, 3},
		{2024, 53, "2024-12-31", 52, 1},
		{2012, 54, "2012-01-01", 0, 6},
		{2012, 54, "2012-12-31", 53, 0},
		{2026, 53, "2026-12-31", 52, 3},
	} {
		data, err := Render(tc.year, time.Date(tc.year, 12, 31, 0, 0, 0, 0, time.UTC), map[string]int{tc.day: 5})
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		x, y := gridLeft+tc.col*step+cell/2, gridTop+tc.row*step+cell/2
		if img.Bounds().Dx() != gridLeft+tc.weeks*step+40 || color.RGBAModel.Convert(img.At(x, y)) != moods[4] {
			t.Errorf("%s: wrong calendar position or clipped year", tc.day)
		}
	}
}

func TestMissingFutureAndPadding(t *testing.T) {
	// This local Jan 2 is still Jan 1 in UTC; the caller's date must win.
	today := time.Date(2026, 1, 2, 0, 30, 0, 0, time.FixedZone("UTC+14", 14*3600))
	data, err := Render(2026, today, map[string]int{"2026-01-02": 1})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		row, dx int
		want    color.RGBA
	}{{0, 0, background}, {3, cell / 2, missing}, {4, cell / 2, moods[0]}, {5, 0, future}, {5, cell / 2, background}} {
		if got := color.RGBAModel.Convert(img.At(gridLeft+tc.dx, gridTop+tc.row*step+cell/2)); got != tc.want {
			t.Errorf("row %d offset %d = %v, want %v", tc.row, tc.dx, got, tc.want)
		}
	}
}

func TestInvalidRatings(t *testing.T) {
	today := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	for day, value := range map[string]int{"2026-02-29": 3, "2026-11-24": 3, "2025-01-01": 3, "2026-1-01": 3, "2026-01-01": 0, "2026-01-02": 6} {
		if _, err := Render(2026, today, map[string]int{day: value}); err == nil {
			t.Errorf("accepted %s = %d", day, value)
		}
	}
	if _, err := Render(0, today, nil); err == nil {
		t.Fatal("accepted invalid year")
	}
}
