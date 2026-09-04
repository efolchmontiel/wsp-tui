package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDedupeOneToOneChatsByPhone(t *testing.T) {
	in := []Chat{
		{ID: "56977451117@lid", Name: "+56 9 7745 1117", LastMessageAt: 10, LastMessage: "viejo"},
		{ID: "56977451117@s.whatsapp.net", Name: "+56 9 7745 1117", LastMessageAt: 20, LastMessage: "nuevo"},
		{ID: "120363@g.us", Name: "Grupo", IsGroup: true, LastMessageAt: 5},
	}
	out := dedupeOneToOneChats(in)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(out), out)
	}
	var pn Chat
	for _, c := range out {
		if !c.IsGroup {
			pn = c
		}
	}
	if pn.ID != "56977451117@s.whatsapp.net" {
		t.Fatalf("kept %q", pn.ID)
	}
	if pn.LastMessage != "nuevo" {
		t.Fatalf("preview %q", pn.LastMessage)
	}
}

func TestListChatsFilteredDedupes(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	_ = s.UpsertChat(ctx, Chat{ID: "569111@lid", Name: "+56 9 1111 1111", LastMessageAt: 1, LastMessage: "a"})
	_ = s.UpsertChat(ctx, Chat{ID: "569111@s.whatsapp.net", Name: "+56 9 1111 1111", LastMessageAt: 2, LastMessage: "b"})
	got, err := s.ListChatsFiltered(ctx, FilterAll, 50)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range got {
		if onlyDigits(c.Name) == "5691111111" || onlyDigits(c.ID) == "569111" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected 1 phone chat, got %d (%+v)", n, got)
	}
}
