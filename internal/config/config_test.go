package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/config"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := config.EnsureDefault(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "dark" || !cfg.Mouse {
		t.Fatalf("%+v", cfg)
	}
	cfg.Theme = "ocean"
	cfg.Mouse = false
	cfg.LocalRetention = config.RetentionWeek
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "ocean" || got.Mouse {
		t.Fatalf("%+v", got)
	}
	if got.LocalRetention != config.RetentionWeek {
		t.Fatalf("retention %+v", got.LocalRetention)
	}
	if d, ok := got.LocalRetention.Duration(); !ok || d != 7*24*time.Hour {
		t.Fatalf("duration %v %v", d, ok)
	}
	if _, ok := config.RetentionNever.Duration(); ok {
		t.Fatal("never must disable purge")
	}
	raw, _ := os.ReadFile(path)
	if len(raw) == 0 {
		t.Fatal("empty file")
	}
}
