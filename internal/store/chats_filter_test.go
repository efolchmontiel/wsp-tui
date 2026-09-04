package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestChatFiltersArchiveAndFavorite(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "filters.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	mustUpsert := func(c store.Chat) {
		t.Helper()
		if err := s.UpsertChat(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	mustUpsert(store.Chat{ID: "a@s.whatsapp.net", Name: "Ana", LastMessageAt: 300, LastMessage: "hola"})
	mustUpsert(store.Chat{ID: "g@g.us", Name: "Grupo", IsGroup: true, LastMessageAt: 200, LastMessage: "ok"})
	mustUpsert(store.Chat{ID: "b@s.whatsapp.net", Name: "Bet", IsPinned: true, LastMessageAt: 100, LastMessage: "x"})
	mustUpsert(store.Chat{ID: "c@g.us", Name: "Comunidad", IsGroup: true, IsCommunity: true, LastMessageAt: 250, LastMessage: "novedad"})

	all, err := s.ListChatsFiltered(ctx, store.FilterAll, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("FilterAll want 3 (no community) got %d", len(all))
	}

	fav, err := s.ListChatsFiltered(ctx, store.FilterFavorites, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(fav) != 1 || fav[0].ID != "b@s.whatsapp.net" {
		t.Fatalf("FilterFavorites %+v", fav)
	}

	groups, err := s.ListChatsFiltered(ctx, store.FilterGroups, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || !groups[0].IsGroup || groups[0].IsCommunity {
		t.Fatalf("FilterGroups %+v", groups)
	}

	noved, err := s.ListChatsFiltered(ctx, store.FilterNovedades, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(noved) != 1 || noved[0].ID != "c@g.us" || !noved[0].IsCommunity {
		t.Fatalf("FilterNovedades %+v", noved)
	}

	if err := s.SetChatArchived(ctx, "a@s.whatsapp.net", true); err != nil {
		t.Fatal(err)
	}
	if !s.IsChatArchived(ctx, "a@s.whatsapp.net") {
		t.Fatal("expected archived")
	}

	all, err = s.ListChatsFiltered(ctx, store.FilterAll, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("FilterAll after archive want 2 got %d", len(all))
	}

	arch, err := s.ListChatsFiltered(ctx, store.FilterArchived, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(arch) != 1 || arch[0].ID != "a@s.whatsapp.net" || !arch[0].IsArchived {
		t.Fatalf("FilterArchived %+v", arch)
	}

	if err := s.SetChatFavorite(ctx, "a@s.whatsapp.net", true); err != nil {
		t.Fatal(err)
	}
	// Still archived → not in Favorites (favorites exclude archived).
	fav, err = s.ListChatsFiltered(ctx, store.FilterFavorites, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(fav) != 1 || fav[0].ID != "b@s.whatsapp.net" {
		t.Fatalf("favorite while archived should stay out: %+v", fav)
	}

	if err := s.SetChatArchived(ctx, "a@s.whatsapp.net", false); err != nil {
		t.Fatal(err)
	}
	fav, err = s.ListChatsFiltered(ctx, store.FilterFavorites, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(fav) != 2 {
		t.Fatalf("after unarchive want 2 favorites got %d %+v", len(fav), fav)
	}
}
