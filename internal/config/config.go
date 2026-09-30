package config

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Languages the interface is designed for. Only ru has texts so far, the rest fall back to it.
var Languages = []string{"ru", "en", "fr", "de", "pl"}

type Config struct {
	TZ         string            `yaml:"tz"`
	Lang       string            `yaml:"lang"`
	Anchors    map[string]string `yaml:"anchors"`
	QuietHours struct {
		From string `yaml:"from"`
		To   string `yaml:"to"`
	} `yaml:"quiet_hours"`
	Reminders struct {
		EventOffsetsMin []int  `yaml:"event_offsets_min"`
		EveningAt       string `yaml:"evening_at"`
		Cleanup         bool   `yaml:"cleanup"`
	} `yaml:"reminders"`
	Digest struct {
		At string `yaml:"at"`
	} `yaml:"digest"`

	BotToken   string `yaml:"-"`
	ChatID     int64  `yaml:"-"`
	DBPath     string `yaml:"-"`
	CalendarID string `yaml:"-"`
	SAFile     string `yaml:"-"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	if c.BotToken, err = env("TELEGRAM_TOKEN"); err != nil {
		return nil, err
	}
	raw, err := env("TELEGRAM_CHAT_ID")
	if err != nil {
		return nil, err
	}
	if c.ChatID, err = strconv.ParseInt(raw, 10, 64); err != nil {
		return nil, fmt.Errorf("TELEGRAM_CHAT_ID: %w", err)
	}
	if c.CalendarID, err = env("CALENDAR_ID"); err != nil {
		return nil, err
	}
	if c.SAFile, err = env("GOOGLE_SA_FILE"); err != nil {
		return nil, err
	}
	c.DBPath = envOr("DB_PATH", "organizer.db")
	return c, nil
}

// parse reads and validates the yaml part. Unknown keys are errors: a stale
// config on the server must fail at startup, not silently lose a setting.
func parse(b []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	if _, err := time.LoadLocation(c.TZ); err != nil {
		return nil, fmt.Errorf("tz %q: %w", c.TZ, err)
	}
	if !slices.Contains(Languages, c.Lang) {
		return nil, fmt.Errorf("lang %q: want one of %v", c.Lang, Languages)
	}
	for _, m := range c.Reminders.EventOffsetsMin {
		if m <= 0 {
			return nil, fmt.Errorf("reminders.event_offsets_min: %d is not positive", m)
		}
	}
	if err := c.checkClocks(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) checkClocks() error {
	clocks := map[string]string{
		"quiet_hours.from":     c.QuietHours.From,
		"quiet_hours.to":       c.QuietHours.To,
		"reminders.evening_at": c.Reminders.EveningAt,
		"digest.at":            c.Digest.At,
	}
	for k, v := range c.Anchors {
		clocks["anchors."+k] = v
	}
	for k, v := range clocks {
		if _, err := time.Parse("15:04", v); err != nil {
			return fmt.Errorf("%s %q: want HH:MM", k, v)
		}
	}
	return nil
}

func env(k string) (string, error) {
	v := os.Getenv(k)
	if v == "" {
		return "", fmt.Errorf("missing env %s", k)
	}
	return v, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
