package config

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const defaultTOML = `# wsp-tui — WhatsApp en la terminal
# https://github.com/efolchmontiel/wsp-tui

theme = "dark"
mouse = true

# Local cache cleanup (does NOT affect the phone).
# Examples: 1week, 2weeks, 1month, 3months, 6months, 1year, 2years, never
# Or any N + unit: 5weeks, 4months, 3years
local_retention = "3months"

# Inline media/link previews (Kitty / iTerm2 / Sixel / halfblocks).
show_media_previews = true
# auto | kitty | iterm2 | sixel | halfblocks
preview_protocol = "auto"

# Optional — https://developers.giphy.com/dashboard/ (leave empty to use local .gif files only)
giphy_api_key = ""
`

// RetentionPeriod controls how long local messages/media stay on disk.
// Canonical forms: "never" | "{n}week(s)" | "{n}month(s)" | "{n}year(s)".
type RetentionPeriod string

const (
	RetentionNever   RetentionPeriod = "never"
	Retention3Months RetentionPeriod = "3months" // default
)

// RetentionUnit is semana | mes | año.
type RetentionUnit string

const (
	UnitWeek  RetentionUnit = "week"
	UnitMonth RetentionUnit = "month"
	UnitYear  RetentionUnit = "year"
)

// RetentionPresets shown in the settings modal (custom values also allowed).
var RetentionPresets = []RetentionPeriod{
	"1week",
	"2weeks",
	"1month",
	"3months",
	"6months",
	"1year",
	"2years",
	RetentionNever,
}

var retentionRe = regexp.MustCompile(`(?i)^\s*(\d+)\s*(w|week|weeks|semana|semanas|m|month|months|mes|meses|y|year|years|año|años|ano|anos)\s*$`)

// PreviewProtocol selects how inline images are drawn.
type PreviewProtocol string

const (
	PreviewAuto       PreviewProtocol = "auto"
	PreviewKitty      PreviewProtocol = "kitty"
	PreviewITerm2     PreviewProtocol = "iterm2"
	PreviewSixel      PreviewProtocol = "sixel"
	PreviewHalfblocks PreviewProtocol = "halfblocks"
)

// Config is the user-facing preferences loaded from config.toml.
type Config struct {
	Theme             string
	Mouse             bool
	LocalRetention    RetentionPeriod
	ShowMediaPreviews bool
	PreviewProtocol   PreviewProtocol
	GiphyAPIKey       string // optional; empty = local GIF files only
}

// Default returns built-in defaults.
func Default() Config {
	return Config{
		Theme:             "dark",
		Mouse:             true,
		LocalRetention:    Retention3Months,
		ShowMediaPreviews: true,
		PreviewProtocol:   PreviewAuto,
		GiphyAPIKey:       "",
	}
}

// FormatRetention builds a canonical period from amount + unit.
func FormatRetention(n int, unit RetentionUnit) RetentionPeriod {
	if n <= 0 {
		return RetentionNever
	}
	switch unit {
	case UnitWeek:
		if n == 1 {
			return RetentionPeriod("1week")
		}
		return RetentionPeriod(fmt.Sprintf("%dweeks", n))
	case UnitMonth:
		if n == 1 {
			return RetentionPeriod("1month")
		}
		return RetentionPeriod(fmt.Sprintf("%dmonths", n))
	case UnitYear:
		if n == 1 {
			return RetentionPeriod("1year")
		}
		return RetentionPeriod(fmt.Sprintf("%dyears", n))
	default:
		return Retention3Months
	}
}

// ParseRetention normalizes a config value; unknown → 3months.
func ParseRetention(s string) RetentionPeriod {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "default":
		return Retention3Months
	case "never", "off", "none", "nunca":
		return RetentionNever
	case "week", "semana", "7d", "1w":
		return "1week"
	case "month", "mes", "30d", "1m":
		return "1month"
	case "year", "año", "ano", "365d", "1y":
		return "1year"
	}
	if m := retentionRe.FindStringSubmatch(s); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		if n <= 0 {
			return RetentionNever
		}
		unit := parseUnitToken(m[2])
		return FormatRetention(n, unit)
	}
	// Already canonical-ish: 3months, 2weeks, …
	if m := retentionRe.FindStringSubmatch(s + "s"); len(m) == 3 {
		// no-op fallback
	}
	// Accept bare "3months" without regex unit quirks
	for _, suf := range []string{"weeks", "week", "months", "month", "years", "year"} {
		if strings.HasSuffix(s, suf) {
			num := strings.TrimSuffix(s, suf)
			if n, err := strconv.Atoi(num); err == nil && n > 0 {
				return FormatRetention(n, parseUnitToken(suf))
			}
		}
	}
	return Retention3Months
}

func parseUnitToken(tok string) RetentionUnit {
	switch strings.ToLower(tok) {
	case "w", "week", "weeks", "semana", "semanas":
		return UnitWeek
	case "m", "month", "months", "mes", "meses":
		return UnitMonth
	case "y", "year", "years", "año", "años", "ano", "anos":
		return UnitYear
	default:
		return UnitMonth
	}
}

// Parts splits a period into amount + unit. never → (0, "").
func (r RetentionPeriod) Parts() (n int, unit RetentionUnit, never bool) {
	r = ParseRetention(string(r))
	if r == RetentionNever {
		return 0, "", true
	}
	s := string(r)
	for _, suf := range []struct {
		s string
		u RetentionUnit
	}{
		{"weeks", UnitWeek}, {"week", UnitWeek},
		{"months", UnitMonth}, {"month", UnitMonth},
		{"years", UnitYear}, {"year", UnitYear},
	} {
		if strings.HasSuffix(s, suf.s) {
			num := strings.TrimSuffix(s, suf.s)
			if v, err := strconv.Atoi(num); err == nil && v > 0 {
				return v, suf.u, false
			}
		}
	}
	return 3, UnitMonth, false
}

// Duration returns how long to keep local data. ok=false means never purge.
func (r RetentionPeriod) Duration() (d time.Duration, ok bool) {
	n, unit, never := ParseRetention(string(r)).Parts()
	if never {
		return 0, false
	}
	day := 24 * time.Hour
	switch unit {
	case UnitWeek:
		return time.Duration(n) * 7 * day, true
	case UnitMonth:
		return time.Duration(n) * 30 * day, true
	case UnitYear:
		return time.Duration(n) * 365 * day, true
	default:
		return 90 * day, true
	}
}

// Label is a short Spanish label for UI/logs.
func (r RetentionPeriod) Label() string {
	n, unit, never := ParseRetention(string(r)).Parts()
	if never {
		return "nunca"
	}
	switch unit {
	case UnitWeek:
		if n == 1 {
			return "1 semana"
		}
		return fmt.Sprintf("%d semanas", n)
	case UnitMonth:
		if n == 1 {
			return "1 mes"
		}
		return fmt.Sprintf("%d meses", n)
	case UnitYear:
		if n == 1 {
			return "1 año"
		}
		return fmt.Sprintf("%d años", n)
	default:
		return "3 meses"
	}
}

// EqualRetention compares normalized periods.
func EqualRetention(a, b RetentionPeriod) bool {
	return ParseRetention(string(a)) == ParseRetention(string(b))
}

// ParsePreviewProtocol normalizes preview_protocol; unknown → auto.
func ParsePreviewProtocol(s string) PreviewProtocol {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "kitty":
		return PreviewKitty
	case "iterm2", "iterm", "imgcat":
		return PreviewITerm2
	case "sixel":
		return PreviewSixel
	case "halfblocks", "halfblock", "blocks", "ansi":
		return PreviewHalfblocks
	default:
		return PreviewAuto
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
		case "show_media_previews", "media_previews", "show_images":
			if b, err := strconv.ParseBool(val); err == nil {
				cfg.ShowMediaPreviews = b
			}
		case "preview_protocol", "image_protocol":
			cfg.PreviewProtocol = ParsePreviewProtocol(val)
		case "giphy_api_key", "giphy_key":
			cfg.GiphyAPIKey = val
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
	cfg.LocalRetention = ParseRetention(string(cfg.LocalRetention))
	cfg.PreviewProtocol = ParsePreviewProtocol(string(cfg.PreviewProtocol))
	body := fmt.Sprintf(
		`# wsp-tui — WhatsApp en la terminal
# https://github.com/efolchmontiel/wsp-tui

theme = %q
mouse = %v

# Local cache cleanup (does NOT affect the phone).
# Any N+unit: 1week, 2weeks, 3months, 1year, 5years, never
local_retention = %q

# Inline media/link previews (Kitty / iTerm2 / Sixel / halfblocks).
show_media_previews = %v
# auto | kitty | iterm2 | sixel | halfblocks
preview_protocol = %q

# Optional Giphy search (https://developers.giphy.com/dashboard/)
# Leave empty to pick local .gif files only.
giphy_api_key = %q
`,
		cfg.Theme, cfg.Mouse, string(cfg.LocalRetention),
		cfg.ShowMediaPreviews, string(cfg.PreviewProtocol),
		cfg.GiphyAPIKey,
	)
	return os.WriteFile(path, []byte(body), 0o600)
}
