package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	// DefaultRetention is used when callers pass a zero duration with purge enabled.
	// Prefer passing an explicit duration from config.LocalRetention.
	DefaultRetention = 90 * 24 * time.Hour

	// purgeMinInterval avoids repeating the scan on every short restart.
	purgeMinInterval = 24 * time.Hour

	metaLastPurge = "last_purge_at"
)

// Deprecated: use config.LocalRetention + PurgeOlderThan retention argument.
const Retention = DefaultRetention

// PurgeStats summarizes a local retention cleanup.
type PurgeStats struct {
	Messages int
	Media    int
	Files    int
	Skipped  bool // true when last purge was too recent or retention is never
	Cutoff   int64
	Label    string
}

// ShouldRunPurge reports whether a retention pass is due.
func (s *Store) ShouldRunPurge(ctx context.Context, now time.Time) bool {
	raw, err := s.getMeta(ctx, metaLastPurge)
	if err != nil || raw == "" {
		return true
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return true
	}
	return now.Sub(time.Unix(sec, 0)) >= purgeMinInterval
}

// PurgeOlderThan deletes local messages and media older than keep.
// Favorites and archived chats are included. The phone copy is untouched.
// When mediaDir is set, old unreferenced files under it are removed too.
func (s *Store) PurgeOlderThan(ctx context.Context, now time.Time, mediaDir string, keep time.Duration) (PurgeStats, error) {
	if keep <= 0 {
		keep = DefaultRetention
	}
	stats := PurgeStats{Cutoff: now.Add(-keep).Unix()}

	paths, err := s.listMediaPathsToPurge(ctx, stats.Cutoff)
	if err != nil {
		return stats, err
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if err := os.Remove(p); err == nil {
			stats.Files++
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, err
	}
	defer func() { _ = tx.Rollback() }()

	oldRows, err := tx.QueryContext(ctx, `
SELECT rowid FROM messages WHERE timestamp > 0 AND timestamp < ?`, stats.Cutoff)
	if err != nil {
		return stats, fmt.Errorf("purge list: %w", err)
	}
	var oldRowIDs []int64
	for oldRows.Next() {
		var id int64
		if err := oldRows.Scan(&id); err != nil {
			_ = oldRows.Close()
			return stats, err
		}
		oldRowIDs = append(oldRowIDs, id)
	}
	err = oldRows.Err()
	_ = oldRows.Close()
	if err != nil {
		return stats, err
	}
	for _, id := range oldRowIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, id); err != nil {
			return stats, fmt.Errorf("purge fts: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx, `
DELETE FROM messages WHERE timestamp > 0 AND timestamp < ?`, stats.Cutoff)
	if err != nil {
		return stats, fmt.Errorf("purge messages: %w", err)
	}
	if n, e := res.RowsAffected(); e == nil {
		stats.Messages = int(n)
	}

	res, err = tx.ExecContext(ctx, `
DELETE FROM media WHERE created_at > 0 AND created_at < ?`, stats.Cutoff)
	if err != nil {
		return stats, fmt.Errorf("purge media by age: %w", err)
	}
	if n, e := res.RowsAffected(); e == nil {
		stats.Media += int(n)
	}

	res, err = tx.ExecContext(ctx, `
DELETE FROM media WHERE NOT EXISTS (
  SELECT 1 FROM messages m
  WHERE m.chat_id = media.chat_id AND m.id = media.message_id
)`)
	if err != nil {
		return stats, fmt.Errorf("purge orphan media: %w", err)
	}
	if n, e := res.RowsAffected(); e == nil {
		stats.Media += int(n)
	}

	if err := refreshChatPreviewsTx(ctx, tx); err != nil {
		return stats, err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO meta(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		metaLastPurge, strconv.FormatInt(now.Unix(), 10)); err != nil {
		return stats, err
	}

	if err := tx.Commit(); err != nil {
		return stats, err
	}

	if mediaDir != "" {
		n, _ := s.pruneUnreferencedMediaFiles(ctx, mediaDir, stats.Cutoff)
		stats.Files += n
	}
	return stats, nil
}

// MaybePurgeOlderThan runs PurgeOlderThan at most once per purgeMinInterval.
// If enabled is false (retention = never), it skips entirely.
func (s *Store) MaybePurgeOlderThan(ctx context.Context, now time.Time, mediaDir string, keep time.Duration, enabled bool) (PurgeStats, error) {
	if !enabled || keep <= 0 {
		return PurgeStats{Skipped: true, Label: "nunca"}, nil
	}
	if !s.ShouldRunPurge(ctx, now) {
		return PurgeStats{Skipped: true, Cutoff: now.Add(-keep).Unix()}, nil
	}
	return s.PurgeOlderThan(ctx, now, mediaDir, keep)
}

func (s *Store) listMediaPathsToPurge(ctx context.Context, cutoff int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT local_path FROM media
WHERE local_path != '' AND (
  (created_at > 0 AND created_at < ?)
  OR NOT EXISTS (
    SELECT 1 FROM messages m
    WHERE m.chat_id = media.chat_id AND m.id = media.message_id
  )
  OR EXISTS (
    SELECT 1 FROM messages m
    WHERE m.chat_id = media.chat_id AND m.id = media.message_id
      AND m.timestamp > 0 AND m.timestamp < ?
  )
)`, cutoff, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func refreshChatPreviewsTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM chats`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().Unix()
	for _, id := range ids {
		var text string
		var ts int64
		err := tx.QueryRowContext(ctx, `
SELECT text, timestamp FROM messages
WHERE chat_id = ? ORDER BY timestamp DESC LIMIT 1`, id).Scan(&text, &ts)
		switch {
		case err == sql.ErrNoRows:
			if _, err := tx.ExecContext(ctx, `
UPDATE chats SET last_message = '', last_message_at = 0, unread_count = 0, updated_at = ? WHERE id = ?`,
				now, id); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, `
UPDATE chats SET last_message = ?, last_message_at = ?, updated_at = ? WHERE id = ?`,
				text, ts, now, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) getMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// pruneUnreferencedMediaFiles removes old files under mediaDir that are not
// referenced by any remaining media row.
func (s *Store) pruneUnreferencedMediaFiles(ctx context.Context, mediaDir string, cutoff int64) (int, error) {
	keep := map[string]struct{}{}
	rows, err := s.db.QueryContext(ctx, `SELECT local_path FROM media WHERE local_path != ''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return 0, err
		}
		keep[p] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	cutoffTime := time.Unix(cutoff, 0)
	n := 0
	_ = filepath.Walk(mediaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if _, ok := keep[path]; ok {
			return nil
		}
		if info.ModTime().Before(cutoffTime) {
			if remErr := os.Remove(path); remErr == nil {
				n++
			}
		}
		return nil
	})
	return n, nil
}
