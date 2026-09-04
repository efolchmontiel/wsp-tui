package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efolchmontiel/wsp-tui/internal/media"
	"github.com/efolchmontiel/wsp-tui/internal/store"
)

// Message cursor ([ ]): selects ANY message for reactions / open / download.
// Previously only hopped media rows — that blocked reacting to text.

func (m *Model) selectLastMsgCursor() {
	if len(m.messages) == 0 {
		m.msgCursor = -1
		return
	}
	m.msgCursor = len(m.messages) - 1
}

func (m *Model) moveMsgCursor(delta int) {
	if len(m.messages) == 0 {
		m.msgCursor = -1
		return
	}
	if m.msgCursor < 0 || m.msgCursor >= len(m.messages) {
		m.msgCursor = len(m.messages) - 1
	} else {
		m.msgCursor = (m.msgCursor + delta + len(m.messages)*8) % len(m.messages)
	}
	msg := m.messages[m.msgCursor]
	label := msg.Text
	if label == "" {
		label = msg.Type
	}
	kind := "msg"
	if msg.MediaID != "" || isMediaType(msg.Type) || looksLikeMediaText(msg.Text) {
		kind = "media"
	}
	m.setInfo(fmt.Sprintf("%s %d/%d: %s", kind, m.msgCursor+1, len(m.messages), truncate(label, 40)))
	m.refreshViewport(false)
}

func (m Model) openSelectedMedia() (tea.Model, tea.Cmd) {
	if m.msgCursor < 0 || m.msgCursor >= len(m.messages) {
		m.selectLastMsgCursor()
	}
	if m.msgCursor < 0 || m.msgCursor >= len(m.messages) {
		m.setInfo("No hay mensajes en este chat")
		return m, nil
	}
	msg := m.messages[m.msgCursor]
	if msg.MediaID != "" {
		// Same clip already playing / opening → ignore until it finishes.
		if m.playingMediaID == msg.MediaID {
			m.setInfo("Ya está reproduciendo")
			return m, nil
		}
		m.playingMsgID = msg.ID
		if msg.Type == store.TypeAudio || msg.Type == store.TypeVideo || looksLikeMediaText(msg.Text) {
			m.playingMediaID = msg.MediaID // claim slot so spam-o is blocked
		}
		m.setInfo("Abriendo…")
		return m, openMediaCmd(m.eng, msg.MediaID)
	}
	if isMediaType(msg.Type) || looksLikeMediaText(msg.Text) {
		m.pendingOpenChat = msg.ChatID
		m.pendingOpenMsgID = msg.ID
		m.setInfo("Sin claves — pidiendo al teléfono…")
		return m, requestResendCmd(m.eng, msg)
	}
	m.setInfo("Ese mensaje no tiene adjunto — usa [ ] o la tecla o en un audio/imagen")
	return m, nil
}

func (m Model) setChatFilter(f store.ChatFilter) (tea.Model, tea.Cmd) {
	m.chatFilter = f
	m.chatCursor = 0
	m.sidebarOff = 0
	m.setInfo("Filtro: " + filterLabel(f))
	return m, loadChatsCmd(m.store, m.chatFilter)
}

func filterLabel(f store.ChatFilter) string {
	switch f {
	case store.FilterFavorites:
		return "Favoritos"
	case store.FilterGroups:
		return "Grupos"
	case store.FilterNovedades:
		return "Novedades"
	case store.FilterArchived:
		return "Archivados"
	default:
		return "Todos"
	}
}

func (m Model) toggleArchiveSelected() (tea.Model, tea.Cmd) {
	id := m.selectedSidebarChatID()
	if id == "" {
		m.setInfo("Elige un chat")
		return m, nil
	}
	cur := m.store.IsChatArchived(context.Background(), id)
	if err := m.store.SetChatArchived(context.Background(), id, !cur); err != nil {
		m.errMsg = err.Error()
		m.modal = modalError
		return m, nil
	}
	if !cur {
		m.setInfo("Archivado")
		if m.selectedID == id {
			m.selectedID = ""
			m.messages = nil
		}
	} else {
		m.setInfo("Desarchivado")
	}
	return m, loadChatsCmd(m.store, m.chatFilter)
}

func (m Model) toggleFavoriteSelected() (tea.Model, tea.Cmd) {
	id := m.selectedSidebarChatID()
	if id == "" {
		m.setInfo("Elige un chat")
		return m, nil
	}
	fav := true
	for _, c := range m.chats {
		if c.ID == id {
			fav = !c.IsPinned
			break
		}
	}
	if err := m.store.SetChatFavorite(context.Background(), id, fav); err != nil {
		m.errMsg = err.Error()
		m.modal = modalError
		return m, nil
	}
	if fav {
		m.setInfo("Marcado favorito")
	} else {
		m.setInfo("Sacado de favoritos")
	}
	return m, loadChatsCmd(m.store, m.chatFilter)
}

func (m Model) selectedSidebarChatID() string {
	if m.chatCursor >= 0 && m.chatCursor < len(m.chats) {
		return m.chats[m.chatCursor].ID
	}
	return m.selectedID
}

func filterBar(active store.ChatFilter, width int, th theme) string {
	tabs := []struct {
		f store.ChatFilter
		s string
	}{
		{store.FilterAll, "Todos"},
		{store.FilterFavorites, "Fav"},
		{store.FilterGroups, "Grupos"},
		{store.FilterNovedades, "Noved"},
		{store.FilterArchived, "Arch"},
	}
	parts := make([]string, 0, 5)
	for _, t := range tabs {
		if t.f == active {
			parts = append(parts, th.accent.Bold(true).Render("["+t.s+"]"))
		} else {
			parts = append(parts, th.muted.Render(t.s))
		}
	}
	line := strings.Join(parts, " ")
	return truncate(line+th.muted.Render(" ·1-5"), max(10, width-2))
}

func waveUnder(playing bool, phase float64, width int, th theme) string {
	if !playing {
		return ""
	}
	w := media.Waveframe(phase, min(36, max(12, width-8)))
	return th.accent.Render("  " + w)
}
