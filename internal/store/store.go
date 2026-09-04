package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// Store is the application SQLite database (never whatsmeow session tables).
type Store struct {
	db *sql.DB
}

// Open opens or creates whatstui.db and applies Phase 1 schema.
func Open(path string) (*Store, error) {
	if err := ensurePrivateFile(path); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app db: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chats (
    id              TEXT PRIMARY KEY NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    is_group        INTEGER NOT NULL DEFAULT 0,
    is_pinned       INTEGER NOT NULL DEFAULT 0,
    is_muted        INTEGER NOT NULL DEFAULT 0,
    is_community    INTEGER NOT NULL DEFAULT 0,
    unread_count    INTEGER NOT NULL DEFAULT 0,
    last_message_at INTEGER NOT NULL DEFAULT 0,
    last_message    TEXT NOT NULL DEFAULT '',
    updated_at      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS contacts (
    id         TEXT PRIMARY KEY NOT NULL,
    push_name  TEXT NOT NULL DEFAULT '',
    full_name  TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS messages (
    id                TEXT NOT NULL,
    chat_id           TEXT NOT NULL,
    sender            TEXT NOT NULL DEFAULT '',
    timestamp         INTEGER NOT NULL DEFAULT 0,
    text              TEXT NOT NULL DEFAULT '',
    type              TEXT NOT NULL DEFAULT 'text',
    status            TEXT NOT NULL DEFAULT '',
    quoted_message_id TEXT,
    media_id          TEXT,
    is_from_me        INTEGER NOT NULL DEFAULT 0,
    is_deleted        INTEGER NOT NULL DEFAULT 0,
    metadata_json     TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (chat_id, id)
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON messages(chat_id, timestamp DESC);

CREATE TABLE IF NOT EXISTS media (
    id            TEXT PRIMARY KEY NOT NULL,
    chat_id       TEXT NOT NULL,
    message_id    TEXT NOT NULL,
    mime_type     TEXT NOT NULL DEFAULT '',
    file_name     TEXT NOT NULL DEFAULT '',
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    local_path    TEXT NOT NULL DEFAULT '',
    download_state TEXT NOT NULL DEFAULT 'pending',
    created_at    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS drafts (
    chat_id    TEXT PRIMARY KEY NOT NULL,
    text       TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS chat_settings (
    chat_id    TEXT PRIMARY KEY NOT NULL,
    pinned     INTEGER NOT NULL DEFAULT 0,
    muted_until INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0,
    pronoun    TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO meta(key, value) VALUES('schema_version', '1');
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate app db: %w", err)
	}
	// Additive migrations for DBs created before these columns existed.
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE chat_settings ADD COLUMN pronoun TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE chats ADD COLUMN is_community INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE chat_settings ADD COLUMN ephemeral_seconds INTEGER NOT NULL DEFAULT 0`)
	// Hide legacy reaction stubs that older builds stored as unsupported rows.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM messages WHERE type = 'other' AND text = 'Unsupported message' AND (media_id IS NULL OR media_id = '')`)
	_, _ = s.db.ExecContext(ctx, `UPDATE chats SET last_message = '' WHERE last_message = 'Unsupported message'`)
	if err := s.ensureFTS(ctx); err != nil {
		return err
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the underlying handle for later repositories.
func (s *Store) DB() *sql.DB { return s.db }

func ensurePrivateFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("ensure db file: %w", err)
	}
	return f.Close()
}
