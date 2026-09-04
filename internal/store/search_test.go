package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestUpsertMessageUpdatesFTSWithoutConstraint(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "fts-upd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.UpsertChat(ctx, store.Chat{ID: "c1", Name: "Ana"}))
	must(s.UpsertMessage(ctx, store.Message{
		ID: "call|abc", ChatID: "c1", Text: "Llamada entrante · voz",
		Type: store.TypeCallIncoming, Timestamp: 100, Status: store.StatusReceived,
	}))
	// Second upsert (terminate → missed) previously failed FTS with constraint 1555.
	must(s.UpsertMessage(ctx, store.Message{
		ID: "call|abc", ChatID: "c1", Text: "Llamada perdida · voz",
		Type: store.TypeCallMissed, Timestamp: 101, Status: store.StatusReceived,
	}))
	got, err := s.GetMessage(ctx, "c1", "call|abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != store.TypeCallMissed || got.Text != "Llamada perdida · voz" {
		t.Fatalf("got %+v", got)
	}
}


func TestSearchFTSAndChats(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if err := s.UpsertChat(ctx, store.Chat{ID: "c1", Name: "Papito", LastMessageAt: 10, LastMessage: "hola"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertChat(ctx, store.Chat{ID: "c2", Name: "siameses", LastMessageAt: 20, LastMessage: "miau"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(ctx, store.Message{
		ID: "m1", ChatID: "c1", Text: "vamos a comer pizza juntos", Timestamp: 11, Type: store.TypeText,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(ctx, store.Message{
		ID: "m2", ChatID: "c2", Text: "los gatos duermen", Timestamp: 21, Type: store.TypeText,
	}); err != nil {
		t.Fatal(err)
	}

	var ver string
	if err := s.DB().QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	if ver != "2" {
		t.Fatalf("schema_version=%q", ver)
	}

	hits, err := s.Search(ctx, "pizza", 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.Kind == "message" && h.MessageID == "m1" {
			found = true
			if h.ChatName != "Papito" {
				t.Fatalf("chat name %q", h.ChatName)
			}
		}
	}
	if !found {
		t.Fatalf("expected pizza hit, got %+v", hits)
	}

	chatHits, err := s.SearchChats(ctx, "siame", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chatHits) != 1 || chatHits[0].ChatID != "c2" {
		t.Fatalf("chat hits %+v", chatHits)
	}
}
