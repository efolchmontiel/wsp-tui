package store

import (
	"context"
	"time"
)

// GetChatEphemeralSeconds returns the locally remembered disappearing timer (0 = off).
func (s *Store) GetChatEphemeralSeconds(ctx context.Context, chatID string) int {
	var sec int
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(ephemeral_seconds, 0) FROM chat_settings WHERE chat_id = ?`, chatID).Scan(&sec)
	if err != nil {
		return 0
	}
	return sec
}

// SetChatEphemeralSeconds stores the disappearing timer for a chat.
func (s *Store) SetChatEphemeralSeconds(ctx context.Context, chatID string, seconds int) error {
	if seconds < 0 {
		seconds = 0
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chat_settings (chat_id, pinned, muted_until, archived, pronoun, ephemeral_seconds)
VALUES (?, 0, 0, 0, '', ?)
ON CONFLICT(chat_id) DO UPDATE SET ephemeral_seconds = excluded.ephemeral_seconds
`, chatID, seconds)
	return err
}

// EphemeralCycle is Off → 24h → 7d → 90d → Off.
func EphemeralCycle(currentSec int) (next time.Duration, label string) {
	switch {
	case currentSec <= 0:
		return 24 * time.Hour, "24 horas"
	case currentSec <= int((24 * time.Hour).Seconds()):
		return 7 * 24 * time.Hour, "7 días"
	case currentSec <= int((7 * 24 * time.Hour).Seconds()):
		return 90 * 24 * time.Hour, "90 días"
	default:
		return 0, "desactivado"
	}
}
