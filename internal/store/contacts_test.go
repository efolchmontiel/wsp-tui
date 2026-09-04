package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestContactDisplayName(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "whatstui.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	id := "54911@s.whatsapp.net"
	if got := s.ContactDisplayName(ctx, id); got != "" {
		t.Fatalf("empty store want \"\" got %q", got)
	}
	if err := s.UpsertLocalContact(ctx, id, "Papito", "Christian"); err != nil {
		t.Fatal(err)
	}
	if got := s.ContactDisplayName(ctx, id); got != "Papito" {
		t.Fatalf("want Papito got %q", got)
	}
	if err := s.UpsertLocalContact(ctx, "onlypush@s.whatsapp.net", "", "PushOnly"); err != nil {
		t.Fatal(err)
	}
	if got := s.ContactDisplayName(ctx, "onlypush@s.whatsapp.net"); got != "PushOnly" {
		t.Fatalf("want PushOnly got %q", got)
	}
}
