package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultRetentionIs3Months(t *testing.T) {
	cfg := Default()
	if cfg.LocalRetention != Retention3Months {
		t.Fatalf("default %q", cfg.LocalRetention)
	}
	d, ok := cfg.LocalRetention.Duration()
	if !ok || d != 90*24*time.Hour {
		t.Fatalf("duration %v ok=%v", d, ok)
	}
}

func TestParseRetentionFlexible(t *testing.T) {
	tests := []struct {
		in    string
		label string
		days  int
		never bool
	}{
		{"week", "1 semana", 7, false},
		{"2weeks", "2 semanas", 14, false},
		{"5weeks", "5 semanas", 35, false},
		{"4months", "4 meses", 120, false},
		{"3years", "3 años", 1095, false},
		{"nunca", "nunca", 0, true},
		{"bogus", "3 meses", 90, false},
	}
	for _, tt := range tests {
		got := ParseRetention(tt.in)
		if got.Label() != tt.label {
			t.Fatalf("%q label %q want %q", tt.in, got.Label(), tt.label)
		}
		d, ok := got.Duration()
		if tt.never {
			if ok {
				t.Fatalf("%q should be never", tt.in)
			}
			continue
		}
		if !ok || d != time.Duration(tt.days)*24*time.Hour {
			t.Fatalf("%q duration %v ok=%v", tt.in, d, ok)
		}
	}
}

func TestFormatRetention(t *testing.T) {
	if got := FormatRetention(3, UnitMonth); got != "3months" {
		t.Fatalf("%q", got)
	}
	if got := FormatRetention(1, UnitWeek); got != "1week" {
		t.Fatalf("%q", got)
	}
}

func TestSaveLoadCustomRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := Default()
	cfg.LocalRetention = FormatRetention(5, UnitWeek)
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !EqualRetention(got.LocalRetention, FormatRetention(5, UnitWeek)) {
		t.Fatalf("got %q", got.LocalRetention)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
