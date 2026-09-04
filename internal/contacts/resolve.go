package contacts

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// Resolved is a display name with provenance.
type Resolved struct {
	Name           string
	FromAddressBook bool // true if FullName/FirstName from phone contacts
}

// ResolveDisplayName prefers the address-book name (e.g. "Papito") over push name.
// Groups and broadcasts are not resolved via the contact book.
func ResolveDisplayName(ctx context.Context, client *whatsmeow.Client, jid types.JID, fallback string) Resolved {
	fallback = strings.TrimSpace(fallback)
	if jid.Server == types.GroupServer {
		if fallback != "" {
			return Resolved{Name: fallback}
		}
		return Resolved{Name: "Grupo"}
	}
	if jid.Server == "broadcast" {
		if jid.User == "status" {
			return Resolved{Name: "Estados"}
		}
		return Resolved{Name: firstNonEmpty(fallback, "Broadcast")}
	}
	if client == nil || client.Store == nil {
		return Resolved{Name: firstNonEmpty(fallback, jid.User)}
	}

	best := lookup(ctx, client, jid)
	if alt, err := client.Store.GetAltJID(ctx, jid); err == nil && !alt.IsEmpty() {
		altInfo := lookup(ctx, client, alt)
		best = prefer(best, altInfo)
	}

	if best.FromAddressBook && best.Name != "" {
		return best
	}
	if best.Name != "" {
		return best
	}
	if fallback != "" {
		return Resolved{Name: fallback}
	}
	return Resolved{Name: jid.User}
}

func lookup(ctx context.Context, client *whatsmeow.Client, jid types.JID) Resolved {
	if jid.IsEmpty() {
		return Resolved{}
	}
	info, err := client.Store.Contacts.GetContact(ctx, jid.ToNonAD())
	if err != nil {
		return Resolved{}
	}
	if name := strings.TrimSpace(info.FullName); name != "" {
		return Resolved{Name: name, FromAddressBook: true}
	}
	if name := strings.TrimSpace(info.FirstName); name != "" {
		return Resolved{Name: name, FromAddressBook: true}
	}
	if name := strings.TrimSpace(info.BusinessName); name != "" {
		return Resolved{Name: name, FromAddressBook: true}
	}
	if name := strings.TrimSpace(info.PushName); name != "" {
		return Resolved{Name: name, FromAddressBook: false}
	}
	return Resolved{}
}

func prefer(a, b Resolved) Resolved {
	switch {
	case a.FromAddressBook && a.Name != "":
		return a
	case b.FromAddressBook && b.Name != "":
		return b
	case a.Name != "":
		return a
	default:
		return b
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
