package store

import (
	"context"
)

// Pronoun for optional Él/Ella labels (WhatsApp does not provide gender).
type Pronoun string

const (
	PronounAuto Pronoun = ""
	PronounEl   Pronoun = "el"
	PronounElla Pronoun = "ella"
)

// GetChatPronoun returns el/ella preference; empty means use contact name.
func (s *Store) GetChatPronoun(ctx context.Context, chatID string) Pronoun {
	var p string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(pronoun, '') FROM chat_settings WHERE chat_id = ?`, chatID).Scan(&p)
	if err != nil {
		return PronounAuto
	}
	return Pronoun(p)
}

// SetChatPronoun stores el/ella preference for a chat.
func (s *Store) SetChatPronoun(ctx context.Context, chatID string, p Pronoun) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chat_settings (chat_id, pinned, muted_until, archived, pronoun)
VALUES (?, 0, 0, 0, ?)
ON CONFLICT(chat_id) DO UPDATE SET pronoun = excluded.pronoun
`, chatID, string(p))
	return err
}
