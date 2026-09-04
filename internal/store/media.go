package store

import (
	"context"
	"fmt"
	"time"
)

// Media download states.
const (
	MediaPending     = "pending"
	MediaDownloading = "downloading"
	MediaReady       = "ready"
	MediaFailed      = "failed"
)

// MediaRow is a filesystem-backed attachment.
type MediaRow struct {
	ID            string
	ChatID        string
	MessageID     string
	MimeType      string
	FileName      string
	SizeBytes     int64
	LocalPath     string
	DownloadState string
	CreatedAt     int64
}

// UpsertMedia inserts or updates a media row.
func (s *Store) UpsertMedia(ctx context.Context, m MediaRow) error {
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().Unix()
	}
	if m.DownloadState == "" {
		m.DownloadState = MediaPending
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO media (id, chat_id, message_id, mime_type, file_name, size_bytes, local_path, download_state, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  mime_type = excluded.mime_type,
  file_name = excluded.file_name,
  size_bytes = excluded.size_bytes,
  local_path = CASE WHEN excluded.local_path != '' THEN excluded.local_path ELSE media.local_path END,
  download_state = excluded.download_state
`, m.ID, m.ChatID, m.MessageID, m.MimeType, m.FileName, m.SizeBytes, m.LocalPath, m.DownloadState, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert media: %w", err)
	}
	return nil
}

// GetMedia returns a media row by id.
func (s *Store) GetMedia(ctx context.Context, id string) (MediaRow, error) {
	var m MediaRow
	err := s.db.QueryRowContext(ctx, `
SELECT id, chat_id, message_id, mime_type, file_name, size_bytes, local_path, download_state, created_at
FROM media WHERE id = ?`, id).Scan(
		&m.ID, &m.ChatID, &m.MessageID, &m.MimeType, &m.FileName, &m.SizeBytes, &m.LocalPath, &m.DownloadState, &m.CreatedAt,
	)
	return m, err
}

// GetMediaByMessage looks up media for a chat message.
func (s *Store) GetMediaByMessage(ctx context.Context, chatID, messageID string) (MediaRow, error) {
	var m MediaRow
	err := s.db.QueryRowContext(ctx, `
SELECT id, chat_id, message_id, mime_type, file_name, size_bytes, local_path, download_state, created_at
FROM media WHERE chat_id = ? AND message_id = ?`, chatID, messageID).Scan(
		&m.ID, &m.ChatID, &m.MessageID, &m.MimeType, &m.FileName, &m.SizeBytes, &m.LocalPath, &m.DownloadState, &m.CreatedAt,
	)
	return m, err
}

// UpdateMediaState sets download state and optional local path.
func (s *Store) UpdateMediaState(ctx context.Context, id, state, localPath string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE media SET download_state = ?,
  local_path = CASE WHEN ? != '' THEN ? ELSE local_path END
WHERE id = ?`, state, localPath, localPath, id)
	return err
}
