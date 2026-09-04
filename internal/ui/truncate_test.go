package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestTruncateANSIWidth(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render("[Todos] Fav Grupos Estados Noved Arch ·1-6")
	got := truncate(styled, 14)
	if w := ansi.StringWidth(got); w > 14 {
		t.Fatalf("width %d > 14 for %q", w, got)
	}
}

func TestTruncateEmojiWidth(t *testing.T) {
	got := truncate("Familia 🐷🐷 chancho", 10)
	if ansi.StringWidth(got) > 10 {
		t.Fatalf("width %d for %q", ansi.StringWidth(got), got)
	}
}

func TestTruncatePlainShort(t *testing.T) {
	if got := truncate("hola", 10); got != "hola" {
		t.Fatalf("got %q", got)
	}
}
