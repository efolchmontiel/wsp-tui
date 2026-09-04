package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// SearchHit is one FTS / chat-name match for the Phase 5 search UI.
type SearchHit struct {
	Kind      string // "chat" | "message"
	ChatID    string
	ChatName  string
	MessageID string
	Snippet   string
	Timestamp int64
}

// ensureFTS creates the messages FTS5 index and backfills once.
// Index maintenance is done in UpsertMessage (not triggers) to avoid
// SQLite FTS5 trigger pitfalls on status-only UPDATEs.
func (s *Store) ensureFTS(ctx context.Context) error {
	var ver string
	_ = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&ver)

	if _, err := s.db.ExecContext(ctx, `
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
	text,
	chat_id UNINDEXED,
	message_id UNINDEXED,
	tokenize = 'unicode61'
)`); err != nil {
		return fmt.Errorf("fts create: %w", err)
	}

	// Drop legacy triggers from earlier Phase 5 drafts if present.
	_, _ = s.db.ExecContext(ctx, `DROP TRIGGER IF EXISTS messages_ai`)
	_, _ = s.db.ExecContext(ctx, `DROP TRIGGER IF EXISTS messages_ad`)
	_, _ = s.db.ExecContext(ctx, `DROP TRIGGER IF EXISTS messages_au`)

	if ver != "2" {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM messages_fts`); err != nil {
			return fmt.Errorf("fts clear: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO messages_fts(rowid, text, chat_id, message_id)
SELECT rowid, text, chat_id, id FROM messages WHERE text != ''`); err != nil {
			return fmt.Errorf("fts backfill: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO meta(key, value) VALUES('schema_version', '2')
ON CONFLICT(key) DO UPDATE SET value = excluded.value`); err != nil {
			return fmt.Errorf("schema bump: %w", err)
		}
	}
	return nil
}

// syncMessageFTS upserts one message into the FTS index by table rowid.
// modernc FTS5 rejects the INSERT ...'delete' command; use SQL DELETE instead.
func (s *Store) syncMessageFTS(ctx context.Context, chatID, messageID, text string) error {
	var rowid int64
	err := s.db.QueryRowContext(ctx, `
SELECT rowid FROM messages WHERE chat_id = ? AND id = ?`, chatID, messageID).Scan(&rowid)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, rowid)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO messages_fts(rowid, text, chat_id, message_id) VALUES (?, ?, ?, ?)`,
		rowid, text, chatID, messageID)
	return err
}

// SanitizeFTS turns free-text into a safe FTS5 MATCH query.
// Each token is quoted so operators like AND/OR/* don't break MATCH.
func SanitizeFTS(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := b.String()
		b.Reset()
		tok = strings.ReplaceAll(tok, `"`, "")
		if tok == "" {
			return
		}
		tokens = append(tokens, `"`+tok+`"`)
	}
	for _, r := range q {
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		switch r {
		case '"', '*', '(', ')', ':', '^', '{', '}', '[', ']', '~':
			continue
		default:
			b.WriteRune(r)
		}
	}
	flush()
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, " ")
}

// SearchChats finds chats whose name or last_message matches (LIKE, case-insensitive).
func (s *Store) SearchChats(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	like := "%" + escapeLike(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, last_message, last_message_at
FROM chats
WHERE name LIKE ? ESCAPE '\' OR last_message LIKE ? ESCAPE '\'
ORDER BY is_pinned DESC, last_message_at DESC
LIMIT ?`, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("search chats: %w", err)
	}
	defer rows.Close()

	out := make([]SearchHit, 0, limit)
	for rows.Next() {
		var h SearchHit
		var last string
		if err := rows.Scan(&h.ChatID, &h.ChatName, &last, &h.Timestamp); err != nil {
			return nil, err
		}
		h.Kind = "chat"
		if h.ChatName == "" {
			h.ChatName = h.ChatID
		}
		h.Snippet = last
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchMessages runs FTS5 over message text.
func (s *Store) SearchMessages(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	match := SanitizeFTS(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 40
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT m.chat_id, COALESCE(c.name, ''), m.id, m.timestamp,
       snippet(messages_fts, 0, '«', '»', '…', 14)
FROM messages_fts
JOIN messages m ON m.rowid = messages_fts.rowid
LEFT JOIN chats c ON c.id = m.chat_id
WHERE messages_fts MATCH ?
ORDER BY m.timestamp DESC
LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	out := make([]SearchHit, 0, limit)
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ChatID, &h.ChatName, &h.MessageID, &h.Timestamp, &h.Snippet); err != nil {
			return nil, err
		}
		h.Kind = "message"
		if h.ChatName == "" {
			h.ChatName = h.ChatID
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Search combines chat-name hits and FTS message hits (chats first).
func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 40
	}
	chats, err := s.SearchChats(ctx, query, minInt(10, limit))
	if err != nil {
		return nil, err
	}
	msgs, err := s.SearchMessages(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SearchHit, 0, len(chats)+len(msgs))
	out = append(out, chats...)
	seen := map[string]struct{}{}
	for _, c := range chats {
		seen[c.ChatID+"|"] = struct{}{}
	}
	for _, m := range msgs {
		key := m.ChatID + "|" + m.MessageID
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, m)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
