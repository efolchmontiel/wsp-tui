package store

import (
	"context"
	"fmt"
	"time"
)

// DeleteChat removes a chat and related local rows (messages, media, FTS, settings, drafts).
// This is a local delete only — it does not remove the conversation on WhatsApp servers.
func (s *Store) DeleteChat(ctx context.Context, chatID string) error {
	if chatID == "" {
		return fmt.Errorf("empty chat id")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Drop FTS rows for this chat (best-effort; table may be empty).
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages_fts(messages_fts, rowid)
SELECT 'delete', rowid FROM messages WHERE chat_id = ?`, chatID); err != nil {
		// Fallback: ignore if FTS missing mid-migration
		_, _ = tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE chat_id = ?`, chatID)
	}

	stmts := []string{
		`DELETE FROM media WHERE chat_id = ?`,
		`DELETE FROM messages WHERE chat_id = ?`,
		`DELETE FROM chat_settings WHERE chat_id = ?`,
		`DELETE FROM drafts WHERE chat_id = ?`,
		`DELETE FROM chats WHERE id = ?`,
	}
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q, chatID); err != nil {
			return fmt.Errorf("delete chat: %w", err)
		}
	}
	return tx.Commit()
}

// UpsertLocalContact stores a contact label in the app DB (display cache).
func (s *Store) UpsertLocalContact(ctx context.Context, id, fullName, pushName string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO contacts (id, push_name, full_name, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  push_name = CASE WHEN excluded.push_name != '' THEN excluded.push_name ELSE contacts.push_name END,
  full_name = CASE WHEN excluded.full_name != '' THEN excluded.full_name ELSE contacts.full_name END,
  updated_at = excluded.updated_at
`, id, pushName, fullName, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("upsert contact: %w", err)
	}
	return nil
}
