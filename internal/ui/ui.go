package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/config"
	"github.com/efolchmontiel/wsp-tui/internal/engine"
	"github.com/efolchmontiel/wsp-tui/internal/giphy"
	"github.com/efolchmontiel/wsp-tui/internal/notify"
	"github.com/efolchmontiel/wsp-tui/internal/preview"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"github.com/efolchmontiel/wsp-tui/internal/version"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	messagePageSize = 50
	// Each sidebar chat occupies name + preview.
	sidebarChatRows = 2
	// filter bar + "Chats" title inside the bordered sidebar.
	sidebarHeaderRows = 2
)

type focusPane int

const (
	focusSidebar focusPane = iota
	focusMessages
	focusInput
)

type modalKind int

const (
	modalNone modalKind = iota
	modalHelp
	modalError
	modalConfirmDelete
	modalRetention
	modalGiphyKey
)

// Model is the root TUI.
type Model struct {
	theme   theme
	cfg     config.Config
	cfgPath string
	bus     *app.Bus
	eng     *engine.Engine
	store   *store.Store
	width   int
	height  int

	state     app.ConnectionState
	status    string
	qrArt     string
	pairCode  string
	errMsg    string
	infoMsg   string
	syncNote  string
	showLogin bool

	pairingMode bool
	phoneInput  textinput.Model

	focus       focusPane
	chats       []store.Chat
	chatCursor  int
	selectedID  string
	messages    []store.Message
	msgVP       viewport.Model
	input       textinput.Model
	sidebarOff  int
	loadingMsgs bool
	quitting    bool

	// Phase 3: coalesce expensive sidebar reloads while input stays hot.
	chatsDirtyGen int

	// Phase 4: multimedia
	pickingFile bool
	filePicker  filepicker.Model
	uploadNote  string
	mediaHint   string // last media state tip

	// Emoji / GIF picker + reactions
	pickingEmoji bool
	emojiMode    emojiPickerMode
	emojiCat     int
	emojiIdx     int
	emojiGIF     bool

	// Phase 5: UX
	searching    bool
	searchInput  textinput.Model
	searchHits   []store.SearchHit
	searchCursor int
	searchGen    int
	modal        modalKind
	infoUntil    time.Time

	// Contacts: add + delete
	addingContact bool
	addName       textinput.Model
	addPhone      textinput.Model
	addField      int // 0=name, 1=phone
	pendingDelete string

	// After requesting a resend, auto-open when MediaID arrives.
	pendingOpenMsgID string
	pendingOpenChat  string

	// Sidebar filters + media navigation + playback visualizer
	chatFilter      store.ChatFilter
	msgCursor       int // selected message index for [ ] / r / o / d (-1 none)
	playingMediaID  string
	playingMsgID    string
	wavePhase       float64
	desktopNotify   bool
	gifFrame        int // advances for animated GIF previews

	// Retention settings modal (key R)
	retCursor int
	retCustom bool
	retUnit   config.RetentionUnit
	retAmount textinput.Model

	// Giphy search (optional; needs giphy_api_key)
	gifQuery   textinput.Model
	gifResults []giphy.Result
	gifCursor  int
	gifBusy    bool
	gifErr     string

	// Giphy API key modal (Ctrl+G)
	giphyKeyInput  textinput.Model
	giphyKeyStatus string
	giphyKeyBusy   bool
}

type (
	engineEventMsg app.Event
	tickMsg        time.Time
	chatsLoadedMsg struct {
		chats []store.Chat
		err   error
	}
	messagesLoadedMsg struct {
		chatID string
		msgs   []store.Message
		err    error
	}
	olderMessagesMsg struct {
		chatID string
		msgs   []store.Message
		err    error
	}
	pairResultMsg struct {
		code string
		err  error
	}
	sendResultMsg   struct{ err error }
	chatsReloadMsg  struct{ gen int }
	searchReloadMsg struct {
		gen   int
		query string
	}
	searchLoadedMsg struct {
		gen  int
		hits []store.SearchHit
		err  error
	}
	startChatMsg struct {
		chatID string
		name   string
		err    error
	}
	addContactMsg struct {
		chatID string
		name   string
		err    error
	}
	deleteChatMsg struct {
		chatID string
		err    error
	}
)

// New creates the root model.
func New(bus *app.Bus, eng *engine.Engine, st *store.Store, hasSession bool, cfg config.Config, cfgPath string) Model {
	ti := textinput.New()
	ti.Placeholder = "Escribe… (Enter enviar · Ctrl+E emoji/GIF · / buscar · ? ayuda)"
	ti.CharLimit = 4096
	ti.Prompt = "› "

	phone := textinput.New()
	phone.Placeholder = "54911..."
	phone.CharLimit = 20
	phone.Width = 24

	fp := filepicker.New()
	fp.CurrentDirectory = homeDir()
	fp.FileAllowed = true
	fp.DirAllowed = false
	fp.ShowHidden = false
	fp.AutoHeight = false
	fp.Height = 12

	si := textinput.New()
	si.Placeholder = "Buscar chats, mensajes o contactos…"
	si.CharLimit = 120
	si.Prompt = "/ "
	si.Width = 48

	an := textinput.New()
	an.Placeholder = "Nombre (ej. Papito)"
	an.CharLimit = 80
	an.Prompt = "Nombre  › "
	an.Width = 40

	ap := textinput.New()
	ap.Placeholder = "54911… o +569…"
	ap.CharLimit = 24
	ap.Prompt = "Teléfono › "
	ap.Width = 40

	if cfg.Theme == "" {
		cfg.Theme = "dark"
	}

	return Model{
		theme:         themeByName(cfg.Theme),
		cfg:           cfg,
		cfgPath:       cfgPath,
		bus:           bus,
		eng:           eng,
		store:         st,
		state:         app.StateStarting,
		status:        "Starting…",
		showLogin:     !hasSession,
		phoneInput:    phone,
		input:         ti,
		searchInput:   si,
		addName:       an,
		addPhone:      ap,
		focus:         focusSidebar,
		msgVP:         viewport.New(40, 10),
		filePicker:    fp,
		desktopNotify: true,
		msgCursor:     -1,
		retAmount:     newRetentionAmountInput(),
		retUnit:       config.UnitMonth,
		gifQuery:      newGIFQueryInput(),
		giphyKeyInput: newGiphyKeyInput(),
	}
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return "."
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitEvent(m.bus),
		loadChatsCmd(m.store, m.chatFilter),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func waitEvent(bus *app.Bus) tea.Cmd {
	return func() tea.Msg {
		return engineEventMsg(<-bus.Events())
	}
}

func loadChatsCmd(st *store.Store, filter store.ChatFilter) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		chats, err := st.ListChatsFiltered(ctx, filter, 200)
		return chatsLoadedMsg{chats: chats, err: err}
	}
}

func loadMessagesCmd(st *store.Store, chatID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		msgs, err := st.ListMessages(ctx, chatID, messagePageSize, 0)
		return messagesLoadedMsg{chatID: chatID, msgs: msgs, err: err}
	}
}

func loadOlderMessagesCmd(st *store.Store, chatID string, beforeTS int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		msgs, err := st.ListMessages(ctx, chatID, messagePageSize, beforeTS)
		return olderMessagesMsg{chatID: chatID, msgs: msgs, err: err}
	}
}

func sendTextCmd(eng *engine.Engine, chatID, text string) tea.Cmd {
	return func() tea.Msg {
		_, err := eng.SendText(context.Background(), chatID, text)
		return sendResultMsg{err: err}
	}
}

func sendFileCmd(eng *engine.Engine, chatID, path, caption string) tea.Cmd {
	return func() tea.Msg {
		_, err := eng.SendFile(context.Background(), chatID, path, caption)
		return sendResultMsg{err: err}
	}
}

func openMediaCmd(eng *engine.Engine, mediaID string) tea.Cmd {
	return func() tea.Msg {
		err := eng.OpenMedia(mediaID)
		return sendResultMsg{err: err}
	}
}

func downloadMediaCmd(eng *engine.Engine, mediaID string) tea.Cmd {
	return func() tea.Msg {
		err := eng.DownloadMedia(mediaID)
		return sendResultMsg{err: err}
	}
}

func requestPairCmd(eng *engine.Engine, phone string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		code, err := eng.RequestPairingCode(ctx, phone)
		return pairResultMsg{code: code, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relayout()
		return m, nil

	case tickMsg:
		interval := time.Second
		needRefresh := false
		if m.playingMediaID != "" {
			interval = 120 * time.Millisecond
			m.wavePhase += 0.22
			needRefresh = true
		}
		if m.cfg.ShowMediaPreviews && m.hasAnimatedPreview() {
			interval = 120 * time.Millisecond
			m.gifFrame++
			needRefresh = true
		}
		if needRefresh {
			m.refreshViewport(false)
		}
		cmds := []tea.Cmd{tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })}
		if !m.infoUntil.IsZero() && time.Now().After(m.infoUntil) {
			m.infoMsg = ""
			m.infoUntil = time.Time{}
		}
		if m.eng != nil && m.eng.IsRecording() {
			sec := int(m.eng.RecordingElapsed().Seconds())
			m.uploadNote = fmt.Sprintf("● REC %ds  (v enviar · Esc cancelar)", sec)
		}
		return m, tea.Batch(cmds...)

	case engineEventMsg:
		cmd := m.applyEvent(app.Event(msg))
		return m, tea.Batch(waitEvent(m.bus), cmd)

	case chatsReloadMsg:
		if msg.gen != m.chatsDirtyGen {
			return m, nil // superseded by a newer dirty signal
		}
		return m, loadChatsCmd(m.store, m.chatFilter)

	case searchReloadMsg:
		if msg.gen != m.searchGen {
			return m, nil
		}
		return m, searchCmd(m.store, m.eng, msg.gen, msg.query)

	case searchLoadedMsg:
		if msg.gen != m.searchGen {
			return m, nil
		}
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.modal = modalError
			return m, nil
		}
		m.searchHits = msg.hits
		if m.searchCursor >= len(m.searchHits) {
			m.searchCursor = max(0, len(m.searchHits)-1)
		}
		return m, nil

	case chatsLoadedMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.chats = msg.chats
		m.syncCursorToSelected()
		return m, nil

	case messagesLoadedMsg:
		m.loadingMsgs = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		if msg.chatID != m.selectedID {
			return m, nil
		}
		m.messages = msg.msgs
		m.selectLastMsgCursor()
		m.refreshViewport(true)
		return m, nil

	case giphySearchMsg:
		return m.applyGiphySearch(msg)

	case giphySendMsg:
		return m.applyGiphySend(msg)

	case giphyKeyCheckMsg:
		return m.applyGiphyKeyCheck(msg)

	case olderMessagesMsg:
		m.loadingMsgs = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		if msg.chatID != m.selectedID || len(msg.msgs) == 0 {
			return m, nil
		}
		m.messages = append(msg.msgs, m.messages...)
		m.refreshViewport(false)
		return m, nil

	case pairResultMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.pairCode = msg.code
		m.state = app.StatePairingCode
		m.status = "Enter pairing code on phone"
		return m, nil

	case sendResultMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.modal = modalError
		}
		return m, nil

	case startChatMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.modal = modalError
			return m, nil
		}
		return m.openChatID(msg.chatID, msg.name)

	case addContactMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.modal = modalError
			return m, nil
		}
		m.setInfo("Contacto agregado")
		return m.openChatID(msg.chatID, msg.name)

	case deleteChatMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.modal = modalError
			return m, nil
		}
		if m.selectedID == msg.chatID {
			m.selectedID = ""
			m.messages = nil
			m.refreshViewport(true)
		}
		m.setInfo("Chat eliminado")
		return m, loadChatsCmd(m.store, m.chatFilter)

	case tea.MouseMsg:
		if m.pickingFile || m.pickingEmoji || m.searching || m.modal != modalNone {
			return m, nil
		}
		needLogin := m.showLogin && m.state != app.StateConnected &&
			m.state != app.StateReconnecting && m.state != app.StateConnecting
		if needLogin || m.state == app.StateQR || m.state == app.StateNeedsLogin || m.state == app.StatePairingCode {
			if m.state != app.StateConnected {
				return m, nil
			}
		}
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if m.modal == modalConfirmDelete {
			return m.updateConfirmDeleteKeys(msg)
		}
		if m.modal != modalNone {
			return m.updateModalKeys(msg)
		}
		if m.addingContact {
			return m.updateAddContactKeys(msg)
		}
		if m.searching {
			return m.updateSearchKeys(msg)
		}
		if m.pickingEmoji {
			return m.updateEmojiPickerKeys(msg)
		}
		if m.pickingFile {
			return m.updatePickerKeys(msg)
		}
		needLogin := m.showLogin && m.state != app.StateConnected &&
			m.state != app.StateReconnecting && m.state != app.StateConnecting
		if needLogin || m.state == app.StateQR || m.state == app.StateNeedsLogin || m.state == app.StatePairingCode {
			if m.state != app.StateConnected {
				return m.updateLoginKeys(msg)
			}
		}
		return m.updateMainKeys(msg)
	}
	if m.pickingFile {
		var cmd tea.Cmd
		m.filePicker, cmd = m.filePicker.Update(msg)
		if did, path := m.filePicker.DidSelectFile(msg); did {
			m.pickingFile = false
			caption := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			m.uploadNote = "Adjuntando…"
			return m, tea.Batch(cmd, sendFileCmd(m.eng, m.selectedID, path, caption))
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) applyEvent(evt app.Event) tea.Cmd {
	switch evt.Kind {
	case app.EventQR:
		m.qrArt = renderQR(evt.QRCode)
		m.state = app.StateQR
		m.showLogin = true
		m.status = "Scan QR"
		m.errMsg = ""
	case app.EventPairCode:
		m.pairCode = evt.PairCode
		m.state = app.StatePairingCode
		m.showLogin = true
	case app.EventStateChanged:
		m.state = evt.State
		m.status = humanStatus(evt.State, evt.Message)
		if evt.State == app.StateConnected {
			m.showLogin = false
			m.errMsg = ""
			m.qrArt = ""
			return loadChatsCmd(m.store, m.chatFilter)
		}
		if evt.State == app.StateLoggedOut || evt.State == app.StateNeedsLogin {
			m.showLogin = true
		}
	case app.EventError:
		m.errMsg = evt.Message
		m.modal = modalError
	case app.EventInfo:
		m.setInfo(evt.Message)
	case app.EventSyncProgress:
		m.syncNote = evt.Message
	case app.EventChatsDirty, app.EventChatUpserted:
		return m.scheduleChatsReload()
	case app.EventMessageUpserted:
		if evt.Msg != nil {
			if !evt.IsReaction {
				m.patchChatPreview(*evt.Msg)
			}
			if evt.Msg.ChatID == m.selectedID {
				m.upsertLocalMessage(*evt.Msg)
				if m.msgCursor < 0 {
					m.selectLastMsgCursor()
				}
				m.refreshViewport(false)
			}
			if !evt.IsReaction && m.desktopNotify && !evt.Msg.IsFromMe && evt.Msg.ChatID != m.selectedID {
				title := "WhatsTUI"
				for _, c := range m.chats {
					if c.ID == evt.Msg.ChatID {
						if c.Name != "" {
							title = c.Name
						}
						break
					}
				}
				go notify.Desktop(title, evt.Msg.Text)
			}
			if m.pendingOpenChat != "" && evt.Msg.ChatID == m.pendingOpenChat &&
				evt.Msg.ID == m.pendingOpenMsgID && evt.Msg.MediaID != "" {
				id := evt.Msg.MediaID
				m.pendingOpenChat = ""
				m.pendingOpenMsgID = ""
				m.playingMsgID = evt.Msg.ID
				m.setInfo("Adjunto recuperado — abriendo…")
				return tea.Batch(m.scheduleChatsReload(), openMediaCmd(m.eng, id))
			}
		}
		if evt.IsReaction {
			return nil
		}
		return m.scheduleChatsReload()
	case app.EventMessageStatus:
		if evt.ChatID == m.selectedID {
			idset := map[string]struct{}{}
			for _, id := range evt.MessageIDs {
				idset[id] = struct{}{}
			}
			for i := range m.messages {
				if _, ok := idset[m.messages[i].ID]; ok {
					m.messages[i].Status = evt.Status
				}
			}
			m.refreshViewport(false)
		}
	case app.EventMediaPlaying:
		m.playingMediaID = evt.MediaID
		if evt.Message != "" {
			m.playingMsgID = evt.Message
		}
		m.wavePhase = 0
		m.setInfo("Reproduciendo…")
		m.refreshViewport(false)
		return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
	case app.EventMediaStopped:
		// Ignore stale stop from a clip we already replaced.
		if evt.MediaID == "" || evt.MediaID == m.playingMediaID {
			m.playingMediaID = ""
			m.playingMsgID = ""
			m.refreshViewport(false)
		}
	case app.EventMediaUpdated:
		m.mediaHint = evt.Message
		if evt.Status == store.MediaReady {
			m.setInfo("Media listo")
		} else if evt.Status == store.MediaFailed {
			m.errMsg = "Descarga falló: " + evt.Message
			m.modal = modalError
		} else if evt.Message != "" {
			m.setInfo(evt.Message)
		}
		m.refreshViewport(false)
	case app.EventUploadProgress:
		if evt.Progress < 0 {
			m.uploadNote = ""
			m.errMsg = evt.Message
			m.modal = modalError
		} else if evt.Progress >= 100 {
			m.uploadNote = ""
			m.setInfo(evt.Message)
		} else {
			m.uploadNote = fmt.Sprintf("%s (%d%%)", evt.Message, evt.Progress)
		}
	}
	return nil
}

func (m *Model) setInfo(msg string) {
	m.infoMsg = msg
	m.infoUntil = time.Now().Add(4 * time.Second)
}

func (m *Model) scheduleChatsReload() tea.Cmd {
	m.chatsDirtyGen++
	gen := m.chatsDirtyGen
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return chatsReloadMsg{gen: gen}
	})
}

func (m *Model) patchChatPreview(msg store.Message) {
	preview := msg.Text
	if len([]rune(preview)) > 80 {
		r := []rune(preview)
		preview = string(r[:79]) + "…"
	}
	preview = strings.ReplaceAll(preview, "\n", " ")
	found := false
	for i := range m.chats {
		if m.chats[i].ID != msg.ChatID {
			continue
		}
		found = true
		m.chats[i].LastMessage = preview
		m.chats[i].LastMessageAt = msg.Timestamp
		if !msg.IsFromMe && msg.ChatID != m.selectedID {
			m.chats[i].UnreadCount++
		}
		break
	}
	if !found {
		m.chats = append([]store.Chat{{
			ID:            msg.ChatID,
			LastMessage:   preview,
			LastMessageAt: msg.Timestamp,
			UnreadCount:   boolToUnread(!msg.IsFromMe),
		}}, m.chats...)
	}
}

func boolToUnread(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (m Model) updateLoginKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pairingMode {
		switch msg.String() {
		case "esc":
			m.pairingMode = false
			m.phoneInput.Blur()
			return m, nil
		case "enter":
			phone := strings.TrimSpace(m.phoneInput.Value())
			m.pairingMode = false
			m.phoneInput.Blur()
			return m, requestPairCmd(m.eng, phone)
		}
		var cmd tea.Cmd
		m.phoneInput, cmd = m.phoneInput.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "p":
		m.pairingMode = true
		m.phoneInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) updateMainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Voice note takes Esc / v even while "recording overlay" semantics.
	if m.eng != nil && m.eng.IsRecording() {
		switch msg.String() {
		case "esc":
			m.eng.CancelVoiceRecord()
			m.uploadNote = ""
			return m, nil
		case "v", "enter":
			return m.finishVoiceNote()
		case "ctrl+c":
			m.eng.CancelVoiceRecord()
			m.quitting = true
			return m, tea.Quit
		}
	}

	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "q":
		if m.focus != focusInput {
			m.quitting = true
			return m, tea.Quit
		}
	case "ctrl+o":
		return m.openFilePicker()
	case "ctrl+e":
		m.openEmojiPicker(emojiModeInsert)
		return m, nil
	case "ctrl+g":
		m.openGiphyKeyModal()
		return m, nil
	case "r":
		if m.focus != focusInput || strings.TrimSpace(m.input.Value()) == "" {
			if m.selectedID == "" || len(m.messages) == 0 {
				m.setInfo("Abrí un chat y elegí un mensaje ([ ])")
				return m, nil
			}
			m.openEmojiPicker(emojiModeReact)
			return m, nil
		}
	case "ctrl+f":
		return m.openSearch()
	case "a":
		if m.focus != focusInput {
			return m.openAddContact()
		}
	case "x", "delete":
		if m.focus == focusSidebar {
			return m.askDeleteSelected()
		}
	case "v":
		if m.focus != focusInput || strings.TrimSpace(m.input.Value()) == "" {
			return m.toggleVoiceNote()
		}
	case "1":
		if m.focus != focusInput {
			return m.setChatFilter(store.FilterAll)
		}
	case "2":
		if m.focus != focusInput {
			return m.setChatFilter(store.FilterFavorites)
		}
	case "3":
		if m.focus != focusInput {
			return m.setChatFilter(store.FilterGroups)
		}
	case "4":
		if m.focus != focusInput {
			return m.setChatFilter(store.FilterNovedades)
		}
	case "5":
		if m.focus != focusInput {
			return m.setChatFilter(store.FilterArchived)
		}
	case "e":
		if m.focus != focusInput {
			return m.toggleArchiveSelected()
		}
	case "f", "*":
		if m.focus != focusInput {
			return m.toggleFavoriteSelected()
		}
	case "m":
		if m.focus != focusInput {
			return m.cycleDisappearing()
		}
	case "[":
		if m.focus != focusInput {
			m.moveMsgCursor(-1)
			return m, nil
		}
	case "]":
		if m.focus != focusInput {
			m.moveMsgCursor(1)
			return m, nil
		}
	case "?":
		if m.focus != focusInput {
			m.modal = modalHelp
			return m, nil
		}
	case "t":
		if m.focus != focusInput {
			return m.cycleTheme()
		}
	case "R":
		if m.focus != focusInput {
			m.openRetentionModal()
			return m, nil
		}
	case "/":
		if m.focus != focusInput {
			return m.openSearch()
		}
	case "tab":
		m.focus = (m.focus + 1) % 3
		m.applyFocus()
		return m, nil
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
		m.applyFocus()
		return m, nil
	case "ctrl+h":
		m.focus = focusSidebar
		m.applyFocus()
		return m, nil
	case "ctrl+l":
		m.focus = focusInput
		m.applyFocus()
		return m, textinput.Blink
	}

	switch m.focus {
	case focusSidebar:
		return m.updateSidebarKeys(msg)
	case focusMessages:
		return m.updateMessageKeys(msg)
	default:
		return m.updateInputKeys(msg)
	}
}

func (m Model) toggleVoiceNote() (tea.Model, tea.Cmd) {
	if m.selectedID == "" {
		m.setInfo("Elegí un chat antes de grabar")
		return m, nil
	}
	if m.state != app.StateConnected {
		m.errMsg = "Conectate antes de enviar voz"
		m.modal = modalError
		return m, nil
	}
	if m.eng.IsRecording() {
		return m.finishVoiceNote()
	}
	if err := m.eng.StartVoiceRecord(); err != nil {
		m.errMsg = err.Error()
		m.modal = modalError
		return m, nil
	}
	m.uploadNote = "● REC 0s  (v enviar · Esc cancelar)"
	m.focus = focusMessages
	m.applyFocus()
	return m, nil
}

func (m Model) finishVoiceNote() (tea.Model, tea.Cmd) {
	chatID := m.selectedID
	if chatID == "" {
		m.eng.CancelVoiceRecord()
		m.uploadNote = ""
		m.setInfo("Sin chat seleccionado")
		return m, nil
	}
	m.uploadNote = "Enviando nota de voz…"
	eng := m.eng
	return m, func() tea.Msg {
		_, err := eng.StopVoiceRecordAndSend(context.Background(), chatID)
		return sendResultMsg{err: err}
	}
}

func (m Model) cycleTheme() (tea.Model, tea.Cmd) {
	next := nextTheme(m.theme.name)
	m.theme = themeByName(string(next))
	m.cfg.Theme = string(next)
	if m.cfgPath != "" {
		_ = config.Save(m.cfgPath, m.cfg)
	}
	m.setInfo("Tema: " + string(next))
	m.refreshViewport(false)
	return m, nil
}

func (m Model) openSearch() (tea.Model, tea.Cmd) {
	m.searching = true
	m.searchHits = nil
	m.searchCursor = 0
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	m.input.Blur()
	return m, textinput.Blink
}

func (m Model) updateModalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal == modalRetention {
		return m.updateRetentionModalKeys(msg)
	}
	if m.modal == modalGiphyKey {
		return m.updateGiphyKeyModalKeys(msg)
	}
	switch msg.String() {
	case "esc", "enter", "q", "?", "ctrl+c":
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		m.modal = modalNone
		if m.errMsg != "" && msg.String() != "?" {
			// keep err in footer briefly after dismissing modal
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.searching = false
		m.searchInput.Blur()
		m.searchHits = nil
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	case "ctrl+n":
		m.searching = false
		m.searchInput.Blur()
		return m.openAddContact()
	case "up", "ctrl+k":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.searchCursor < len(m.searchHits)-1 {
			m.searchCursor++
		}
		return m, nil
	case "enter":
		return m.applySearchHit()
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchGen++
	gen := m.searchGen
	q := m.searchInput.Value()
	return m, tea.Batch(cmd, tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg {
		return searchReloadMsg{gen: gen, query: q}
	}))
}

func (m Model) applySearchHit() (tea.Model, tea.Cmd) {
	if m.searchCursor < 0 || m.searchCursor >= len(m.searchHits) {
		m.searching = false
		m.searchInput.Blur()
		return m, nil
	}
	hit := m.searchHits[m.searchCursor]
	m.searching = false
	m.searchInput.Blur()
	if hit.Kind == "contact" {
		return m, startChatCmd(m.eng, hit.ChatID, hit.ChatName)
	}
	return m.openChatID(hit.ChatID, hit.ChatName)
}

func (m Model) openFilePicker() (tea.Model, tea.Cmd) {
	if m.selectedID == "" {
		m.errMsg = "Elegí un chat antes de adjuntar"
		return m, nil
	}
	if m.state != app.StateConnected {
		m.errMsg = "Conectate antes de enviar archivos"
		return m, nil
	}
	m.pickingFile = true
	m.filePicker.AllowedTypes = nil // any file
	m.filePicker.Height = max(8, m.height-8)
	return m, m.filePicker.Init()
}

func (m Model) updatePickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.pickingFile = false
		m.infoMsg = "Adjuntar cancelado"
		return m, nil
	}
	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)
	if did, path := m.filePicker.DidSelectFile(msg); did {
		m.pickingFile = false
		caption := strings.TrimSpace(m.input.Value())
		m.input.SetValue("")
		m.uploadNote = "Adjuntando…"
		return m, tea.Batch(cmd, sendFileCmd(m.eng, m.selectedID, path, caption))
	}
	return m, cmd
}

func (m Model) lastMediaMessage() (store.Message, bool) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.MediaID != "" || isMediaType(msg.Type) || looksLikeMediaText(msg.Text) {
			return msg, true
		}
	}
	return store.Message{}, false
}

func isMediaType(t string) bool {
	switch t {
	case store.TypeImage, store.TypeVideo, store.TypeAudio, store.TypeDocument, store.TypeSticker:
		return true
	default:
		return false
	}
}

func looksLikeMediaText(text string) bool {
	t := strings.ToLower(text)
	return strings.Contains(t, "voice") || strings.Contains(t, "voz") ||
		strings.Contains(t, "🎤") || strings.Contains(t, "audio") ||
		strings.HasPrefix(t, "img") || strings.HasPrefix(t, "vid") ||
		strings.Contains(t, "imagen") || strings.Contains(t, "video") ||
		strings.Contains(t, "documento") || strings.Contains(t, "sticker")
}

func (m Model) openOrRecoverMedia() (tea.Model, tea.Cmd) {
	return m.openSelectedMedia()
}

func (m Model) downloadOrRecoverMedia() (tea.Model, tea.Cmd) {
	var msg store.Message
	ok := false
	if m.msgCursor >= 0 && m.msgCursor < len(m.messages) {
		cand := m.messages[m.msgCursor]
		if cand.MediaID != "" || isMediaType(cand.Type) || looksLikeMediaText(cand.Text) {
			msg, ok = cand, true
		}
	}
	if !ok {
		msg, ok = m.lastMediaMessage()
	}
	if !ok {
		m.setInfo("No hay adjuntos en este chat")
		return m, nil
	}
	if msg.MediaID != "" {
		m.setInfo("Descargando…")
		return m, downloadMediaCmd(m.eng, msg.MediaID)
	}
	m.pendingOpenChat = msg.ChatID
	m.pendingOpenMsgID = msg.ID
	m.setInfo("Sin claves de descarga — pidiendo el adjunto al teléfono…")
	return m, requestResendCmd(m.eng, msg)
}

func requestResendCmd(eng *engine.Engine, msg store.Message) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := eng.RequestMessageResend(ctx, msg.ChatID, msg.Sender, msg.ID)
		return sendResultMsg{err: err}
	}
}

func (m Model) lastMediaID() string {
	msg, ok := m.lastMediaMessage()
	if !ok {
		return ""
	}
	return msg.MediaID
}

func (m Model) updateSidebarKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.chats)
	switch msg.String() {
	case "up", "k":
		if m.chatCursor > 0 {
			m.chatCursor--
			m.ensureSidebarVisible()
		}
	case "down", "j":
		if m.chatCursor < n-1 {
			m.chatCursor++
			m.ensureSidebarVisible()
		}
	case "pgup":
		m.chatCursor = max(0, m.chatCursor-10)
		m.ensureSidebarVisible()
	case "pgdown":
		m.chatCursor = min(n-1, m.chatCursor+10)
		m.ensureSidebarVisible()
	case "home":
		m.chatCursor = 0
		m.ensureSidebarVisible()
	case "end":
		if n > 0 {
			m.chatCursor = n - 1
			m.ensureSidebarVisible()
		}
	case "enter", "right", "l":
		return m.selectChatAtCursor()
	case "g":
		return m.cyclePronoun()
	}
	return m, nil
}

func (m Model) cycleDisappearing() (tea.Model, tea.Cmd) {
	id := m.selectedSidebarChatID()
	if id == "" {
		m.setInfo("Elegí un chat")
		return m, nil
	}
	if m.state != app.StateConnected {
		m.errMsg = "Conectate antes de cambiar mensajes temporales"
		m.modal = modalError
		return m, nil
	}
	eng := m.eng
	return m, func() tea.Msg {
		label, err := eng.CycleDisappearingTimer(context.Background(), id)
		if err != nil {
			return sendResultMsg{err: err}
		}
		return engineEventMsg(app.Event{Kind: app.EventInfo, Message: "Mensajes temporales: " + label})
	}
}

func (m Model) cyclePronoun() (tea.Model, tea.Cmd) {
	if m.selectedID == "" && m.chatCursor >= 0 && m.chatCursor < len(m.chats) {
		m.selectedID = m.chats[m.chatCursor].ID
	}
	if m.selectedID == "" {
		return m, nil
	}
	cur := m.store.GetChatPronoun(context.Background(), m.selectedID)
	next := store.PronounEl
	switch cur {
	case store.PronounEl:
		next = store.PronounElla
	case store.PronounElla:
		next = store.PronounAuto
	}
	_ = m.store.SetChatPronoun(context.Background(), m.selectedID, next)
	m.infoMsg = "Pronombre: " + pronounLabel(next)
	m.refreshViewport(false)
	return m, nil
}

func (m Model) updateMessageKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.msgVP, cmd = m.msgVP.Update(msg)
	switch msg.String() {
	case "enter", "i":
		m.focus = focusInput
		m.applyFocus()
		return m, textinput.Blink
	case "o":
		mod, c := m.openOrRecoverMedia()
		return mod, tea.Batch(cmd, c)
	case "r":
		if m.selectedID == "" || len(m.messages) == 0 {
			m.setInfo("Abrí un chat y elegí un mensaje ([ ])")
			return m, cmd
		}
		m.openEmojiPicker(emojiModeReact)
		return m, nil
	case "d":
		mod, c := m.downloadOrRecoverMedia()
		return mod, tea.Batch(cmd, c)
	case "g":
		return m.cyclePronoun()
	case "pgup":
		if m.msgVP.AtTop() && m.selectedID != "" && len(m.messages) > 0 && !m.loadingMsgs {
			m.loadingMsgs = true
			return m, tea.Batch(cmd, loadOlderMessagesCmd(m.store, m.selectedID, m.messages[0].Timestamp))
		}
	}
	return m, cmd
}

func (m Model) updateInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" || m.selectedID == "" {
			return m, nil
		}
		m.input.SetValue("")
		return m, sendTextCmd(m.eng, m.selectedID, text)
	case "esc":
		m.focus = focusMessages
		m.applyFocus()
		return m, nil
	case "ctrl+e":
		m.openEmojiPicker(emojiModeInsert)
		return m, nil
	case "ctrl+g":
		m.openGiphyKeyModal()
		return m, nil
	case "ctrl+o":
		return m.openFilePicker()
	case "o":
		// only treat as open-media when input is empty so typing "o" still works
		if strings.TrimSpace(m.input.Value()) == "" {
			return m.openOrRecoverMedia()
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) selectChatAtCursor() (tea.Model, tea.Cmd) {
	if m.chatCursor < 0 || m.chatCursor >= len(m.chats) {
		return m, nil
	}
	id := m.chats[m.chatCursor].ID
	m.selectedID = id
	m.messages = nil
	m.loadingMsgs = true
	m.msgVP.SetYOffset(0)
	m.msgVP.SetContent(m.theme.muted.Render("Cargando mensajes…"))
	m.focus = focusInput
	m.applyFocus()
	_ = m.store.ClearUnread(context.Background(), id)
	return m, tea.Batch(loadMessagesCmd(m.store, id), loadChatsCmd(m.store, m.chatFilter), textinput.Blink)
}

func (m *Model) upsertLocalMessage(msg store.Message) {
	for i := range m.messages {
		if m.messages[i].ID == msg.ID {
			m.messages[i] = msg
			return
		}
	}
	m.messages = append(m.messages, msg)
}

func (m *Model) syncCursorToSelected() {
	if m.selectedID == "" {
		return
	}
	for i, c := range m.chats {
		if c.ID == m.selectedID {
			m.chatCursor = i
			m.ensureSidebarVisible()
			return
		}
	}
}

func (m *Model) applyFocus() {
	m.input.Blur()
	m.phoneInput.Blur()
	if m.focus == focusInput {
		m.input.Focus()
	}
}

func (m *Model) relayout() {
	sidebarW, contentW, bodyH := m.layoutSizes()
	_ = sidebarW
	// contentW includes border; padding(0,1) + border eat 4 cols; head/sep/input eat rows.
	m.msgVP.Width = max(20, contentW-4)
	m.msgVP.Height = max(3, bodyH-5)
	m.input.Width = max(10, contentW-8)
	m.refreshViewport(false)
}

func (m *Model) layoutSizes() (sidebarW, contentW, bodyH int) {
	w := max(m.width, 60)
	h := max(m.height, 16)
	sidebarW = min(34, w/3)
	if sidebarW < 22 {
		sidebarW = 22
	}
	contentW = w - sidebarW - 3
	bodyH = h - 5
	return
}

func (m *Model) sidebarVisibleChats() int {
	_, _, bodyH := m.layoutSizes()
	// Inside bordered box: header rows + chat rows must fit in bodyH.
	avail := max(1, bodyH-2 /*borders*/ -sidebarHeaderRows)
	return max(1, avail/sidebarChatRows)
}

func (m *Model) ensureSidebarVisible() {
	vis := m.sidebarVisibleChats()
	if m.chatCursor < m.sidebarOff {
		m.sidebarOff = m.chatCursor
	}
	if m.chatCursor >= m.sidebarOff+vis {
		m.sidebarOff = m.chatCursor - vis + 1
	}
	if m.sidebarOff < 0 {
		m.sidebarOff = 0
	}
}

func (m *Model) refreshViewport(stickBottom bool) {
	atBottom := m.msgVP.AtBottom()
	m.msgVP.SetContent(m.renderMessages())
	if stickBottom || atBottom {
		m.msgVP.GotoBottom()
	}
	// Clamp if content shrank (deleted stubs / chat switch).
	if m.msgVP.PastBottom() {
		m.msgVP.GotoBottom()
	}
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.modal == modalConfirmDelete {
		return m.viewConfirmDelete()
	}
	if m.modal == modalHelp {
		return m.viewHelpModal()
	}
	if m.modal == modalRetention {
		return m.viewRetentionModal()
	}
	if m.modal == modalGiphyKey {
		return m.viewGiphyKeyModal()
	}
	if m.modal == modalError {
		return m.viewErrorModal()
	}
	if m.pickingEmoji {
		return m.viewEmojiPicker()
	}
	if m.addingContact {
		return m.viewAddContact()
	}
	if m.searching {
		return m.viewSearch()
	}
	if m.pickingFile {
		return m.viewFilePicker()
	}
	if m.showLogin && m.state != app.StateConnected && m.state != app.StateReconnecting {
		return m.viewLogin()
	}
	return m.viewMain()
}

func (m Model) viewHelpModal() string {
	body := `Atajos WhatsTUI

  Tab / Shift+Tab     Cambiar panel
  Ctrl+H / Ctrl+L     Chats / Input
  1 2 3 4 5           Todos / Favoritos / Grupos / Novedades / Archivados
  e                   Archivar / desarchivar chat
  f / *               Favorito (pin)
  m                   Mensajes temporales (Off→24h→7d→90d) · 1:1 y grupos
  /  o  Ctrl+F        Buscar chats, mensajes y contactos
  a                   Agregar contacto nuevo (teléfono + verificar WA)
  x                   Eliminar chat local (sidebar)
  [ / ]               Seleccionar mensaje (texto o media) · r reacciona · o/d si hay adjunto
  R                   Retención local (modal: presets + personalizado)
  Ctrl+G              Giphy API key (validar y guardar en config)
  Ctrl+E              Panel emoji / GIF (Giphy si hay API key; si no, archivo .gif)
  r                   Reaccionar al mensaje seleccionado ([ ])
  Ctrl+O              Adjuntar archivo
  o / d               Abrir / descargar media del mensaje seleccionado
  v                   Nota de voz (v otra vez = enviar · Esc cancelar)
  t                   Ciclar tema
  g                   Pronombre Él/Ella
  ?                   Esta ayuda
  Esc                 Cerrar modal / búsqueda / picker / cancelar voz
  q                   Salir

Novedades (4): comunidades a las que te uniste (no aparecen en Todos/Grupos).
Llamadas: fondo amarillo = entrante · rojo claro = perdida/rechazada
(no se pueden contestar desde la TUI).
Ticks: ✓ enviado · ✓✓ entregado · ✓✓ azul = leído (si el otro tiene
confirmación de lectura activada en WhatsApp).
Notificaciones de escritorio + sonido al llegar un mensaje en otro chat.
Emoji: Ctrl+E inserta; Tab → GIF busca (Giphy) o archivo .gif.
Giphy key: Ctrl+G para pegar/validar/guardar (o dejar vacía).
Reacciones: [ ] elige el mensaje (también texto) y pulsá r.

Mouse: click en chat, scroll en lista/mensajes, click en input.`
	box := m.theme.box.Width(max(48, min(m.width-4, 72)))
	return m.theme.title.Render("Ayuda") + "\n\n" + box.Render(body) + "\n\n" +
		m.theme.help.Render("Esc / Enter cerrar")
}

func (m Model) viewErrorModal() string {
	msg := m.errMsg
	if msg == "" {
		msg = "Error desconocido"
	}
	box := m.theme.box.Width(max(40, min(m.width-4, 64))).
		BorderForeground(lipgloss.Color("196"))
	return m.theme.statusErr.Render("Error") + "\n\n" + box.Render(msg) + "\n\n" +
		m.theme.help.Render("Esc / Enter cerrar")
}

func (m Model) viewSearch() string {
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Buscar"))
	b.WriteString(m.theme.muted.Render("  Enter abrir · Esc cancelar · ↑↓ navegar · a nuevo contacto"))
	b.WriteString("\n\n")
	b.WriteString(m.searchInput.View())
	b.WriteString("\n\n")
	if len(m.searchHits) == 0 {
		q := strings.TrimSpace(m.searchInput.Value())
		if q == "" {
			b.WriteString(m.theme.muted.Render("Chats, mensajes o contactos de la agenda…"))
		} else {
			b.WriteString(m.theme.muted.Render("Sin resultados — probá «a» para agregar por teléfono"))
		}
		return b.String()
	}
	for i, h := range m.searchHits {
		var line string
		switch h.Kind {
		case "contact":
			line = fmt.Sprintf("› %s  %s", h.ChatName, truncate(h.Snippet, 24))
		case "chat":
			line = fmt.Sprintf("· %s — %s", h.ChatName, truncate(h.Snippet, 50))
		default:
			line = fmt.Sprintf("· %s — %s", h.ChatName, truncate(h.Snippet, 50))
		}
		if i == m.searchCursor {
			b.WriteString(m.theme.sidebarSel.Width(max(40, m.width-4)).Render("▸ " + line))
		} else {
			b.WriteString(m.theme.sidebar.Render("  " + line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) viewFilePicker() string {
	var b strings.Builder
	b.WriteString(m.theme.title.Render("Adjuntar archivo"))
	b.WriteString(m.theme.muted.Render("  Enter elegir · Esc cancelar · ← carpeta arriba"))
	b.WriteString("\n\n")
	b.WriteString(m.filePicker.View())
	if m.selectedID != "" {
		b.WriteString("\n")
		b.WriteString(m.theme.muted.Render("Chat: " + m.selectedID))
		cap := strings.TrimSpace(m.input.Value())
		if cap != "" {
			b.WriteString(m.theme.muted.Render(" · Caption: " + cap))
		}
	}
	return b.String()
}

func (m Model) viewLogin() string {
	var b strings.Builder
	title := m.theme.title.Render("WhatsTUI") + m.theme.muted.Render("  v"+version.Version)
	status := m.renderStatus()
	pad := max(1, m.width-lipgloss.Width(title)-lipgloss.Width(status)-2)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", pad), status))
	b.WriteString("\n\n")

	body := ""
	switch {
	case m.pairingMode:
		body = fmt.Sprintf("Pair with phone number\n\n%s\n\nEnter confirm · Esc cancel", m.phoneInput.View())
	case m.pairCode != "":
		body = fmt.Sprintf("Pairing code:\n\n  %s\n\nWhatsApp → Linked Devices → Link with phone number", m.pairCode)
	case m.qrArt != "":
		body = fmt.Sprintf("Scan this QR with WhatsApp → Linked Devices\n\n%s", m.qrArt)
	default:
		body = "Preparing secure link…\n\n(UI stays responsive while the engine connects)"
	}
	box := m.theme.box.Width(max(40, m.width-4))
	b.WriteString(box.Render(body))
	b.WriteString("\n\n")
	b.WriteString(m.theme.help.Render("p Pairing code   q Quit"))
	if m.errMsg != "" {
		b.WriteString("\n" + m.theme.statusErr.Render("⚠ "+m.errMsg))
	}
	return b.String()
}

func (m Model) viewMain() string {
	sidebarW, contentW, bodyH := m.layoutSizes()
	header := m.viewHeader()
	sidebar := m.viewSidebar(sidebarW, bodyH)
	content := m.viewContent(contentW, bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", content)
	help := m.theme.help.Render("? ayuda · Ctrl+E emoji · r reaccionar · e archivar · [ ] msg · v voz · / buscar · q salir")
	footer := help
	if m.uploadNote != "" {
		footer = m.theme.statusWait.Render(m.uploadNote) + "  " + footer
	}
	if m.syncNote != "" {
		footer = m.theme.muted.Render(m.syncNote) + "  " + footer
	}
	if m.errMsg != "" {
		footer += "\n" + m.theme.statusErr.Render("⚠ "+m.errMsg)
	}
	if m.infoMsg != "" {
		footer += "\n" + m.theme.muted.Render(m.infoMsg)
	}
	return header + "\n" + body + "\n" + footer
}

func (m Model) viewHeader() string {
	title := m.theme.title.Render("WhatsTUI")
	status := m.renderStatus()
	pad := max(1, m.width-lipgloss.Width(title)-lipgloss.Width(status)-2)
	return lipgloss.JoinHorizontal(lipgloss.Top, title, strings.Repeat(" ", pad), status)
}

func (m Model) viewSidebar(width, height int) string {
	var b strings.Builder
	b.WriteString(filterBar(m.chatFilter, width, m.theme))
	b.WriteString("\n")
	title := "Chats"
	if m.focus == focusSidebar {
		title = "▸ Chats"
	}
	b.WriteString(m.theme.header.Render(title))
	b.WriteString("\n")
	vis := m.sidebarVisibleChats()
	end := min(len(m.chats), m.sidebarOff+vis)
	innerW := max(8, width-2) // account for border when truncating
	for i := m.sidebarOff; i < end; i++ {
		c := m.chats[i]
		name := c.Name
		if name == "" {
			name = c.ID
		}
		mark := " "
		if c.IsPinned {
			mark = "★"
		}
		line := fmt.Sprintf("%s%s", mark, truncate(name, innerW-6))
		if c.UnreadCount > 0 {
			line += fmt.Sprintf(" (%d)", c.UnreadCount)
		}
		line = truncate(line, innerW-2)
		last := c.LastMessage
		if last == "Unsupported message" {
			last = ""
		}
		preview := truncate(last, innerW-4)
		block := line + "\n " + m.theme.muted.Render(preview)
		if i == m.chatCursor {
			b.WriteString(m.theme.sidebarSel.Width(innerW).MaxHeight(sidebarChatRows).Render(block))
		} else {
			b.WriteString(m.theme.sidebar.Width(innerW).MaxHeight(sidebarChatRows).Render(block))
		}
		b.WriteString("\n")
	}
	if len(m.chats) == 0 {
		b.WriteString(m.theme.muted.Render("  Sin chats en este filtro.\n  1–5 para cambiar."))
	}
	style := lipgloss.NewStyle().Width(width).Height(height).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238"))
	if m.focus == focusSidebar {
		style = style.BorderForeground(lipgloss.Color("86"))
	}
	return style.Render(b.String())
}

func (m Model) viewContent(width, height int) string {
	name := "Select a conversation"
	if m.selectedID != "" {
		name = m.selectedID
		for _, c := range m.chats {
			if c.ID == m.selectedID {
				if c.Name != "" {
					name = c.Name
				}
				break
			}
		}
	}
	head := m.theme.header.Render(name)
	msgs := m.msgVP.View()
	inputFocus := ""
	if m.focus == focusInput {
		inputFocus = "▸ "
	}
	inputLine := inputFocus + m.input.View()
	inner := head + "\n" + strings.Repeat("─", max(10, width-4)) + "\n" + msgs + "\n" + inputLine
	style := lipgloss.NewStyle().Width(width).Height(height).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	if m.focus == focusMessages || m.focus == focusInput {
		style = style.BorderForeground(lipgloss.Color("86"))
	}
	return style.Render(inner)
}

func (m Model) renderMessages() string {
	if m.selectedID == "" {
		return m.theme.muted.Render("Select a conversation from the left.")
	}
	if len(m.messages) == 0 {
		return m.theme.muted.Render("No messages loaded yet.")
	}
	peer := m.peerLabel()
	pw := min(preview.DefaultWidth, max(20, m.msgVP.Width-4))
	var b strings.Builder
	for i, msg := range m.messages {
		ts := time.Unix(msg.Timestamp, 0).Local().Format("15:04")
		who := peer
		style := m.theme.theirs
		if msg.IsFromMe {
			who = "Tú"
			style = m.theme.mine
		}
		status := statusGlyph(msg, m.theme)
		text := msg.Text
		switch {
		case msg.Type == store.TypeCallIncoming:
			style = m.theme.callIncoming
			who = "Llamada"
			if text == "" {
				text = "Llamada entrante"
			}
		case msg.Type == store.TypeCallMissed:
			style = m.theme.callMissed
			who = "Llamada"
			if text == "" {
				text = "Llamada perdida"
			}
		case msg.MediaID != "":
			text = mediaLine(msg)
		case isMediaType(msg.Type) || looksLikeMediaText(msg.Text):
			text = msg.Text + "  [o recuperar]"
		}
		if rx := store.FormatReactions(msg.MetadataJSON); rx != "" {
			text = text + "  " + rx
		}
		cursor := "  "
		if i == m.msgCursor {
			cursor = "▸ "
		}
		line := fmt.Sprintf("%s%s  %s: %s %s", cursor, ts, who, text, status)
		b.WriteString(style.Render(line))
		b.WriteString("\n")

		if link, ok := store.ParseLinkPreview(msg.MetadataJSON); ok && (link.Title != "" || link.URL != "" || link.Desc != "") {
			card := preview.FormatLinkCard(link.Title, link.Desc, link.URL, func(s string) string {
				return m.theme.muted.Render(s)
			})
			for _, cl := range strings.Split(card, "\n") {
				b.WriteString(m.theme.accent.Render(cl))
				b.WriteString("\n")
			}
		}

		if m.cfg.ShowMediaPreviews {
			if img := m.renderMsgPreview(msg, pw, preview.DefaultHeight); img != "" {
				b.WriteString(img)
				if !strings.HasSuffix(img, "\n") {
					b.WriteString("\n")
				}
			}
		}

		if m.playingMsgID != "" && msg.ID == m.playingMsgID && m.playingMediaID != "" {
			if w := waveUnder(true, m.wavePhase, m.msgVP.Width, m.theme); w != "" {
				b.WriteString(w)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func (m Model) renderMsgPreview(msg store.Message, width, height int) string {
	path := m.previewPathFor(msg)
	if path == "" {
		return ""
	}
	proto := preview.ProtocolFromConfig(m.cfg.PreviewProtocol)
	var (
		out string
		err error
	)
	if preview.GIFFrameCount(path) > 1 {
		out, err = preview.RenderGIFFrame(path, width, height, m.gifFrame, proto)
	} else {
		out, err = preview.RenderFile(path, width, height, proto)
	}
	if err != nil || out == "" {
		return ""
	}
	return out
}

func (m Model) previewPathFor(msg store.Message) string {
	// Prefer full local media for GIF/image when ready.
	if msg.MediaID != "" {
		row, err := m.store.GetMedia(context.Background(), msg.MediaID)
		if err == nil && row.LocalPath != "" && row.DownloadState == store.MediaReady {
			return row.LocalPath
		}
	}
	if thumb := store.ParsePreviewThumb(msg.MetadataJSON); thumb != "" {
		if st, err := os.Stat(thumb); err == nil && st.Size() > 0 {
			return thumb
		}
	}
	return ""
}

func (m Model) hasAnimatedPreview() bool {
	if !m.cfg.ShowMediaPreviews || m.selectedID == "" {
		return false
	}
	for _, msg := range m.messages {
		path := m.previewPathFor(msg)
		if path != "" && preview.GIFFrameCount(path) > 1 {
			return true
		}
	}
	return false
}

func mediaLine(msg store.Message) string {
	base := msg.Text
	if base == "" {
		base = "[" + msg.Type + "]"
	}
	return base + "  [o abrir · d descargar]"
}

// peerLabel is the other party label in the transcript.
// Priority: explicit Él/Ella pronoun → address-book/chat name → "Contacto".
// WhatsApp does not send gender; set pronoun via chat settings (g key in UI).
func (m Model) peerLabel() string {
	if m.selectedID != "" {
		switch m.store.GetChatPronoun(context.Background(), m.selectedID) {
		case store.PronounEl:
			return "Él"
		case store.PronounElla:
			return "Ella"
		}
	}
	for _, c := range m.chats {
		if c.ID == m.selectedID {
			if c.IsGroup {
				return "Grupo"
			}
			if c.Name != "" {
				return c.Name
			}
			break
		}
	}
	return "Contacto"
}

func (m Model) renderStatus() string {
	label := humanStatus(m.state, m.status)
	switch m.state {
	case app.StateConnected:
		return m.theme.statusOK.Render("● " + label)
	case app.StateError, app.StateLoggedOut:
		return m.theme.statusErr.Render("● " + label)
	default:
		return m.theme.statusWait.Render("◐ " + label)
	}
}

func pronounLabel(p store.Pronoun) string {
	switch p {
	case store.PronounEl:
		return "Él"
	case store.PronounElla:
		return "Ella"
	default:
		return "nombre del contacto"
	}
}

func statusGlyph(m store.Message, th theme) string {
	if !m.IsFromMe {
		return ""
	}
	switch m.Status {
	case store.StatusSending:
		return th.muted.Render("⏳")
	case store.StatusSent:
		return th.muted.Render("✓")
	case store.StatusDelivered:
		return th.muted.Render("✓✓")
	case store.StatusRead:
		// Blue double-check — only arrives if peer has read receipts enabled.
		return th.readTick.Render("✓✓")
	case store.StatusFailed:
		return th.statusErr.Render("⚠")
	default:
		return ""
	}
}

func humanStatus(state app.ConnectionState, detail string) string {
	switch state {
	case app.StateConnected:
		return "Connected"
	case app.StateReconnecting:
		return "Reconnecting…"
	case app.StateConnecting:
		return "Connecting…"
	case app.StateQR:
		return "Waiting for QR scan"
	case app.StatePairingCode:
		return "Waiting for pairing code"
	case app.StateNeedsLogin:
		return "No session"
	case app.StateLoggedOut:
		return "Logged out"
	case app.StateError:
		if detail != "" {
			return detail
		}
		return "Error"
	default:
		if detail != "" {
			return detail
		}
		return string(state)
	}
}

func renderQR(code string) string {
	if code == "" {
		return ""
	}
	qr, err := qrcode.New(code, qrcode.Low)
	if err != nil {
		return "(failed to render QR)"
	}
	return qr.ToSmallString(false)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if n <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return ansi.Truncate(s, n, "…")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Run starts the Bubble Tea program.
func Run(bus *app.Bus, eng *engine.Engine, st *store.Store, cfg config.Config, cfgPath string) error {
	hasSession := eng.HasSession()
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.Mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(New(bus, eng, st, hasSession, cfg, cfgPath), opts...)
	_, err := p.Run()
	return err
}
