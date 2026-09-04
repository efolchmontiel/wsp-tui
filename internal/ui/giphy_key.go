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
	b.WriteString(m.theme.title.Render("GIF / Giphy"))
	b.WriteString("\n")
	b.WriteString(m.theme.muted.Render("Proveedor: Tab cicla · auto = Giphy si hay key, si no local"))
	b.WriteString("\n\n")

	prov := config.ParseGIFProvider(string(m.cfg.GIFProvider))
	for _, p := range []config.GIFProvider{config.GIFProviderAuto, config.GIFProviderGiphy, config.GIFProviderLocal} {
		label := p.Label()
		if p == prov {
			b.WriteString(m.theme.accent.Bold(true).Render("[" + label + "]"))
		} else {
			b.WriteString(m.theme.muted.Render(label))
		}
		b.WriteString("  ")
	}
	b.WriteString("\n\n")

	switch prov {
	case config.GIFProviderLocal:
		b.WriteString(m.theme.muted.Render("Modo local: solo archivos .gif del disco (sin búsqueda online)."))
	case config.GIFProviderGiphy:
		b.WriteString(m.theme.muted.Render("Modo giphy: hace falta API key."))
		b.WriteString("\n")
		b.WriteString(m.theme.muted.Render("https://developers.giphy.com/dashboard/"))
	default:
		b.WriteString(m.theme.muted.Render("Modo auto: con key busca en Giphy; sin key usa archivo local."))
		b.WriteString("\n")
		b.WriteString(m.theme.muted.Render("https://developers.giphy.com/dashboard/"))
	}
	b.WriteString("\n\n")

	if prov != config.GIFProviderLocal {
		cur := strings.TrimSpace(m.cfg.GiphyAPIKey)
		if cur == "" {
			b.WriteString(m.theme.muted.Render("Key actual: (vacía)"))
		} else {
			b.WriteString(m.theme.accent.Render("Key actual: " + maskAPIKey(cur)))
		}
		b.WriteString("\n\n")
		b.WriteString(m.giphyKeyInput.View())
		b.WriteString("\n\n")
	}

	switch {
	case m.giphyKeyBusy:
		b.WriteString(m.theme.accent.Render("Validando con Giphy…"))
	case m.giphyKeyStatus != "":
		if strings.HasPrefix(m.giphyKeyStatus, "OK") ||
			strings.HasPrefix(m.giphyKeyStatus, "Key borrada") ||
			strings.HasPrefix(m.giphyKeyStatus, "Proveedor") {
			b.WriteString(m.theme.accent.Render(m.giphyKeyStatus))
		} else {
			b.WriteString(m.theme.statusErr.Render(m.giphyKeyStatus))
		}
	default:
		if prov == config.GIFProviderLocal {
			b.WriteString(m.theme.muted.Render("Tab cambia proveedor · Esc cierra (ya guardado)"))
		} else {
			b.WriteString(m.theme.muted.Render("Enter valida/guarda key · Ctrl+U borra · Tab proveedor · Esc cierra"))
		}
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
	case "tab", "shift+tab":
		m.cfg.GIFProvider = config.CycleGIFProvider(m.cfg.GIFProvider)
		if m.cfgPath != "" {
			_ = config.Save(m.cfgPath, m.cfg)
		}
		m.giphyKeyStatus = "Proveedor: " + m.cfg.GIFProvider.Label()
		if config.ParseGIFProvider(string(m.cfg.GIFProvider)) == config.GIFProviderLocal {
			m.giphyKeyInput.Blur()
		} else {
			m.giphyKeyInput.Focus()
		}
		return m, nil
	case "ctrl+u":
		if config.ParseGIFProvider(string(m.cfg.GIFProvider)) == config.GIFProviderLocal {
			return m, nil
		}
		m.giphyKeyInput.SetValue("")
		m.giphyKeyStatus = ""
		return m, nil
	case "enter":
		if m.giphyKeyBusy {
			return m, nil
		}
		if config.ParseGIFProvider(string(m.cfg.GIFProvider)) == config.GIFProviderLocal {
			m.modal = modalNone
			m.giphyKeyInput.Blur()
			return m, nil
		}
		return m.saveGiphyKeyFromModal()
	}
	if config.ParseGIFProvider(string(m.cfg.GIFProvider)) == config.GIFProviderLocal {
		return m, nil
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
		m.giphyKeyStatus = "Key borrada — sin búsqueda online (auto/local)"
		m.setInfo("GIF: sin API key")
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
