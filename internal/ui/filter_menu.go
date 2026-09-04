package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efolchmontiel/wsp-tui/internal/config"
	"github.com/efolchmontiel/wsp-tui/internal/store"
)

type filterMenuItem struct {
	label string
	get   func(*config.FilterVisibility) *bool
	filt  store.ChatFilter
}

func filterMenuItems() []filterMenuItem {
	return []filterMenuItem{
		{"Todos", func(f *config.FilterVisibility) *bool { return &f.All }, store.FilterAll},
		{"Favoritos", func(f *config.FilterVisibility) *bool { return &f.Favorites }, store.FilterFavorites},
		{"Grupos", func(f *config.FilterVisibility) *bool { return &f.Groups }, store.FilterGroups},
		{"Estados", func(f *config.FilterVisibility) *bool { return &f.Estados }, store.FilterEstados},
		{"Novedades", func(f *config.FilterVisibility) *bool { return &f.Novedades }, store.FilterNovedades},
		{"Archivados", func(f *config.FilterVisibility) *bool { return &f.Archived }, store.FilterArchived},
	}
}

func visibleChatFilters(vis config.FilterVisibility) []store.ChatFilter {
	vis = vis.Normalize()
	items := filterMenuItems()
	out := make([]store.ChatFilter, 0, len(items))
	for _, it := range items {
		if *it.get(&vis) {
			out = append(out, it.filt)
		}
	}
	if len(out) == 0 {
		return []store.ChatFilter{store.FilterAll}
	}
	return out
}

func (m Model) filterEnabled(f store.ChatFilter) bool {
	for _, x := range visibleChatFilters(m.cfg.Filters) {
		if x == f {
			return true
		}
	}
	return false
}

func (m *Model) openFilterMenuModal() {
	m.modal = modalFilterMenu
	m.filterDraft = m.cfg.Filters.Normalize()
	m.filterMenuCursor = 0
}

func (m Model) viewFilterMenuModal() string {
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Menú de filtros"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("Elige qué pestañas mostrar en la barra lateral."))
	b.WriteString("\n\n")
	items := filterMenuItems()
	for i, it := range items {
		mark := "[ ]"
		if *it.get(&m.filterDraft) {
			mark = "[x]"
		}
		line := fmt.Sprintf("%s %s", mark, it.label)
		if i == m.filterMenuCursor {
			line = m.theme.sidebarSel.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.help.Render("Espacio marcar · ↑↓ · Enter guardar · Esc cancelar"))
	box := m.theme.box.Width(max(40, min(m.width-4, 56)))
	return box.Render(b.String())
}

func (m Model) updateFilterMenuModalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := filterMenuItems()
	switch msg.String() {
	case "esc", "ctrl+c":
		if msg.String() == "ctrl+c" {
			m.quitting = true
			m.modal = modalNone
			return m, tea.Quit
		}
		m.modal = modalNone
		return m, nil
	case "up", "k":
		if m.filterMenuCursor > 0 {
			m.filterMenuCursor--
		}
		return m, nil
	case "down", "j":
		if m.filterMenuCursor < len(items)-1 {
			m.filterMenuCursor++
		}
		return m, nil
	case " ", "enter":
		if msg.String() == " " {
			it := items[m.filterMenuCursor]
			p := it.get(&m.filterDraft)
			*p = !*p
			m.filterDraft = m.filterDraft.Normalize()
			return m, nil
		}
		return m.saveFilterMenu()
	}
	return m, nil
}

func (m Model) saveFilterMenu() (tea.Model, tea.Cmd) {
	m.cfg.Filters = m.filterDraft.Normalize()
	m.modal = modalNone
	if m.cfgPath != "" {
		if err := config.Save(m.cfgPath, m.cfg); err != nil {
			m.errMsg = err.Error()
			m.modal = modalError
			return m, nil
		}
	}
	if !m.filterEnabled(m.chatFilter) {
		vis := visibleChatFilters(m.cfg.Filters)
		return m.setChatFilter(vis[0])
	}
	m.setInfo("Menú de filtros guardado")
	return m, nil
}

func (m Model) setFilterByDigit(digit int) (tea.Model, tea.Cmd) {
	vis := visibleChatFilters(m.cfg.Filters)
	if digit < 1 || digit > len(vis) {
		return m, nil
	}
	return m.setChatFilter(vis[digit-1])
}
