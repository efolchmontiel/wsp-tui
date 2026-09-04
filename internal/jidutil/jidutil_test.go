package jidutil

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestIsGroupAndStatus(t *testing.T) {
	g, _ := types.ParseJID("120363200382150334@g.us")
	if !IsGroup(g) {
		t.Fatal("expected group")
	}
	st, _ := types.ParseJID("status@broadcast")
	if !IsStatusBroadcast(st) || IsGroup(st) {
		t.Fatalf("status flags wrong")
	}
	u, _ := types.ParseJID("56951520785@s.whatsapp.net")
	if IsGroup(u) || IsStatusBroadcast(u) {
		t.Fatal("user jid misclassified")
	}
}
