package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efolchmontiel/wsp-tui/internal/config"
)

// Retention modal: pick a preset (with *) or custom number + unit.
// Opened with R — never dumps status under the chat.

func (m *Model) openRetentionModal() {
	m.modal = modalRetention
	n, unit, never := m.cfg.LocalRetention.Parts()
	m.retCursor = 0
	m.retCustom = false
	m.retUnit = unit
	if m.retUnit == "" {
		m.retUnit = config.UnitMonth
	}
	m.retAmount.SetValue("")
	if !never && n > 0 {
		// Prefer matching a preset row.
		cur := config.ParseRetention(string(m.cfg.LocalRetention))
		for i, p := range config.RetentionPresets {
			if config.EqualRetention(p, cur) {
				m.retCursor = i
				return
			}
		}
		m.retCustom = true
		m.retCursor = len(config.RetentionPresets) // custom row
		m.retAmount.SetValue(strconv.Itoa(n))
		m.retAmount.Focus()
	}
}

func (m Model) viewRetentionModal() string {
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Retención local"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("Borra mensajes/media de ESTA PC. No toca el teléfono."))
	b.WriteString("\n\n")

	cur := config.ParseRetention(string(m.cfg.LocalRetention))
	for i, p := range config.RetentionPresets {
		mark := " "
		if config.EqualRetention(p, cur) {
			mark = "*"
		}
		line := fmt.Sprintf("%s %s", mark, p.Label())
		if i == m.retCursor && !m.retCustom {
			line = m.theme.sidebarSel.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Custom row
	custMark := " "
	_, _, never := cur.Parts()
	if !never {
		matched := false
		for _, p := range config.RetentionPresets {
			if config.EqualRetention(p, cur) {
				matched = true
				break
			}
		}
		if !matched {
			custMark = "*"
		}
	}
	custLine := fmt.Sprintf("%s Personalizado: %s %s", custMark, m.retAmount.View(), unitLabelES(m.retUnit))
	if m.retCursor == len(config.RetentionPresets) {
		custLine = m.theme.sidebarSel.Render("▸ " + custLine)
	} else {
		custLine = "  " + custLine
	}
	b.WriteString(custLine)
	b.WriteString("\n\n")
	if tip := strings.TrimSpace(m.infoMsg); tip != "" && time.Now().Before(m.infoUntil) {
		b.WriteString(m.theme.statusErr.Render(tip))
		b.WriteString("\n\n")
	}
	b.WriteString(m.theme.help.Render("↑↓ elegir · ←→ unidad (en personalizado) · Enter aplicar · Esc cerrar"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("Unidades: semana · mes · año  |  default del proyecto: 3 meses"))

	box := m.theme.box.Width(max(48, min(m.width-4, 64)))
	return box.Render(b.String())
}

func unitLabelES(u config.RetentionUnit) string {
	switch u {
	case config.UnitWeek:
		return "semana(s)"
	case config.UnitYear:
		return "año(s)"
	default:
		return "mes(es)"
	}
}

func cycleUnit(u config.RetentionUnit, dir int) config.RetentionUnit {
	order := []config.RetentionUnit{config.UnitWeek, config.UnitMonth, config.UnitYear}
	idx := 1
	for i, x := range order {
		if x == u {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(order)) % len(order)
	return order[idx]
}

func (m Model) updateRetentionModalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.modal = modalNone
		m.retAmount.Blur()
		return m, nil
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.retCursor > 0 {
			m.retCursor--
		}
		m.retCustom = m.retCursor == len(config.RetentionPresets)
		if m.retCustom {
			m.retAmount.Focus()
		} else {
			m.retAmount.Blur()
		}
		return m, nil
	case "down", "j":
		maxIdx := len(config.RetentionPresets) // custom row
		if m.retCursor < maxIdx {
			m.retCursor++
		}
		m.retCustom = m.retCursor == len(config.RetentionPresets)
		if m.retCustom {
			m.retAmount.Focus()
		} else {
			m.retAmount.Blur()
		}
		return m, nil
	case "left", "h":
		if m.retCustom {
			m.retUnit = cycleUnit(m.retUnit, -1)
		}
		return m, nil
	case "right", "l":
		if m.retCustom {
			m.retUnit = cycleUnit(m.retUnit, 1)
		}
		return m, nil
	case "enter":
		return m.applyRetentionSelection()
	case "tab":
		if m.retCustom {
			m.retUnit = cycleUnit(m.retUnit, 1)
			return m, nil
		}
	}
	if m.retCustom {
		var cmd tea.Cmd
		m.retAmount, cmd = m.retAmount.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) applyRetentionSelection() (tea.Model, tea.Cmd) {
	var next config.RetentionPeriod
	if m.retCursor < len(config.RetentionPresets) {
		next = config.ParseRetention(string(config.RetentionPresets[m.retCursor]))
	} else {
		raw := strings.TrimSpace(m.retAmount.Value())
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			m.setInfo("Ingresa un número ≥ 1 para personalizado")
			return m, nil
		}
		unit := m.retUnit
		if unit == "" {
			unit = config.UnitMonth
		}
		next = config.FormatRetention(n, unit)
	}
	m.cfg.LocalRetention = next
	if m.cfgPath != "" {
		_ = config.Save(m.cfgPath, m.cfg)
	}
	m.modal = modalNone
	m.retAmount.Blur()
	m.setInfo("Retención: " + next.Label())
	return m, nil
}

func newRetentionAmountInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "N"
	ti.CharLimit = 4
	ti.Width = 4
	ti.Prompt = ""
	return ti
}
