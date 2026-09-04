package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestPurgeOlderThanRemovesOldKeepRecent(t *testing.T) {
	dir := t.TempDir()
	mediaDir := filepath.Join(dir, "media")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	oldTS := now.Add(-100 * 24 * time.Hour).Unix() // > 90 days
	newTS := now.Add(-10 * 24 * time.Hour).Unix()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(s.UpsertChat(ctx, store.Chat{ID: "c1", Name: "Ana", IsPinned: true, LastMessageAt: oldTS, LastMessage: "viejo"}))
	must(s.UpsertMessage(ctx, store.Message{
		ID: "m-old", ChatID: "c1", Text: "viejo", Timestamp: oldTS, Type: store.TypeText, Status: store.StatusReceived,
	}))
	must(s.UpsertMessage(ctx, store.Message{
		ID: "m-new", ChatID: "c1", Text: "nuevo", Timestamp: newTS, Type: store.TypeText, Status: store.StatusReceived,
	}))

	oldFile := filepath.Join(mediaDir, "old.ogg")
	newFile := filepath.Join(mediaDir, "new.ogg")
	must(os.WriteFile(oldFile, []byte("old"), 0o600))
	must(os.WriteFile(newFile, []byte("new"), 0o600))
	must(s.UpsertMedia(ctx, store.MediaRow{
		ID: "c1|m-old", ChatID: "c1", MessageID: "m-old", LocalPath: oldFile,
		DownloadState: store.MediaReady, CreatedAt: oldTS,
	}))
	must(s.UpsertMedia(ctx, store.MediaRow{
		ID: "c1|m-new", ChatID: "c1", MessageID: "m-new", LocalPath: newFile,
		DownloadState: store.MediaReady, CreatedAt: newTS,
	}))
	must(s.SetChatArchived(ctx, "c1", true))

	stats, err := s.PurgeOlderThan(ctx, now, mediaDir, store.DefaultRetention)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Skipped {
		t.Fatal("should not skip")
	}
	if stats.Messages != 1 {
		t.Fatalf("messages purged=%d want 1", stats.Messages)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("old media file should be gone: %v", err)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("new media file should remain: %v", err)
	}

	msgs, err := s.ListMessages(ctx, "c1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].ID != "m-new" {
		t.Fatalf("remaining msgs=%+v", msgs)
	}

	chat, err := s.GetChat(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if chat.LastMessage != "nuevo" || chat.LastMessageAt != newTS {
		t.Fatalf("preview not refreshed: %+v", chat)
	}
	if !chat.IsArchived || !chat.IsPinned {
		t.Fatalf("chat flags should remain: archived=%v pinned=%v", chat.IsArchived, chat.IsPinned)
	}

	// Throttle: second MaybePurge should skip.
	st2, err := s.MaybePurgeOlderThan(ctx, now.Add(time.Hour), mediaDir, store.DefaultRetention, true)
	if err != nil {
		t.Fatal(err)
	}
	if !st2.Skipped {
		t.Fatal("expected skip within 24h")
	}

	stNever, err := s.MaybePurgeOlderThan(ctx, now.Add(48*time.Hour), mediaDir, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !stNever.Skipped {
		t.Fatal("never retention must skip")
	}
}
