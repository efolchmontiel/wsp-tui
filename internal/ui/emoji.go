package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/config"
	"github.com/efolchmontiel/wsp-tui/internal/engine"
	"github.com/efolchmontiel/wsp-tui/internal/giphy"
	"github.com/efolchmontiel/wsp-tui/internal/media"
	"github.com/efolchmontiel/wsp-tui/internal/preview"
	"github.com/efolchmontiel/wsp-tui/internal/store"
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

type (
	giphySearchMsg struct {
		query   string
		results []giphy.Result
		err     error
	}
	giphySendMsg struct {
		path string
		err  error
	}
	giphyPreviewMsg struct {
		id   string
		path string
		err  error
	}
	giphyOpenMsg struct {
		err error
	}
)

func (m *Model) openEmojiPicker(mode emojiPickerMode) {
	m.pickingEmoji = true
	m.emojiMode = mode
	m.emojiCat = 0
	m.emojiIdx = 0
	m.emojiGIF = false
	m.gifResults = nil
	m.gifCursor = 0
	m.gifBusy = false
	m.gifErr = ""
	m.clearGifPreview()
	m.gifQuery.SetValue("")
	m.gifQuery.Blur()
	m.input.Blur()
}

func (m *Model) closeEmojiPicker() {
	m.pickingEmoji = false
	m.emojiGIF = false
	m.gifQuery.Blur()
	m.gifBusy = false
	m.clearGifPreview()
	if m.emojiMode == emojiModeInsert {
		m.focus = focusInput
		m.applyFocus()
	} else {
		m.focus = focusMessages
		m.applyFocus()
	}
}

func (m *Model) clearGifPreview() {
	if m.gifPreviewPath != "" {
		_ = os.Remove(m.gifPreviewPath)
	}
	m.gifPreviewPath = ""
	m.gifPreviewID = ""
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

func (m Model) hasGiphyKey() bool {
	return strings.TrimSpace(m.cfg.GiphyAPIKey) != ""
}

func (m Model) gifSearchOnline() bool {
	return m.cfg.GIFSearchOnline()
}

func (m Model) gifPickerPreviewSize() (w, h int) {
	boxW := max(48, min(m.width-4, 80))
	w = min(64, max(40, boxW-4))
	h = max(16, min(22, max(16, m.height/3)))
	return w, h
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
		body = m.viewGIFTab()
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

func (m Model) viewGIFTab() string {
	var b strings.Builder
	prov := config.ParseGIFProvider(string(m.cfg.GIFProvider))
	if m.gifSearchOnline() {
		b.WriteString(m.theme.muted.Render("Buscar en Giphy · f = archivo .gif local · Ctrl+G proveedor/key"))
		b.WriteString("\n")
		b.WriteString(m.gifQuery.View())
		b.WriteString("\n\n")
		if m.gifBusy {
			b.WriteString(m.theme.accent.Render("Buscando…"))
		} else if m.gifErr != "" {
			b.WriteString(m.theme.statusErr.Render(m.gifErr))
		} else if len(m.gifResults) == 0 {
			b.WriteString(m.theme.muted.Render("Escribe y Enter para buscar"))
		} else {
			const listWindow = 5
			start := m.gifCursor - listWindow/2
			if start < 0 {
				start = 0
			}
			end := start + listWindow
			if end > len(m.gifResults) {
				end = len(m.gifResults)
				start = max(0, end-listWindow)
			}
			if start > 0 {
				b.WriteString(m.theme.muted.Render("  …"))
				b.WriteString("\n")
			}
			for i := start; i < end; i++ {
				r := m.gifResults[i]
				line := truncate(r.Title, 48)
				if i == m.gifCursor {
					line = m.theme.sidebarSel.Render("▸ " + line)
				} else {
					line = "  " + line
				}
				b.WriteString(line)
				b.WriteString("\n")
			}
			if end < len(m.gifResults) {
				b.WriteString(m.theme.muted.Render("  …"))
				b.WriteString("\n")
			}
			b.WriteString("\n")
			pw, ph := m.gifPickerPreviewSize()
			// Same as chat: Kitty/iTerm blank out inside Bubble Tea; mosaic always shows.
			proto := preview.Halfblocks()
			if m.gifPreviewPath != "" {
				var (
					img string
					err error
				)
				if preview.GIFFrameCount(m.gifPreviewPath) > 1 {
					img, err = preview.RenderGIFFrame(m.gifPreviewPath, pw, ph, m.gifFrame, proto)
				} else {
					img, err = preview.RenderFile(m.gifPreviewPath, pw, ph, proto)
				}
				if err == nil && img != "" {
					b.WriteString(img)
					if !strings.HasSuffix(img, "\n") {
						b.WriteString("\n")
					}
				} else {
					b.WriteString(m.theme.muted.Render("(vista previa no disponible)"))
					b.WriteString("\n")
				}
			} else {
				b.WriteString(m.theme.muted.Render("Cargando vista previa…"))
				b.WriteString("\n")
			}
			b.WriteString("\n")
			b.WriteString(m.theme.muted.Render("↑↓ elegir · o abrir · Enter enviar · f archivo local"))
		}
		return b.String()
	}

	switch prov {
	case config.GIFProviderGiphy:
		b.WriteString(m.theme.statusErr.Render("Proveedor giphy sin API key."))
		b.WriteString("\n\n")
		b.WriteString(m.theme.muted.Render("Ctrl+G para pegar la key, o Tab → auto/local."))
		b.WriteString("\n\n")
		b.WriteString(m.theme.accent.Render("f"))
		b.WriteString(" / Enter = archivo .gif local")
	default:
		b.WriteString(m.theme.muted.Render("Modo local (sin búsqueda online)."))
		b.WriteString("\n\n")
		b.WriteString(m.theme.accent.Render("Enter"))
		b.WriteString(" abrir selector (solo .gif)\n")
		b.WriteString(m.theme.muted.Render("Ctrl+G: cambiar a auto/giphy y agregar key."))
	}
	return b.String()
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
			m.gifQuery.Blur()
			if msg.String() == "tab" {
				m.emojiCat = 0
			} else {
				m.emojiCat = len(emojiCategories) - 1
			}
		} else if msg.String() == "tab" {
			if m.emojiCat >= len(emojiCategories)-1 {
				m.emojiGIF = true
				m.enterGIFTab()
			} else {
				m.emojiCat++
			}
		} else { // shift+tab
			if m.emojiCat <= 0 {
				m.emojiGIF = true
				m.enterGIFTab()
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
		if m.emojiGIF && m.gifSearchOnline() && len(m.gifResults) > 0 {
			if m.gifCursor > 0 {
				m.gifCursor--
			}
			return m, m.ensureGifPreviewCmd()
		}
		if !m.emojiGIF {
			m.emojiIdx = emojiGridMove(m.emojiIdx, len(m.emojiItems()), emojiCols, -1, 0)
		}
		return m, nil
	case "down", "j":
		if m.emojiGIF && m.gifSearchOnline() && len(m.gifResults) > 0 {
			if m.gifCursor < len(m.gifResults)-1 {
				m.gifCursor++
			}
			return m, m.ensureGifPreviewCmd()
		}
		if !m.emojiGIF {
			m.emojiIdx = emojiGridMove(m.emojiIdx, len(m.emojiItems()), emojiCols, 1, 0)
		}
		return m, nil
	case "f":
		if m.emojiGIF {
			m.closeEmojiPicker()
			return m.openGIFPicker()
		}
	case "o":
		if m.emojiGIF && m.gifSearchOnline() && len(m.gifResults) > 0 {
			return m.openSelectedGifExternal()
		}
	case "backspace", "delete":
		if m.emojiMode == emojiModeReact && !m.emojiGIF {
			return m.applyEmojiSelection("")
		}
	case "enter":
		if m.emojiGIF {
			return m.confirmGIFTab()
		}
		items := m.emojiItems()
		if len(items) == 0 {
			return m, nil
		}
		m.emojiIdx = clampEmojiIdx(m.emojiIdx, len(items))
		return m.applyEmojiSelection(items[m.emojiIdx])
	case " ":
		if m.emojiGIF && m.gifSearchOnline() {
			var cmd tea.Cmd
			m.gifQuery, cmd = m.gifQuery.Update(msg)
			return m, cmd
		}
		if m.emojiGIF {
			return m.confirmGIFTab()
		}
		items := m.emojiItems()
		if len(items) == 0 {
			return m, nil
		}
		m.emojiIdx = clampEmojiIdx(m.emojiIdx, len(items))
		return m.applyEmojiSelection(items[m.emojiIdx])
	}
	if m.emojiGIF && m.gifSearchOnline() {
		var cmd tea.Cmd
		m.gifQuery, cmd = m.gifQuery.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) enterGIFTab() {
	m.gifErr = ""
	if m.gifSearchOnline() {
		m.gifQuery.Focus()
	}
}

func (m Model) confirmGIFTab() (tea.Model, tea.Cmd) {
	if !m.gifSearchOnline() {
		m.closeEmojiPicker()
		return m.openGIFPicker()
	}
	if len(m.gifResults) > 0 {
		if m.gifCursor < 0 || m.gifCursor >= len(m.gifResults) {
			m.gifCursor = 0
		}
		sel := m.gifResults[m.gifCursor]
		m.gifBusy = true
		m.gifErr = ""
		m.setInfo("Descargando GIF…")
		return m, downloadGiphyCmd(sel.URL)
	}
	return m.runGiphySearch()
}

func (m Model) runGiphySearch() (tea.Model, tea.Cmd) {
	q := strings.TrimSpace(m.gifQuery.Value())
	if q == "" {
		m.gifErr = "Escribe un término de búsqueda"
		return m, nil
	}
	m.gifBusy = true
	m.gifErr = ""
	m.gifResults = nil
	key := m.cfg.GiphyAPIKey
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := giphy.Search(ctx, key, q, 12)
		return giphySearchMsg{query: q, results: res, err: err}
	}
}

func downloadGiphyCmd(gifURL string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		path, err := giphy.DownloadToTemp(ctx, gifURL)
		return giphySendMsg{path: path, err: err}
	}
}

// openSelectedGifExternal opens the full-resolution GIF with the OS viewer (same as chat `o`).
func (m Model) openSelectedGifExternal() (tea.Model, tea.Cmd) {
	if m.gifCursor < 0 || m.gifCursor >= len(m.gifResults) {
		m.setInfo("Elegí un GIF con ↑↓")
		return m, nil
	}
	sel := m.gifResults[m.gifCursor]
	url := strings.TrimSpace(sel.URL)
	if url == "" {
		url = strings.TrimSpace(sel.PreviewURL)
	}
	local := m.gifPreviewPath
	if sel.ID != "" && sel.ID == m.gifPreviewID {
		// keep local as fast fallback while full URL downloads
	} else {
		local = ""
	}
	if url == "" && local == "" {
		m.setInfo("Sin archivo para abrir")
		return m, nil
	}
	m.setInfo("Abriendo…")
	return m, openGifExternalCmd(url, local)
}

func openGifExternalCmd(fullURL, localPath string) tea.Cmd {
	return func() tea.Msg {
		path := localPath
		var cleanup string
		if fullURL != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			p, err := giphy.DownloadToTemp(ctx, fullURL)
			if err == nil && p != "" {
				path = p
				cleanup = p
			} else if path == "" {
				return giphyOpenMsg{err: err}
			}
		}
		if path == "" {
			return giphyOpenMsg{err: fmt.Errorf("sin archivo local")}
		}
		_, err := media.OpenExternal(path, "image")
		if cleanup != "" {
			go func(p string) {
				time.Sleep(90 * time.Second)
				_ = os.Remove(p)
			}(cleanup)
		}
		return giphyOpenMsg{err: err}
	}
}

func (m Model) applyGiphyOpen(msg giphyOpenMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setInfo("No se pudo abrir: " + msg.err.Error())
		return m, nil
	}
	m.setInfo("Abierto")
	return m, nil
}

func (m Model) ensureGifPreviewCmd() tea.Cmd {
	if !m.pickingEmoji || !m.emojiGIF || len(m.gifResults) == 0 {
		return nil
	}
	if m.gifCursor < 0 || m.gifCursor >= len(m.gifResults) {
		return nil
	}
	sel := m.gifResults[m.gifCursor]
	if sel.ID != "" && sel.ID == m.gifPreviewID && m.gifPreviewPath != "" {
		return nil
	}
	url := strings.TrimSpace(sel.PreviewURL)
	if url == "" {
		url = sel.URL
	}
	if url == "" {
		return nil
	}
	id := sel.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		path, err := giphy.DownloadToTemp(ctx, url)
		return giphyPreviewMsg{id: id, path: path, err: err}
	}
}

func (m Model) applyGiphySearch(msg giphySearchMsg) (tea.Model, tea.Cmd) {
	if !m.pickingEmoji || !m.emojiGIF {
		return m, nil
	}
	m.gifBusy = false
	m.clearGifPreview()
	if msg.err != nil {
		m.gifErr = msg.err.Error()
		m.gifResults = nil
		return m, nil
	}
	m.gifResults = msg.results
	m.gifCursor = 0
	if len(msg.results) == 0 {
		m.gifErr = "Sin resultados"
		return m, nil
	}
	return m, m.ensureGifPreviewCmd()
}

func (m Model) applyGiphyPreview(msg giphyPreviewMsg) (tea.Model, tea.Cmd) {
	if !m.pickingEmoji || !m.emojiGIF {
		if msg.path != "" {
			_ = os.Remove(msg.path)
		}
		return m, nil
	}
	if len(m.gifResults) == 0 || m.gifCursor < 0 || m.gifCursor >= len(m.gifResults) {
		if msg.path != "" {
			_ = os.Remove(msg.path)
		}
		return m, nil
	}
	if msg.id != "" && msg.id != m.gifResults[m.gifCursor].ID {
		if msg.path != "" {
			_ = os.Remove(msg.path)
		}
		return m, nil
	}
	if msg.err != nil || msg.path == "" {
		if msg.path != "" {
			_ = os.Remove(msg.path)
		}
		if msg.err != nil {
			m.gifErr = "Vista previa: " + msg.err.Error()
		}
		return m, nil
	}
	m.gifErr = ""
	m.clearGifPreview()
	m.gifPreviewID = msg.id
	m.gifPreviewPath = msg.path
	return m, nil
}

func (m Model) applyGiphySend(msg giphySendMsg) (tea.Model, tea.Cmd) {
	m.gifBusy = false
	if msg.err != nil {
		m.gifErr = msg.err.Error()
		return m, nil
	}
	path := msg.path
	if path == "" {
		m.gifErr = "descarga vacía"
		return m, nil
	}
	m.closeEmojiPicker()
	if m.selectedID == "" {
		_ = os.Remove(path)
		m.errMsg = "Elige un chat antes de enviar un GIF"
		m.modal = modalError
		return m, nil
	}
	m.uploadNote = "Enviando GIF…"
	eng := m.eng
	chat := m.selectedID
	tmp := path
	return m, func() tea.Msg {
		defer os.Remove(tmp)
		msg, err := eng.SendFile(context.Background(), chat, tmp, "")
		var mp *store.Message
		if err == nil {
			cp := msg
			mp = &cp
		}
		return sendResultMsg{err: err, chatID: chat, msg: mp}
	}
}

func (m Model) applyEmojiSelection(emoji string) (tea.Model, tea.Cmd) {
	mode := m.emojiMode
	m.closeEmojiPicker()
	if mode == emojiModeReact {
		if m.selectedID == "" || len(m.messages) == 0 {
			m.setInfo("Elige un mensaje con [ ]")
			return m, nil
		}
		idx := m.msgCursor
		if idx < 0 || idx >= len(m.messages) {
			idx = len(m.messages) - 1
		}
		target := m.messages[idx]
		label := emoji
		if label == "" {
			label = "(quitar)"
		}
		m.setInfo("Reacción " + label + " → " + truncate(target.Text, 28))
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
		m.errMsg = "Elige un chat antes de enviar un GIF"
		return m, nil
	}
	if m.state != app.StateConnected {
		m.errMsg = "Conéctate antes de enviar archivos"
		return m, nil
	}
	m.pickingFile = true
	fp := m.filePicker
	fp.AllowedTypes = []string{".gif", ".GIF"}
	fp.CurrentDirectory = homeDir()
	fp.Height = max(8, m.height-8)
	m.filePicker = fp
	m.setInfo("GIF: elige un archivo .gif")
	return m, m.filePicker.Init()
}

func sendReactionCmd(eng *engine.Engine, chatID, msgID, emoji string) tea.Cmd {
	return func() tea.Msg {
		err := eng.SendReaction(chatID, msgID, emoji)
		return sendResultMsg{err: err}
	}
}

func newGIFQueryInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "buscar gifs…"
	ti.CharLimit = 50
	ti.Width = 40
	ti.Prompt = "GIF › "
	return ti
}
