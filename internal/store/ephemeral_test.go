package store_test

import (
	"testing"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestEphemeralCycle(t *testing.T) {
	cases := []struct {
		cur   int
		want  time.Duration
		label string
	}{
		{0, 24 * time.Hour, "24 horas"},
		{int((24 * time.Hour).Seconds()), 7 * 24 * time.Hour, "7 días"},
		{int((7 * 24 * time.Hour).Seconds()), 90 * 24 * time.Hour, "90 días"},
		{int((90 * 24 * time.Hour).Seconds()), 0, "desactivado"},
	}
	for _, tt := range cases {
		got, label := store.EphemeralCycle(tt.cur)
		if got != tt.want || label != tt.label {
			t.Fatalf("cur=%d got=%v/%q want=%v/%q", tt.cur, got, label, tt.want, tt.label)
		}
	}
}
