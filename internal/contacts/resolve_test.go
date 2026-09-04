package contacts

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestPreferAddressBookOverPush(t *testing.T) {
	a := Resolved{Name: "Christian", FromAddressBook: false}
	b := Resolved{Name: "Papito", FromAddressBook: true}
	got := prefer(a, b)
	if got.Name != "Papito" || !got.FromAddressBook {
		t.Fatalf("%+v", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "papito"); got != "papito" {
		t.Fatalf("%q", got)
	}
	_ = types.EmptyJID
	_ = context.Background()
}
