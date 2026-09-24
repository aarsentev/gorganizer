package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	TZ         string            `yaml:"tz"`
	Anchors    map[string]string `yaml:"anchors"`
	QuietHours struct {
		From string `yaml:"from"`
		To   string `yaml:"to"`
	} `yaml:"quiet_hours"`
	Reminders struct {
		EventOffsetsMin []int  `yaml:"event_offsets_min"`
		DayBeforeAt     string `yaml:"day_before_at"`
	} `yaml:"reminders"`

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
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if _, err := time.LoadLocation(c.TZ); err != nil {
		return nil, fmt.Errorf("tz %q: %w", c.TZ, err)
	}
	if err := c.checkClocks(); err != nil {
		return nil, err
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
	return &c, nil
}

func (c *Config) checkClocks() error {
	clocks := map[string]string{
		"quiet_hours.from":        c.QuietHours.From,
		"quiet_hours.to":          c.QuietHours.To,
		"reminders.day_before_at": c.Reminders.DayBeforeAt,
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
