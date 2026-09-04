package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/jidutil"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow/types"
)

// ContactHit is an address-book entry suitable for starting a chat.
type ContactHit struct {
	JID   string
	Name  string
	Phone string
}

// SearchContacts filters the WhatsApp address book by name/phone/JID user part.
func (e *Engine) SearchContacts(ctx context.Context, query string, limit int) ([]ContactHit, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 30
	}

	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || client.Store == nil || client.Store.Contacts == nil {
		return nil, fmt.Errorf("client not ready")
	}

	all, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ContactHit, 0, limit)
	for jid, info := range all {
		if jid.Server == types.GroupServer || jid.Server == "broadcast" {
			continue
		}
		name := strings.TrimSpace(info.FullName)
		if name == "" {
			name = strings.TrimSpace(info.FirstName)
		}
		if name == "" {
			name = strings.TrimSpace(info.BusinessName)
		}
		if name == "" {
			name = strings.TrimSpace(info.PushName)
		}
		phone := jid.User
		hay := strings.ToLower(name + " " + phone + " " + jid.String())
		if !strings.Contains(hay, query) {
			continue
		}
		if name == "" {
			name = phone
		}
		out = append(out, ContactHit{JID: jid.ToNonAD().String(), Name: name, Phone: phone})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// StartChat ensures a local chat exists for the contact and returns it.
func (e *Engine) StartChat(ctx context.Context, jidStr, name string) (store.Chat, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return store.Chat{}, fmt.Errorf("jid: %w", err)
	}
	jid = jid.ToNonAD()
	name = strings.TrimSpace(name)
	if name == "" {
		name = jid.User
	}
	if e.store == nil {
		return store.Chat{}, fmt.Errorf("store not ready")
	}
	id := jid.String()
	if err := e.store.EnsureChatExists(ctx, id, name, jidutil.IsGroup(jid)); err != nil {
		return store.Chat{}, err
	}
	_ = e.store.SetChatName(ctx, id, name, jidutil.IsGroup(jid))
	_ = e.store.UpsertLocalContact(ctx, id, name, "")
	e.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	return e.store.GetChat(ctx, id)
}

// AddContactByPhone verifies the number on WhatsApp, saves the name, and opens a chat.
func (e *Engine) AddContactByPhone(ctx context.Context, phone, name string) (store.Chat, error) {
	phone = normalizePhone(phone)
	name = strings.TrimSpace(name)
	if phone == "" {
		return store.Chat{}, fmt.Errorf("número requerido")
	}

	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return store.Chat{}, fmt.Errorf("not connected")
	}

	query := phone
	if !strings.HasPrefix(query, "+") {
		query = "+" + query
	}
	res, err := client.IsOnWhatsApp(ctx, []string{query})
	if err != nil {
		return store.Chat{}, fmt.Errorf("verificar número: %w", err)
	}
	if len(res) == 0 || !res[0].IsIn {
		return store.Chat{}, fmt.Errorf("ese número no está en WhatsApp")
	}
	info := res[0]
	jid := info.JID.ToNonAD()
	if jid.IsEmpty() {
		jid = info.PhoneNumber.ToNonAD()
	}
	if jid.IsEmpty() {
		return store.Chat{}, fmt.Errorf("WhatsApp no devolvió un JID válido")
	}
	// Prefer phone-number JID for local chat identity when available (stable vs LID).
	chatJID := jid
	if !info.PhoneNumber.IsEmpty() {
		chatJID = info.PhoneNumber.ToNonAD()
	}
	if name == "" {
		name = chatJID.User
	}
	if client.Store != nil && client.Store.Contacts != nil {
		_ = client.Store.Contacts.PutContactName(ctx, chatJID, name, firstToken(name))
		if !jid.IsEmpty() && jid.String() != chatJID.String() {
			_ = client.Store.Contacts.PutContactName(ctx, jid, name, firstToken(name))
		}
	}
	return e.StartChat(ctx, chatJID.String(), name)
}

// DeleteLocalChat removes the chat from the local DB only.
func (e *Engine) DeleteLocalChat(ctx context.Context, chatID string) error {
	if e.store == nil {
		return fmt.Errorf("store not ready")
	}
	if err := e.store.DeleteChat(ctx, chatID); err != nil {
		return err
	}
	e.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	e.bus.Publish(app.Event{Kind: app.EventInfo, Message: "Chat eliminado (solo local)"})
	return nil
}

func normalizePhone(p string) string {
	p = strings.TrimSpace(p)
	var b strings.Builder
	for i, r := range p {
		if r == '+' && b.Len() == 0 && i == 0 {
			b.WriteRune('+')
			continue
		}
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstToken(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexFunc(name, unicode.IsSpace); i > 0 {
		return name[:i]
	}
	return name
}
