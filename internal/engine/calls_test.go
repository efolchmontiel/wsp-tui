package engine

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestCallChatCandidatesPrefersGroup(t *testing.T) {
	meta := types.BasicCallMeta{
		From:     types.NewJID("111", types.DefaultUserServer),
		GroupJID: types.NewJID("999", types.GroupServer),
	}
	got := callChatCandidates(meta)
	if len(got) != 1 || got[0] != "999@g.us" {
		t.Fatalf("got %#v", got)
	}
}

func TestCallChatCandidatesIncludesAlt(t *testing.T) {
	meta := types.BasicCallMeta{
		From:           types.NewJID("123", types.HiddenUserServer),
		CallCreator:    types.NewJID("123", types.HiddenUserServer),
		CallCreatorAlt: types.NewJID("549111", types.DefaultUserServer),
	}
	got := callChatCandidates(meta)
	if len(got) < 2 {
		t.Fatalf("expected From + Alt, got %#v", got)
	}
	want := map[string]bool{
		"123@lid":                true,
		"549111@s.whatsapp.net": true,
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected candidate %q in %#v", id, got)
		}
	}
}

func TestCallMessageIDStable(t *testing.T) {
	if callMessageID("abc") != "call|abc" {
		t.Fatal(callMessageID("abc"))
	}
	if callMessageID("") == "call|" {
		t.Fatal("empty call id should use timestamp fallback")
	}
}

func TestMissedCallTextFromMedia(t *testing.T) {
	if missedCallText("video", "", "", nil) != "Llamada perdida · video" {
		t.Fatal("video")
	}
	if missedCallText("voz", "", "", nil) != "Llamada perdida · voz" {
		t.Fatal("voz")
	}
}

func TestCallMissTimeoutConstant(t *testing.T) {
	if callMissTimeout < 30*time.Second {
		t.Fatal("timeout too short")
	}
}
