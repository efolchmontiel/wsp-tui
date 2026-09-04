package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// handleMouse maps clicks/wheel to focus, chat selection, and transcript scroll.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x, y := msg.X, msg.Y
	sidebarW, _, bodyH := m.layoutSizes()

	headerH := 1
	bodyY0 := headerH
	bodyY1 := bodyY0 + bodyH

	if tea.MouseEvent(msg).IsWheel() {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if x < sidebarW {
				if m.chatCursor > 0 {
					m.chatCursor--
					m.ensureSidebarVisible()
				}
				return m, nil
			}
			m.msgVP.LineUp(3)
			return m, nil
		case tea.MouseButtonWheelDown:
			if x < sidebarW {
				if m.chatCursor < len(m.chats)-1 {
					m.chatCursor++
					m.ensureSidebarVisible()
				}
				return m, nil
			}
			m.msgVP.LineDown(3)
			return m, nil
		}
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if y < bodyY0 || y >= bodyY1 {
		return m, nil
	}
	if x < sidebarW {
		m.focus = focusSidebar
		m.applyFocus()
		// filter bar + title occupy the first two rows inside the sidebar
		row := y - bodyY0 - 2
		if row >= 0 {
			idx := m.sidebarOff + row
			if idx >= 0 && idx < len(m.chats) {
				m.chatCursor = idx
				return m.selectChatAtCursor()
			}
		}
		return m, nil
	}
	contentX0 := sidebarW + 1
	if x < contentX0 {
		return m, nil
	}
	localY := y - bodyY0
	inputTop := bodyH - 3
	if localY >= inputTop {
		m.focus = focusInput
		m.applyFocus()
		return m, textinput.Blink
	}
	if localY >= 1 {
		m.focus = focusMessages
		m.applyFocus()
	}
	return m, nil
}
