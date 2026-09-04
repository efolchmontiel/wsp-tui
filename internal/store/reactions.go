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

// LinkPreview is WhatsApp's unfurled link card (YouTube, etc.).
type LinkPreview struct {
	Title string `json:"title,omitempty"`
	Desc  string `json:"desc,omitempty"`
	URL   string `json:"url,omitempty"`
	Thumb string `json:"thumb,omitempty"` // local JPEG path
}

type metadataEnvelope struct {
	Media        json.RawMessage `json:"media,omitempty"`
	Reactions    []Reaction      `json:"reactions,omitempty"`
	Link         *LinkPreview    `json:"link,omitempty"`
	PreviewThumb string          `json:"preview_thumb,omitempty"` // local JPEG for image/video/link
}

// ParseLinkPreview reads link card metadata.
func ParseLinkPreview(metadataJSON string) (LinkPreview, bool) {
	if strings.TrimSpace(metadataJSON) == "" || metadataJSON == "{}" {
		return LinkPreview{}, false
	}
	var env metadataEnvelope
	if err := json.Unmarshal([]byte(metadataJSON), &env); err != nil || env.Link == nil {
		return LinkPreview{}, false
	}
	if env.Link.Title == "" && env.Link.URL == "" && env.Link.Thumb == "" && env.Link.Desc == "" {
		return LinkPreview{}, false
	}
	return *env.Link, true
}

// ParsePreviewThumb returns a local thumbnail path for inline rendering.
func ParsePreviewThumb(metadataJSON string) string {
	if strings.TrimSpace(metadataJSON) == "" || metadataJSON == "{}" {
		return ""
	}
	var env metadataEnvelope
	if err := json.Unmarshal([]byte(metadataJSON), &env); err != nil {
		return ""
	}
	if env.Link != nil && env.Link.Thumb != "" {
		return env.Link.Thumb
	}
	return env.PreviewThumb
}

// MergeLinkIntoMetadata stores/replaces the link preview card.
func MergeLinkIntoMetadata(metadataJSON string, link LinkPreview) (string, error) {
	env := metadataEnvelope{}
	if strings.TrimSpace(metadataJSON) != "" && metadataJSON != "{}" {
		_ = json.Unmarshal([]byte(metadataJSON), &env)
	}
	env.Link = &link
	if link.Thumb != "" {
		env.PreviewThumb = link.Thumb
	}
	b, err := json.Marshal(env)
	if err != nil {
		return metadataJSON, err
	}
	return string(b), nil
}

// MergePreviewThumbIntoMetadata sets preview_thumb without clearing other fields.
func MergePreviewThumbIntoMetadata(metadataJSON, thumbPath string) (string, error) {
	env := metadataEnvelope{}
	if strings.TrimSpace(metadataJSON) != "" && metadataJSON != "{}" {
		_ = json.Unmarshal([]byte(metadataJSON), &env)
	}
	env.PreviewThumb = thumbPath
	b, err := json.Marshal(env)
	if err != nil {
		return metadataJSON, err
	}
	return string(b), nil
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

// MergeMessageMetadata combines existing + incoming metadata without dropping
// reactions / link / thumbs when the incoming blob omits them (common on re-sync).
func MergeMessageMetadata(existing, incoming string) string {
	ex := parseEnvelope(existing)
	in := parseEnvelope(incoming)
	out := ex
	if len(in.Media) > 0 && string(in.Media) != "null" {
		out.Media = in.Media
	}
	if in.Link != nil {
		out.Link = in.Link
	}
	if in.PreviewThumb != "" {
		out.PreviewThumb = in.PreviewThumb
	}
	if len(in.Reactions) > 0 {
		// Incoming reactions replace same senders; keep others from existing.
		bySender := map[string]Reaction{}
		order := make([]string, 0, len(ex.Reactions)+len(in.Reactions))
		for _, r := range ex.Reactions {
			if r.Sender == "" || strings.TrimSpace(r.Emoji) == "" {
				continue
			}
			bySender[r.Sender] = r
			order = append(order, r.Sender)
		}
		for _, r := range in.Reactions {
			if r.Sender == "" {
				continue
			}
			if strings.TrimSpace(r.Emoji) == "" {
				delete(bySender, r.Sender)
				continue
			}
			if _, ok := bySender[r.Sender]; !ok {
				order = append(order, r.Sender)
			}
			bySender[r.Sender] = r
		}
		merged := make([]Reaction, 0, len(bySender))
		seen := map[string]bool{}
		for _, s := range order {
			if r, ok := bySender[s]; ok && !seen[s] {
				merged = append(merged, r)
				seen[s] = true
			}
		}
		out.Reactions = merged
	}
	b, err := json.Marshal(out)
	if err != nil {
		if strings.TrimSpace(incoming) != "" {
			return incoming
		}
		return existing
	}
	return string(b)
}

func parseEnvelope(metadataJSON string) metadataEnvelope {
	env := metadataEnvelope{}
	if strings.TrimSpace(metadataJSON) == "" || metadataJSON == "{}" {
		return env
	}
	_ = json.Unmarshal([]byte(metadataJSON), &env)
	return env
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
	// Bypass UpsertMessage merge path: write reactions directly so empty emoji removals stick.
	if err := s.writeMessage(ctx, msg); err != nil {
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
