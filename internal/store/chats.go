package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ListChats returns non-archived chats ordered by pinned then recent activity.
func (s *Store) ListChats(ctx context.Context, limit int) ([]Chat, error) {
	return s.ListChatsFiltered(ctx, FilterAll, limit)
}

// ListChatsFiltered returns chats for a sidebar filter tab.
func (s *Store) ListChatsFiltered(ctx context.Context, filter ChatFilter, limit int) ([]Chat, error) {
	if limit <= 0 {
		limit = 200
	}
	// Communities never appear in Todos / Favoritos / Grupos — only in Novedades (or Archivados).
	where := "COALESCE(s.archived, 0) = 0 AND COALESCE(c.is_community, 0) = 0"
	switch filter {
	case FilterFavorites:
		where = "COALESCE(s.archived, 0) = 0 AND COALESCE(c.is_community, 0) = 0 AND c.is_pinned = 1"
	case FilterGroups:
		where = "COALESCE(s.archived, 0) = 0 AND COALESCE(c.is_community, 0) = 0 AND c.is_group = 1"
	case FilterNovedades:
		where = "COALESCE(s.archived, 0) = 0 AND COALESCE(c.is_community, 0) = 1"
	case FilterArchived:
		where = "COALESCE(s.archived, 0) = 1"
	}

	q := `
SELECT c.id, c.name, c.is_group, c.is_pinned, c.is_muted, c.unread_count,
       c.last_message_at, c.last_message, c.updated_at, COALESCE(s.archived, 0),
       COALESCE(c.is_community, 0)
FROM chats c
LEFT JOIN chat_settings s ON s.chat_id = c.id
WHERE ` + where + `
ORDER BY c.is_pinned DESC, c.last_message_at DESC, c.name ASC
LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	out := make([]Chat, 0, 64)
	for rows.Next() {
		var c Chat
		var isGroup, isPinned, isMuted, archived, isCommunity int
		if err := rows.Scan(
			&c.ID, &c.Name, &isGroup, &isPinned, &isMuted, &c.UnreadCount,
			&c.LastMessageAt, &c.LastMessage, &c.UpdatedAt, &archived, &isCommunity,
		); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		c.IsGroup = isGroup != 0
		c.IsPinned = isPinned != 0
		c.IsMuted = isMuted != 0
		c.IsArchived = archived != 0
		c.IsCommunity = isCommunity != 0
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dedupeOneToOneChats(out), nil
}

// dedupeOneToOneChats collapses LID + phone JID duplicates that share the same
// display phone (e.g. two "+56 9 …" rows for one contact).
func dedupeOneToOneChats(in []Chat) []Chat {
	if len(in) < 2 {
		return in
	}
	best := map[string]int{} // key → index in out
	out := make([]Chat, 0, len(in))
	for _, c := range in {
		if c.IsGroup || c.IsCommunity {
			out = append(out, c)
			continue
		}
		key := chatDedupeKey(c)
		if key == "" {
			out = append(out, c)
			continue
		}
		if prev, ok := best[key]; ok {
			if preferChat(c, out[prev]) {
				out[prev] = c
			}
			continue
		}
		best[key] = len(out)
		out = append(out, c)
	}
	return out
}

func chatDedupeKey(c Chat) string {
	digits := onlyDigits(c.Name)
	if len(digits) >= 8 {
		return digits
	}
	digits = onlyDigits(c.ID)
	if len(digits) >= 8 {
		return digits
	}
	return ""
}

func onlyDigits(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			b = append(b, ch)
		}
	}
	return string(b)
}

func preferChat(a, b Chat) bool {
	// Prefer classic WhatsApp phone JIDs over LID aliases.
	aPN := strings.Contains(a.ID, "@s.whatsapp.net")
	bPN := strings.Contains(b.ID, "@s.whatsapp.net")
	if aPN != bPN {
		return aPN
	}
	if a.LastMessageAt != b.LastMessageAt {
		return a.LastMessageAt > b.LastMessageAt
	}
	if a.UnreadCount != b.UnreadCount {
		return a.UnreadCount > b.UnreadCount
	}
	return a.IsPinned && !b.IsPinned
}

// SetChatArchived marks a conversation archived (hidden from Todos).
func (s *Store) SetChatArchived(ctx context.Context, chatID string, archived bool) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chat_settings (chat_id, pinned, muted_until, archived, pronoun)
VALUES (?, 0, 0, ?, '')
ON CONFLICT(chat_id) DO UPDATE SET archived = excluded.archived
`, chatID, boolInt(archived))
	return err
}

// IsChatArchived reports archive flag.
func (s *Store) IsChatArchived(ctx context.Context, chatID string) bool {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(archived, 0) FROM chat_settings WHERE chat_id = ?`, chatID).Scan(&v)
	return err == nil && v != 0
}

// SetChatFavorite toggles the pinned/favorite flag on the chats row.
func (s *Store) SetChatFavorite(ctx context.Context, chatID string, fav bool) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE chats SET is_pinned = ?, updated_at = ? WHERE id = ?`, boolInt(fav), time.Now().Unix(), chatID)
	return err
}

// UpsertChat inserts or updates a chat row.
func (s *Store) UpsertChat(ctx context.Context, c Chat) error {
	if c.UpdatedAt == 0 {
		c.UpdatedAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chats (
  id, name, is_group, is_pinned, is_muted, is_community, unread_count,
  last_message_at, last_message, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name = CASE WHEN excluded.name != '' THEN excluded.name ELSE chats.name END,
  is_group = excluded.is_group,
  is_pinned = excluded.is_pinned,
  is_muted = excluded.is_muted,
  is_community = CASE
    WHEN excluded.is_community != 0 THEN 1
    ELSE chats.is_community END,
  unread_count = excluded.unread_count,
  last_message_at = CASE
    WHEN excluded.last_message_at >= chats.last_message_at THEN excluded.last_message_at
    ELSE chats.last_message_at END,
  last_message = CASE
    WHEN excluded.last_message_at >= chats.last_message_at AND excluded.last_message != ''
      THEN excluded.last_message
    ELSE chats.last_message END,
  updated_at = excluded.updated_at
`, c.ID, c.Name, boolInt(c.IsGroup), boolInt(c.IsPinned), boolInt(c.IsMuted), boolInt(c.IsCommunity),
		c.UnreadCount, c.LastMessageAt, c.LastMessage, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert chat: %w", err)
	}
	return nil
}

// TouchChatPreview updates last message preview without wiping other fields.
func (s *Store) TouchChatPreview(ctx context.Context, chatID, preview string, ts int64, unreadDelta int) error {
	now := time.Now().Unix()
	unread := unreadDelta
	if unread < 0 {
		unread = 0
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chats (id, name, last_message_at, last_message, unread_count, updated_at)
VALUES (?, '', ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  last_message_at = CASE WHEN excluded.last_message_at >= chats.last_message_at THEN excluded.last_message_at ELSE chats.last_message_at END,
  last_message = CASE WHEN excluded.last_message_at >= chats.last_message_at THEN excluded.last_message ELSE chats.last_message END,
  unread_count = CASE WHEN ? > 0 THEN chats.unread_count + ? ELSE chats.unread_count END,
  updated_at = excluded.updated_at
`, chatID, ts, preview, unread, now, unreadDelta, unreadDelta)
	if err != nil {
		return fmt.Errorf("touch chat: %w", err)
	}
	return nil
}

// SetChatName updates display name / group flag without touching counters.
func (s *Store) SetChatName(ctx context.Context, id, name string, isGroup bool) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE chats SET
  name = CASE WHEN ? != '' THEN ? ELSE name END,
  is_group = ?,
  updated_at = ?
WHERE id = ?`, name, name, boolInt(isGroup), time.Now().Unix(), id)
	return err
}

// SetChatNameIfEmpty only fills name when the current value is empty.
func (s *Store) SetChatNameIfEmpty(ctx context.Context, id, name string, isGroup bool) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE chats SET
  name = CASE WHEN name = '' OR name IS NULL THEN ? ELSE name END,
  is_group = ?,
  updated_at = ?
WHERE id = ?`, name, boolInt(isGroup), time.Now().Unix(), id)
	return err
}

// ClearUnread sets unread_count to 0.
func (s *Store) ClearUnread(ctx context.Context, chatID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE chats SET unread_count = 0, updated_at = ? WHERE id = ?`, time.Now().Unix(), chatID)
	return err
}

// SetChatCommunity marks a chat as a WhatsApp community (parent or linked subgroup).
func (s *Store) SetChatCommunity(ctx context.Context, id string, isCommunity bool) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE chats SET is_community = ?, is_group = CASE WHEN ? != 0 THEN 1 ELSE is_group END, updated_at = ?
WHERE id = ?`, boolInt(isCommunity), boolInt(isCommunity), time.Now().Unix(), id)
	return err
}

// GetChat returns one chat or sql.ErrNoRows.
func (s *Store) GetChat(ctx context.Context, id string) (Chat, error) {
	var c Chat
	var isGroup, isPinned, isMuted, archived, isCommunity int
	err := s.db.QueryRowContext(ctx, `
SELECT c.id, c.name, c.is_group, c.is_pinned, c.is_muted, c.unread_count,
       c.last_message_at, c.last_message, c.updated_at, COALESCE(s.archived, 0),
       COALESCE(c.is_community, 0)
FROM chats c
LEFT JOIN chat_settings s ON s.chat_id = c.id
WHERE c.id = ?`, id).Scan(
		&c.ID, &c.Name, &isGroup, &isPinned, &isMuted, &c.UnreadCount,
		&c.LastMessageAt, &c.LastMessage, &c.UpdatedAt, &archived, &isCommunity,
	)
	if err != nil {
		return Chat{}, err
	}
	c.IsGroup = isGroup != 0
	c.IsPinned = isPinned != 0
	c.IsMuted = isMuted != 0
	c.IsArchived = archived != 0
	c.IsCommunity = isCommunity != 0
	return c, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// EnsureChatExists creates a minimal chat row if missing.
func (s *Store) EnsureChatExists(ctx context.Context, id, name string, isGroup bool) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chats (id, name, is_group, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, id, name, boolInt(isGroup), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("ensure chat: %w", err)
	}
	return nil
}

// ChatCount is a small helper for tests/metrics.
func (s *Store) ChatCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chats`).Scan(&n)
	return n, err
}

// DBPing verifies the handle is alive.
func (s *Store) DBPing(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// ErrNoRows re-export for callers.
var ErrNoRows = sql.ErrNoRows
