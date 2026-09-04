package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestChatAndMessageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "whatstui.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if err := s.UpsertChat(ctx, store.Chat{
		ID: "chat1", Name: "Juan", LastMessageAt: 100, LastMessage: "hola", UnreadCount: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(ctx, store.Message{
		ID: "m1", ChatID: "chat1", Text: "hola", Timestamp: 100, Type: store.TypeText, Status: store.StatusReceived,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(ctx, store.Message{
		ID: "m2", ChatID: "chat1", Text: "chau", Timestamp: 200, Type: store.TypeText, Status: store.StatusReceived, IsFromMe: true,
	}); err != nil {
		t.Fatal(err)
	}

	chats, err := s.ListChats(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 1 || chats[0].Name != "Juan" {
		t.Fatalf("chats=%+v", chats)
	}

	msgs, err := s.ListMessages(ctx, "chat1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 msgs got %d", len(msgs))
	}
	if msgs[0].ID != "m1" || msgs[1].ID != "m2" {
		t.Fatalf("order %+v", msgs)
	}

	older, err := s.ListMessages(ctx, "chat1", 50, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 1 || older[0].ID != "m1" {
		t.Fatalf("pagination %+v", older)
	}

	if err := s.UpdateMessageStatus(ctx, "chat1", "m2", store.StatusRead); err != nil {
		t.Fatal(err)
	}
	msgs, _ = s.ListMessages(ctx, "chat1", 50, 0)
	if msgs[1].Status != store.StatusRead {
		t.Fatalf("status %s", msgs[1].Status)
	}
}

func TestTouchChatPreviewIncrementsUnread(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	_ = s.EnsureChatExists(ctx, "c1", "A", false)
	_ = s.TouchChatPreview(ctx, "c1", "x", 10, 1)
	_ = s.TouchChatPreview(ctx, "c1", "y", 20, 1)
	c, err := s.GetChat(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if c.UnreadCount != 2 || c.LastMessage != "y" {
		t.Fatalf("%+v", c)
	}
	_ = s.ClearUnread(ctx, "c1")
	c, _ = s.GetChat(ctx, "c1")
	if c.UnreadCount != 0 {
		t.Fatalf("unread %d", c.UnreadCount)
	}
}
