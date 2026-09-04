package store

import (
	"context"
	"database/sql"
	"fmt"
)

// ListMessages returns the latest messages for a chat, oldest-first within the page.
// beforeTS: if > 0, return messages strictly older than that timestamp (pagination).
func (s *Store) ListMessages(ctx context.Context, chatID string, limit int, beforeTS int64) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}

	var (
		rows *sql.Rows
		err  error
	)
	if beforeTS > 0 {
		rows, err = s.db.QueryContext(ctx, `
SELECT id, chat_id, sender, timestamp, text, type, status, quoted_message_id,
       media_id, is_from_me, is_deleted, metadata_json
FROM messages
WHERE chat_id = ? AND timestamp < ?
ORDER BY timestamp DESC
LIMIT ?`, chatID, beforeTS, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT id, chat_id, sender, timestamp, text, type, status, quoted_message_id,
       media_id, is_from_me, is_deleted, metadata_json
FROM messages
WHERE chat_id = ?
ORDER BY timestamp DESC
LIMIT ?`, chatID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	tmp := make([]Message, 0, limit)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		tmp = append(tmp, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(tmp)-1; i < j; i, j = i+1, j-1 {
		tmp[i], tmp[j] = tmp[j], tmp[i]
	}
	return tmp, nil
}

// UpsertMessage inserts or updates a message.
func (s *Store) UpsertMessage(ctx context.Context, m Message) error {
	if m.MetadataJSON == "" {
		m.MetadataJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO messages (
  id, chat_id, sender, timestamp, text, type, status, quoted_message_id,
  media_id, is_from_me, is_deleted, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(chat_id, id) DO UPDATE SET
  sender = excluded.sender,
  timestamp = excluded.timestamp,
  text = excluded.text,
  type = excluded.type,
  status = CASE
    WHEN excluded.status != '' THEN excluded.status
    ELSE messages.status END,
  quoted_message_id = excluded.quoted_message_id,
  media_id = excluded.media_id,
  is_from_me = excluded.is_from_me,
  is_deleted = excluded.is_deleted,
  metadata_json = excluded.metadata_json
`, m.ID, m.ChatID, m.Sender, m.Timestamp, m.Text, m.Type, m.Status,
		nullStr(m.QuotedMessageID), nullStr(m.MediaID),
		boolInt(m.IsFromMe), boolInt(m.IsDeleted), m.MetadataJSON)
	if err != nil {
		return fmt.Errorf("upsert message: %w", err)
	}
	if err := s.syncMessageFTS(ctx, m.ChatID, m.ID, m.Text); err != nil {
		return fmt.Errorf("fts sync: %w", err)
	}
	return nil
}

// GetMessage returns one message by chat + id.
func (s *Store) GetMessage(ctx context.Context, chatID, id string) (Message, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, chat_id, sender, timestamp, text, type, status, quoted_message_id,
       media_id, is_from_me, is_deleted, metadata_json
FROM messages WHERE chat_id = ? AND id = ?`, chatID, id)
	return scanMessage(row)
}

// UpdateMessageStatus sets status for one message.
func (s *Store) UpdateMessageStatus(ctx context.Context, chatID, id, status string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE messages SET status = ? WHERE chat_id = ? AND id = ?`, status, chatID, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// UpdateMessagesStatus sets status for many IDs in one chat.
func (s *Store) UpdateMessagesStatus(ctx context.Context, chatID string, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `UPDATE messages SET status = ? WHERE chat_id = ? AND id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, status, chatID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func scanMessage(row interface{ Scan(dest ...any) error }) (Message, error) {
	var m Message
	var quoted, media sql.NullString
	var fromMe, deleted int
	if err := row.Scan(
		&m.ID, &m.ChatID, &m.Sender, &m.Timestamp, &m.Text, &m.Type, &m.Status,
		&quoted, &media, &fromMe, &deleted, &m.MetadataJSON,
	); err != nil {
		return Message{}, fmt.Errorf("scan message: %w", err)
	}
	if quoted.Valid {
		m.QuotedMessageID = quoted.String
	}
	if media.Valid {
		m.MediaID = media.String
	}
	m.IsFromMe = fromMe != 0
	m.IsDeleted = deleted != 0
	return m, nil
}
