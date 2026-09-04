package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/engine"
	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func (m Model) openAddContact() (tea.Model, tea.Cmd) {
	if m.state != app.StateConnected {
		m.errMsg = "Conectate antes de agregar contactos"
		m.modal = modalError
		return m, nil
	}
	m.addingContact = true
	m.addField = 0
	m.addName.SetValue("")
	m.addPhone.SetValue("")
	m.addName.Focus()
	m.addPhone.Blur()
	m.input.Blur()
	return m, textinput.Blink
}

func (m Model) updateAddContactKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addingContact = false
		m.addName.Blur()
		m.addPhone.Blur()
		m.setInfo("Alta cancelada")
		return m, nil
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "tab", "down":
		m.addField = 1
		m.addName.Blur()
		m.addPhone.Focus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.addField = 0
		m.addPhone.Blur()
		m.addName.Focus()
		return m, textinput.Blink
	case "enter":
		if m.addField == 0 {
			m.addField = 1
			m.addName.Blur()
			m.addPhone.Focus()
			return m, textinput.Blink
		}
		name := strings.TrimSpace(m.addName.Value())
		phone := strings.TrimSpace(m.addPhone.Value())
		if phone == "" {
			m.errMsg = "Falta el teléfono"
			m.modal = modalError
			return m, nil
		}
		m.addingContact = false
		m.addName.Blur()
		m.addPhone.Blur()
		m.setInfo("Verificando número…")
		return m, addContactCmd(m.eng, phone, name)
	}
	var cmd tea.Cmd
	if m.addField == 0 {
		m.addName, cmd = m.addName.Update(msg)
	} else {
		m.addPhone, cmd = m.addPhone.Update(msg)
	}
	return m, cmd
}

func (m Model) viewAddContact() string {
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Nuevo contacto"))
	b.WriteString(m.theme.muted.Render("  Tab campos · Enter confirmar · Esc cancelar"))
	b.WriteString("\n\n")
	b.WriteString(m.addName.View())
	b.WriteString("\n")
	b.WriteString(m.addPhone.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.muted.Render("Se verifica en WhatsApp y se abre el chat local."))
	return b.String()
}

func (m Model) viewConfirmDelete() string {
	name := m.pendingDelete
	for _, c := range m.chats {
		if c.ID == m.pendingDelete {
			if c.Name != "" {
				name = c.Name
			}
			break
		}
	}
	body := fmt.Sprintf("¿Eliminar chat con «%s»?\n\nSolo se borra en este dispositivo.\nWhatsApp en el teléfono no se toca.\n\ny / Enter confirmar · n / Esc cancelar", name)
	box := m.theme.box.Width(max(40, min(m.width-4, 64)))
	return m.theme.statusErr.Render("Eliminar chat") + "\n\n" + box.Render(body)
}

func (m Model) askDeleteSelected() (tea.Model, tea.Cmd) {
	id := m.selectedID
	if id == "" && m.chatCursor >= 0 && m.chatCursor < len(m.chats) {
		id = m.chats[m.chatCursor].ID
	}
	if id == "" {
		m.setInfo("No hay chat seleccionado")
		return m, nil
	}
	if id == "status@broadcast" || strings.HasPrefix(id, "status@") {
		m.setInfo("No se puede eliminar Estados")
		return m, nil
	}
	m.pendingDelete = id
	m.modal = modalConfirmDelete
	return m, nil
}

func (m Model) updateConfirmDeleteKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		id := m.pendingDelete
		m.modal = modalNone
		m.pendingDelete = ""
		return m, deleteChatCmd(m.eng, id)
	case "n", "esc":
		m.modal = modalNone
		m.pendingDelete = ""
		m.setInfo("Eliminación cancelada")
		return m, nil
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func addContactCmd(eng *engine.Engine, phone, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		chat, err := eng.AddContactByPhone(ctx, phone, name)
		if err != nil {
			return addContactMsg{err: err}
		}
		return addContactMsg{chatID: chat.ID, name: chat.Name}
	}
}

func startChatCmd(eng *engine.Engine, jid, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		chat, err := eng.StartChat(ctx, jid, name)
		if err != nil {
			return startChatMsg{err: err}
		}
		return startChatMsg{chatID: chat.ID, name: chat.Name}
	}
}

func deleteChatCmd(eng *engine.Engine, chatID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := eng.DeleteLocalChat(ctx, chatID)
		return deleteChatMsg{chatID: chatID, err: err}
	}
}

func searchCmd(st *store.Store, eng *engine.Engine, gen int, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		hits, err := st.Search(ctx, query, 40)
		if err != nil {
			return searchLoadedMsg{gen: gen, err: err}
		}
		contacts, cerr := eng.SearchContacts(ctx, query, 25)
		if cerr == nil && len(contacts) > 0 {
			seen := map[string]struct{}{}
			for _, h := range hits {
				seen[h.ChatID] = struct{}{}
			}
			contactHits := make([]store.SearchHit, 0, len(contacts))
			for _, c := range contacts {
				if _, ok := seen[c.JID]; ok {
					continue
				}
				contactHits = append(contactHits, store.SearchHit{
					Kind:     "contact",
					ChatID:   c.JID,
					ChatName: c.Name,
					Snippet:  c.Phone,
				})
				seen[c.JID] = struct{}{}
			}
			hits = append(contactHits, hits...)
			if len(hits) > 50 {
				hits = hits[:50]
			}
		}
		return searchLoadedMsg{gen: gen, hits: hits}
	}
}

func (m Model) openChatID(chatID, name string) (tea.Model, tea.Cmd) {
	m.selectedID = chatID
	m.messages = nil
	m.loadingMsgs = true
	m.msgVP.SetYOffset(0)
	m.msgVP.SetContent(m.theme.muted.Render("Cargando mensajes…"))
	m.focus = focusInput
	m.applyFocus()
	_ = m.store.ClearUnread(context.Background(), chatID)
	if name != "" {
		m.setInfo("Chat: " + name)
	}
	return m, tea.Batch(loadMessagesCmd(m.store, chatID), loadChatsCmd(m.store, m.chatFilter), textinput.Blink)
}
