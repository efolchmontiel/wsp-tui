package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Reaction is one sender's emoji on a target message.
type Reaction struct {
	Sender string `json:"sender"`
	Emoji  string `json:"emoji"`
}

type metadataEnvelope struct {
	Media     json.RawMessage `json:"media,omitempty"`
	Reactions []Reaction      `json:"reactions,omitempty"`
}

// ParseReactions reads reaction entries from message metadata_json.
func ParseReactions(metadataJSON string) []Reaction {
	if strings.TrimSpace(metadataJSON) == "" || metadataJSON == "{}" {
		return nil
	}
	var env metadataEnvelope
	if err := json.Unmarshal([]byte(metadataJSON), &env); err != nil {
		return nil
	}
	return env.Reactions
}

// FormatReactions collapses reactions for the transcript (e.g. "😂 👍×2").
func FormatReactions(metadataJSON string) string {
	list := ParseReactions(metadataJSON)
	if len(list) == 0 {
		return ""
	}
	counts := map[string]int{}
	order := make([]string, 0, len(list))
	for _, r := range list {
		emoji := strings.TrimSpace(r.Emoji)
		if emoji == "" {
			continue
		}
		if _, ok := counts[emoji]; !ok {
			order = append(order, emoji)
		}
		counts[emoji]++
	}
	if len(order) == 0 {
		return ""
	}
	parts := make([]string, 0, len(order))
	for _, emoji := range order {
		n := counts[emoji]
		if n > 1 {
			parts = append(parts, fmt.Sprintf("%s×%d", emoji, n))
		} else {
			parts = append(parts, emoji)
		}
	}
	return strings.Join(parts, " ")
}

// MergeReactionIntoMetadata updates metadata_json: one emoji per sender.
// Empty emoji removes that sender's reaction.
func MergeReactionIntoMetadata(metadataJSON, sender, emoji string) (string, error) {
	env := metadataEnvelope{}
	if strings.TrimSpace(metadataJSON) != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &env); err != nil {
			// Preserve unknown blobs by wrapping only reactions when parse fails.
			env = metadataEnvelope{}
		}
	}
	emoji = strings.TrimSpace(emoji)
	out := make([]Reaction, 0, len(env.Reactions)+1)
	for _, r := range env.Reactions {
		if r.Sender == sender {
			continue
		}
		if strings.TrimSpace(r.Emoji) == "" {
			continue
		}
		out = append(out, r)
	}
	if emoji != "" {
		out = append(out, Reaction{Sender: sender, Emoji: emoji})
	}
	env.Reactions = out
	b, err := json.Marshal(env)
	if err != nil {
		return metadataJSON, err
	}
	return string(b), nil
}

// ApplyReaction attaches or removes a reaction on the target message.
// Returns the updated target message (or zero + false if the target is missing).
func (s *Store) ApplyReaction(ctx context.Context, chatID, targetID, sender, emoji string) (Message, bool, error) {
	if chatID == "" || targetID == "" {
		return Message{}, false, fmt.Errorf("empty reaction target")
	}
	msg, err := s.GetMessage(ctx, chatID, targetID)
	if err != nil {
		return Message{}, false, nil // target not in local DB yet — skip quietly
	}
	merged, err := MergeReactionIntoMetadata(msg.MetadataJSON, sender, emoji)
	if err != nil {
		return Message{}, false, err
	}
	msg.MetadataJSON = merged
	if err := s.UpsertMessage(ctx, msg); err != nil {
		return Message{}, false, err
	}
	return msg, true, nil
}

// DeleteMessage removes one local message row (and its FTS entry).
func (s *Store) DeleteMessage(ctx context.Context, chatID, id string) error {
	if chatID == "" || id == "" {
		return fmt.Errorf("empty message id")
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid IN (
SELECT rowid FROM messages WHERE chat_id = ? AND id = ?)`, chatID, id)
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE chat_id = ? AND id = ?`, chatID, id)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}
