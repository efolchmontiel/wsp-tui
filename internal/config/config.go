package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultTOML = `# wsp-tui — WhatsApp en la terminal
# https://github.com/efolchmontiel/wsp-tui

theme = "dark"
mouse = true

# Local cache cleanup (does NOT affect the phone).
# week | month | 3months | year | never
local_retention = "3months"
`

// RetentionPeriod controls how long local messages/media stay on disk.
type RetentionPeriod string

const (
	RetentionWeek    RetentionPeriod = "week"
	RetentionMonth   RetentionPeriod = "month"
	Retention3Months RetentionPeriod = "3months"
	RetentionYear    RetentionPeriod = "year"
	RetentionNever   RetentionPeriod = "never"
)

// Config is the user-facing preferences loaded from config.toml.
type Config struct {
	Theme           string
	Mouse           bool
	LocalRetention  RetentionPeriod
}

// Default returns built-in defaults.
func Default() Config {
	return Config{
		Theme:          "dark",
		Mouse:          true,
		LocalRetention: Retention3Months,
	}
}

// ParseRetention normalizes a config value; unknown → 3months.
func ParseRetention(s string) RetentionPeriod {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "week", "semana", "7d":
		return RetentionWeek
	case "month", "mes", "30d":
		return RetentionMonth
	case "3months", "3month", "90d", "default":
		return Retention3Months
	case "year", "año", "ano", "365d":
		return RetentionYear
	case "never", "off", "none", "nunca":
		return RetentionNever
	default:
		return Retention3Months
	}
}

// Duration returns how long to keep local data. ok=false means never purge.
func (r RetentionPeriod) Duration() (d time.Duration, ok bool) {
	switch ParseRetention(string(r)) {
	case RetentionWeek:
		return 7 * 24 * time.Hour, true
	case RetentionMonth:
		return 30 * 24 * time.Hour, true
	case Retention3Months:
		return 90 * 24 * time.Hour, true
	case RetentionYear:
		return 365 * 24 * time.Hour, true
	case RetentionNever:
		return 0, false
	default:
		return 90 * 24 * time.Hour, true
	}
}

// Label is a short Spanish label for UI/logs.
func (r RetentionPeriod) Label() string {
	switch ParseRetention(string(r)) {
	case RetentionWeek:
		return "1 semana"
	case RetentionMonth:
		return "1 mes"
	case Retention3Months:
		return "3 meses"
	case RetentionYear:
		return "1 año"
	case RetentionNever:
		return "nunca"
	default:
		return "3 meses"
	}
}

// EnsureDefault writes a minimal config.toml if it does not exist.
func EnsureDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultTOML), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// Load reads config.toml (best-effort key=value TOML subset).
func Load(path string) (Config, error) {
	cfg := Default()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		switch key {
		case "theme":
			if val != "" {
				cfg.Theme = val
			}
		case "mouse":
			if b, err := strconv.ParseBool(val); err == nil {
				cfg.Mouse = b
			}
		case "local_retention", "retention":
			cfg.LocalRetention = ParseRetention(val)
		}
	}
	if err := sc.Err(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes preferences back to config.toml (overwrites file).
func Save(path string, cfg Config) error {
	if cfg.Theme == "" {
		cfg.Theme = "dark"
	}
	if cfg.LocalRetention == "" {
		cfg.LocalRetention = Retention3Months
	}
	body := fmt.Sprintf(
		`# wsp-tui — WhatsApp en la terminal
# https://github.com/efolchmontiel/wsp-tui

theme = %q
mouse = %v

# Local cache cleanup (does NOT affect the phone).
# week | month | 3months | year | never
local_retention = %q
`,
		cfg.Theme, cfg.Mouse, string(ParseRetention(string(cfg.LocalRetention))),
	)
	return os.WriteFile(path, []byte(body), 0o600)
}
