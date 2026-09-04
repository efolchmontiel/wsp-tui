package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestDeleteChatRemovesRelatedRows(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	_ = s.UpsertChat(ctx, store.Chat{ID: "c1", Name: "A", LastMessageAt: 1})
	_ = s.UpsertMessage(ctx, store.Message{ID: "m1", ChatID: "c1", Text: "hola pizza", Timestamp: 1})
	_ = s.UpsertMedia(ctx, store.MediaRow{ID: "c1|m1", ChatID: "c1", MessageID: "m1"})
	_ = s.SetChatPronoun(ctx, "c1", store.PronounEl)

	if err := s.DeleteChat(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetChat(ctx, "c1"); err == nil {
		t.Fatal("chat should be gone")
	}
	msgs, err := s.ListMessages(ctx, "c1", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages left: %d", len(msgs))
	}
	hits, err := s.SearchMessages(ctx, "pizza", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("fts left: %+v", hits)
	}
}
