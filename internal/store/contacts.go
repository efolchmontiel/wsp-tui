package store

import (
	"context"
	"strings"
)

// ContactDisplayName returns full_name, then push_name, for a contact JID.
func (s *Store) ContactDisplayName(ctx context.Context, id string) string {
	if s == nil || s.db == nil || strings.TrimSpace(id) == "" {
		return ""
	}
	var full, push string
	err := s.db.QueryRowContext(ctx, `
SELECT full_name, push_name FROM contacts WHERE id = ?`, id).Scan(&full, &push)
	if err != nil {
		return ""
	}
	if n := strings.TrimSpace(full); n != "" {
		return n
	}
	return strings.TrimSpace(push)
}
