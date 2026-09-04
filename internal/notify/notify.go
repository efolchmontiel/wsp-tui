package notify

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	chimeMu   sync.Mutex
	lastChime time.Time
)

// Desktop sends a desktop notification via notify-send (no-op if missing)
// and plays a short freedesktop chime (same idea as OpenCode task-done cues).
func Desktop(title, body string) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		title = "WhatsTUI"
	}
	if len([]rune(body)) > 120 {
		r := []rune(body)
		body = string(r[:119]) + "…"
	}
	if _, err := exec.LookPath("notify-send"); err == nil {
		cmd := exec.Command("notify-send", "-a", "WhatsTUI", "-u", "normal", title, body)
		_ = cmd.Start()
	}
	Chime()
}

// Chime plays a short message sound. Debounced so sync bursts don't spam.
func Chime() {
	chimeMu.Lock()
	if time.Since(lastChime) < 1200*time.Millisecond {
		chimeMu.Unlock()
		return
	}
	lastChime = time.Now()
	chimeMu.Unlock()

	go playChime()
}

func playChime() {
	// Prefer XDG sound theme (what canberra / desktop apps use).
	if _, err := exec.LookPath("canberra-gtk-play"); err == nil {
		cmd := exec.Command("canberra-gtk-play", "-i", "message-new-instant")
		if err := cmd.Run(); err == nil {
			return
		}
	}
	path := "/usr/share/sounds/freedesktop/stereo/message-new-instant.oga"
	if _, err := os.Stat(path); err != nil {
		path = "/usr/share/sounds/freedesktop/stereo/message.oga"
		if _, err := os.Stat(path); err != nil {
			return
		}
	}
	if _, err := exec.LookPath("paplay"); err == nil {
		_ = exec.Command("paplay", path).Run()
		return
	}
	if _, err := exec.LookPath("aplay"); err == nil {
		_ = exec.Command("aplay", "-q", path).Run()
	}
}
