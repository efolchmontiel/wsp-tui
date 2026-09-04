package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/engine"
)

type emojiPickerMode int

const (
	emojiModeInsert emojiPickerMode = iota // Ctrl+E → insert into input
	emojiModeReact                         // r → react to selected message
)

type emojiCategory struct {
	Name  string
	Items []string
}

// Built-in emoji set for the TUI picker (offline).
var emojiCategories = []emojiCategory{
	{Name: "Caras", Items: []string{
		"😀", "😃", "😄", "😁", "😆", "😅", "😂", "🤣", "😊", "😇",
		"🙂", "😉", "😌", "😍", "🥰", "😘", "😗", "😙", "😚", "😋",
		"😛", "😜", "🤪", "😝", "🤑", "🤗", "🤭", "🤫", "🤔", "🤐",
		"🤨", "😐", "😑", "😶", "😏", "😒", "🙄", "😬", "😮", "😯",
		"😲", "😳", "🥺", "😢", "😭", "😤", "😠", "😡", "🤬", "😈",
	}},
	{Name: "Gestos", Items: []string{
		"👍", "👎", "👊", "✊", "🤛", "🤜", "👏", "🙌", "👐", "🤲",
		"🤝", "🙏", "✌️", "🤞", "🤟", "🤘", "👌", "🤌", "🤏", "👈",
		"👉", "👆", "👇", "☝️", "✋", "🤚", "🖐", "🖖", "👋", "🤙",
		"💪", "🦾", "✍️", "🤳", "💅", "💃", "🕺", "🫡", "🧘", "👀",
	}},
	{Name: "Corazones", Items: []string{
		"❤️", "🧡", "💛", "💚", "💙", "💜", "🖤", "🤍", "🤎", "💔",
		"❣️", "💕", "💞", "💓", "💗", "💖", "💘", "💝", "💟", "💯",
	}},
	{Name: "Festejo", Items: []string{
		"🎉", "🎊", "🎈", "🎂", "🍰", "🧁", "🥳", "🎁", "🎀", "🎄",
		"🎃", "✨", "⭐", "🌟", "💫", "🔥", "⚡", "💥", "🎯", "🏆",
	}},
	{Name: "Naturaleza", Items: []string{
		"🐶", "🐱", "🐭", "🐹", "🐰", "🦊", "🐻", "🐼", "🐨", "🐯",
		"🦁", "🐮", "🐷", "🐸", "🐵", "🙈", "🙉", "🙊", "🐔", "🐧",
		"🌸", "🌹", "🌺", "🌻", "🌼", "🌷", "🌱", "🌲", "🍀", "🌈",
	}},
	{Name: "Objetos", Items: []string{
		"📱", "💻", "⌚", "📷", "🎮", "🎧", "🎵", "📚", "✏️", "📎",
		"🔒", "🔑", "💡", "🔔", "📌", "📍", "🏠", "✈️", "🚀", "☕",
	}},
}

const emojiCols = 10

func (m *Model) openEmojiPicker(mode emojiPickerMode) {
	m.pickingEmoji = true
	m.emojiMode = mode
	m.emojiCat = 0
	m.emojiIdx = 0
	m.emojiGIF = false
	m.input.Blur()
}

func (m *Model) closeEmojiPicker() {
	m.pickingEmoji = false
	m.emojiGIF = false
	if m.emojiMode == emojiModeInsert {
		m.focus = focusInput
		m.applyFocus()
	} else {
		m.focus = focusMessages
		m.applyFocus()
	}
}

func (m Model) emojiItems() []string {
	if m.emojiGIF || m.emojiCat < 0 || m.emojiCat >= len(emojiCategories) {
		return nil
	}
	return emojiCategories[m.emojiCat].Items
}

func (m *Model) insertEmojiAtCursor(emoji string) {
	val := m.input.Value()
	pos := m.input.Position()
	runes := []rune(val)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	out := string(runes[:pos]) + emoji + string(runes[pos:])
	m.input.SetValue(out)
	m.input.SetCursor(pos + len([]rune(emoji)))
}

func clampEmojiIdx(idx, n int) int {
	if n <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

func emojiGridMove(idx, n, cols, dRow, dCol int) int {
	if n <= 0 {
		return 0
	}
	row := idx / cols
	col := idx % cols
	row += dRow
	col += dCol
	rows := (n + cols - 1) / cols
	if row < 0 {
		row = 0
	}
	if row >= rows {
		row = rows - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= cols {
		col = cols - 1
	}
	return clampEmojiIdx(row*cols+col, n)
}

func (m Model) viewEmojiPicker() string {
	title := "Insertar emoji"
	if m.emojiMode == emojiModeReact {
		title = "Reaccionar al mensaje"
	}
	var tabs []string
	for i, c := range emojiCategories {
		label := c.Name
		if i == m.emojiCat && !m.emojiGIF {
			tabs = append(tabs, m.theme.accent.Bold(true).Render("["+label+"]"))
		} else {
			tabs = append(tabs, m.theme.muted.Render(label))
		}
	}
	if m.emojiGIF {
		tabs = append(tabs, m.theme.accent.Bold(true).Render("[GIF]"))
	} else {
		tabs = append(tabs, m.theme.muted.Render("GIF"))
	}
	tabLine := strings.Join(tabs, " ")

	var body string
	if m.emojiGIF {
		body = m.theme.muted.Render("Elegí un archivo .gif del disco.") + "\n\n" +
			m.theme.accent.Render("Enter") + " abrir selector (solo .gif)\n" +
			m.theme.muted.Render("Tip: Ctrl+O también adjunta cualquier media.")
	} else {
		items := m.emojiItems()
		var b strings.Builder
		for i, em := range items {
			cell := fmt.Sprintf(" %s ", em)
			if i == m.emojiIdx {
				cell = m.theme.sidebarSel.Render(cell)
			}
			b.WriteString(cell)
			if (i+1)%emojiCols == 0 {
				b.WriteString("\n")
			}
		}
		body = b.String()
		if m.emojiMode == emojiModeReact {
			body += "\n\n" + m.theme.muted.Render("Enter = enviar reacción · Backspace = quitar la tuya")
		} else {
			body += "\n\n" + m.theme.muted.Render("Enter = insertar en el input (después Enter envía)")
		}
	}

	box := m.theme.box.Width(max(48, min(m.width-4, 72)))
	help := m.theme.help.Render("←→↑↓ mover · Tab categoría/GIF · Esc cerrar")
	return m.theme.title.Render(title) + "\n\n" +
		box.Render(tabLine+"\n\n"+body) + "\n\n" + help
}

func (m Model) updateEmojiPickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		if msg.String() == "ctrl+c" {
			m.quitting = true
			m.closeEmojiPicker()
			return m, tea.Quit
		}
		m.closeEmojiPicker()
		return m, nil
	case "tab", "shift+tab":
		if m.emojiGIF {
			m.emojiGIF = false
			if msg.String() == "tab" {
				m.emojiCat = 0
			} else {
				m.emojiCat = len(emojiCategories) - 1
			}
		} else if msg.String() == "tab" {
			if m.emojiCat >= len(emojiCategories)-1 {
				m.emojiGIF = true
			} else {
				m.emojiCat++
			}
		} else { // shift+tab
			if m.emojiCat <= 0 {
				m.emojiGIF = true
			} else {
				m.emojiCat--
			}
		}
		m.emojiIdx = 0
		return m, nil
	case "left", "h":
		if !m.emojiGIF {
			m.emojiIdx = emojiGridMove(m.emojiIdx, len(m.emojiItems()), emojiCols, 0, -1)
		}
		return m, nil
	case "right", "l":
		if !m.emojiGIF {
			m.emojiIdx = emojiGridMove(m.emojiIdx, len(m.emojiItems()), emojiCols, 0, 1)
		}
		return m, nil
	case "up", "k":
		if !m.emojiGIF {
			m.emojiIdx = emojiGridMove(m.emojiIdx, len(m.emojiItems()), emojiCols, -1, 0)
		}
		return m, nil
	case "down", "j":
		if !m.emojiGIF {
			m.emojiIdx = emojiGridMove(m.emojiIdx, len(m.emojiItems()), emojiCols, 1, 0)
		}
		return m, nil
	case "backspace", "delete":
		if m.emojiMode == emojiModeReact && !m.emojiGIF {
			return m.applyEmojiSelection("")
		}
		return m, nil
	case "enter", " ":
		if m.emojiGIF {
			m.closeEmojiPicker()
			return m.openGIFPicker()
		}
		items := m.emojiItems()
		if len(items) == 0 {
			return m, nil
		}
		m.emojiIdx = clampEmojiIdx(m.emojiIdx, len(items))
		return m.applyEmojiSelection(items[m.emojiIdx])
	}
	return m, nil
}

func (m Model) applyEmojiSelection(emoji string) (tea.Model, tea.Cmd) {
	mode := m.emojiMode
	m.closeEmojiPicker()
	if mode == emojiModeReact {
		if m.selectedID == "" || len(m.messages) == 0 {
			m.setInfo("Elegí un mensaje con [ ]")
			return m, nil
		}
		idx := m.mediaCursor
		if idx < 0 || idx >= len(m.messages) {
			idx = len(m.messages) - 1
		}
		target := m.messages[idx]
		label := emoji
		if label == "" {
			label = "(quitar)"
		}
		m.setInfo("Reacción " + label)
		return m, sendReactionCmd(m.eng, m.selectedID, target.ID, emoji)
	}
	if emoji != "" {
		m.insertEmojiAtCursor(emoji)
	}
	m.focus = focusInput
	m.applyFocus()
	return m, nil
}

func (m Model) openGIFPicker() (tea.Model, tea.Cmd) {
	if m.selectedID == "" {
		m.errMsg = "Elegí un chat antes de enviar un GIF"
		return m, nil
	}
	if m.state != app.StateConnected {
		m.errMsg = "Conectate antes de enviar archivos"
		return m, nil
	}
	m.pickingFile = true
	fp := m.filePicker
	fp.AllowedTypes = []string{".gif", ".GIF"}
	fp.CurrentDirectory = homeDir()
	fp.Height = max(8, m.height-8)
	m.filePicker = fp
	m.setInfo("GIF: elegí un archivo .gif")
	return m, m.filePicker.Init()
}

func sendReactionCmd(eng *engine.Engine, chatID, msgID, emoji string) tea.Cmd {
	return func() tea.Msg {
		err := eng.SendReaction(chatID, msgID, emoji)
		return sendResultMsg{err: err}
	}
}
