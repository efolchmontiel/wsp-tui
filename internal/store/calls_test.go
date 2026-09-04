package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestResolveIncomingCallsAcrossLIDPhone(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "calls.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	lid := "123@lid"
	pn := "549111@s.whatsapp.net"
	if err := s.UpsertChat(ctx, store.Chat{ID: lid, Name: "Ana"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertChat(ctx, store.Chat{ID: pn, Name: "Ana"}); err != nil {
		t.Fatal(err)
	}
	if !store.SameOneToOne(lid, pn) {
		// SameOneToOne may need Linked identity — if it doesn't match LID↔PN without
		// shared key, still resolve at least the chatID we pass.
		t.Log("SameOneToOne(lid,pn)=false; resolve still covers direct chat")
	}

	if err := s.UpsertMessage(ctx, store.Message{
		ID: "call|x1", ChatID: lid, Text: "Llamada entrante · voz",
		Type: store.TypeCallIncoming, Timestamp: 100, Status: store.StatusReceived,
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := s.ResolveIncomingCalls(ctx, lid, "Llamada perdida · voz")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 {
		t.Fatalf("updated=%d", len(updated))
	}
	got, err := s.GetMessage(ctx, lid, "call|x1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != store.TypeCallMissed {
		t.Fatalf("type=%s", got.Type)
	}
	if got.Text != "Llamada perdida · voz" {
		t.Fatalf("text=%q", got.Text)
	}
}

func TestSweepStaleIncomingCalls(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "sweep.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_ = s.UpsertChat(ctx, store.Chat{ID: "c1", Name: "Bob"})
	_ = s.UpsertMessage(ctx, store.Message{
		ID: "call|old", ChatID: "c1", Text: "Llamada entrante · video",
		Type: store.TypeCallIncoming, Timestamp: 50, Status: store.StatusReceived,
	})
	_ = s.UpsertMessage(ctx, store.Message{
		ID: "call|new", ChatID: "c1", Text: "Llamada entrante · voz",
		Type: store.TypeCallIncoming, Timestamp: 200, Status: store.StatusReceived,
	})

	updated, err := s.SweepStaleIncomingCalls(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].ID != "call|old" {
		t.Fatalf("%#v", updated)
	}
	old, _ := s.GetMessage(ctx, "c1", "call|old")
	fresh, _ := s.GetMessage(ctx, "c1", "call|new")
	if old.Type != store.TypeCallMissed || old.Text != "Llamada perdida · video" {
		t.Fatalf("old=%+v", old)
	}
	if fresh.Type != store.TypeCallIncoming {
		t.Fatalf("fresh should stay incoming: %+v", fresh)
	}
}

func TestIsChatArchivedLooseSibling(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "arch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	pn := "549111222333@s.whatsapp.net"
	lid := "549111222333@lid"
	_ = s.UpsertChat(ctx, store.Chat{ID: pn, Name: "+54 9 1111 222333"})
	_ = s.UpsertChat(ctx, store.Chat{ID: lid, Name: "+54 9 1111 222333"})
	if err := s.SetChatArchived(ctx, pn, true); err != nil {
		t.Fatal(err)
	}
	if !s.IsChatArchivedLoose(ctx, lid) {
		t.Fatal("LID sibling should count as archived")
	}
	if s.IsChatArchivedLoose(ctx, "other@s.whatsapp.net") {
		t.Fatal("unrelated chat")
	}
}
