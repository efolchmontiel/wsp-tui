package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efolchmontiel/wsp-tui/internal/config"
	"github.com/efolchmontiel/wsp-tui/internal/giphy"
)

type (
	giphyKeyCheckMsg struct {
		key   string
		valid bool
		err   error
	}
)

func newGiphyKeyInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "pega tu Giphy API key…"
	ti.CharLimit = 128
	ti.Width = 48
	ti.Prompt = "key › "
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	return ti
}

func (m *Model) openGiphyKeyModal() {
	m.modal = modalGiphyKey
	m.giphyKeyStatus = ""
	m.giphyKeyBusy = false
	m.giphyKeyInput.SetValue(strings.TrimSpace(m.cfg.GiphyAPIKey))
	m.giphyKeyInput.Focus()
	m.input.Blur()
}

func (m Model) viewGiphyKeyModal() string {
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Giphy API key"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("Opcional. Vacío = solo archivos .gif locales."))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("https://developers.giphy.com/dashboard/"))
	b.WriteString("\n\n")

	cur := strings.TrimSpace(m.cfg.GiphyAPIKey)
	if cur == "" {
		b.WriteString(m.theme.muted.Render("Estado actual: sin key"))
	} else {
		b.WriteString(m.theme.accent.Render("Estado actual: " + maskAPIKey(cur)))
	}
	b.WriteString("\n\n")
	b.WriteString(m.giphyKeyInput.View())
	b.WriteString("\n\n")

	switch {
	case m.giphyKeyBusy:
		b.WriteString(m.theme.accent.Render("Validando con Giphy…"))
	case m.giphyKeyStatus != "":
		if strings.HasPrefix(m.giphyKeyStatus, "OK") || strings.HasPrefix(m.giphyKeyStatus, "Key borrada") {
			b.WriteString(m.theme.accent.Render(m.giphyKeyStatus))
		} else {
			b.WriteString(m.theme.statusErr.Render(m.giphyKeyStatus))
		}
	default:
		b.WriteString(m.theme.muted.Render("Enter valida y guarda · Ctrl+U borra · Esc cierra"))
	}

	box := m.theme.box.Width(max(48, min(m.width-4, 68)))
	return box.Render(b.String())
}

func maskAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return "(vacía)"
	}
	r := []rune(key)
	if len(r) <= 8 {
		return strings.Repeat("•", len(r))
	}
	return string(r[:4]) + strings.Repeat("•", len(r)-8) + string(r[len(r)-4:])
}

func (m Model) updateGiphyKeyModalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.modal = modalNone
		m.giphyKeyInput.Blur()
		m.giphyKeyBusy = false
		return m, nil
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "ctrl+u":
		m.giphyKeyInput.SetValue("")
		m.giphyKeyStatus = ""
		return m, nil
	case "enter":
		if m.giphyKeyBusy {
			return m, nil
		}
		return m.saveGiphyKeyFromModal()
	}
	var cmd tea.Cmd
	m.giphyKeyInput, cmd = m.giphyKeyInput.Update(msg)
	return m, cmd
}

func (m Model) saveGiphyKeyFromModal() (tea.Model, tea.Cmd) {
	key := strings.TrimSpace(m.giphyKeyInput.Value())
	if key == "" {
		m.cfg.GiphyAPIKey = ""
		if m.cfgPath != "" {
			_ = config.Save(m.cfgPath, m.cfg)
		}
		m.giphyKeyStatus = "Key borrada — GIF solo por archivo local"
		m.setInfo("Giphy: sin API key")
		return m, nil
	}
	m.giphyKeyBusy = true
	m.giphyKeyStatus = ""
	return m, validateGiphyKeyCmd(key)
}

func validateGiphyKeyCmd(key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		valid, err := giphy.ValidateKey(ctx, key)
		return giphyKeyCheckMsg{key: key, valid: valid, err: err}
	}
}

func (m Model) applyGiphyKeyCheck(msg giphyKeyCheckMsg) (tea.Model, tea.Cmd) {
	if m.modal != modalGiphyKey {
		return m, nil
	}
	m.giphyKeyBusy = false
	if msg.err != nil {
		m.giphyKeyStatus = "Incorrecta: " + msg.err.Error()
		return m, nil
	}
	if !msg.valid {
		m.giphyKeyStatus = "Incorrecta: API key inválida"
		return m, nil
	}
	m.cfg.GiphyAPIKey = strings.TrimSpace(msg.key)
	if m.cfgPath != "" {
		if err := config.Save(m.cfgPath, m.cfg); err != nil {
			m.giphyKeyStatus = "Válida, pero no se pudo guardar: " + err.Error()
			return m, nil
		}
	}
	m.giphyKeyStatus = "OK — key correcta y guardada (" + maskAPIKey(m.cfg.GiphyAPIKey) + ")"
	m.setInfo("Giphy: key OK")
	return m, nil
}
