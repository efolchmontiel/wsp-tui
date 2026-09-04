package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/jidutil"
	"github.com/efolchmontiel/wsp-tui/internal/logging"
	"github.com/efolchmontiel/wsp-tui/internal/media"
	"github.com/efolchmontiel/wsp-tui/internal/msgutil"
	"github.com/efolchmontiel/wsp-tui/internal/paths"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"github.com/efolchmontiel/wsp-tui/internal/syncer"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

const (
	sqliteDialect = "sqlite"
	pairDisplay   = "WhatsTUI"
)

// Engine owns the whatsmeow client lifecycle.
type Engine struct {
	paths  paths.Paths
	logger *slog.Logger
	debug  bool
	bus    *app.Bus
	store  *store.Store
	syncer *syncer.Syncer
	media  *media.Manager

	mu        sync.Mutex
	container *sqlstore.Container
	client    *whatsmeow.Client
	started   atomic.Bool
	cancel    context.CancelFunc
	voiceRec  *media.VoiceRecorder

	// Exclusive A/V playback (mpv). Only one clip at a time.
	playMu    sync.Mutex
	playCmd   *exec.Cmd
	playID    string
	playGen   uint64

	// Incoming call ring state keyed by WhatsApp call-id.
	callMu      sync.Mutex
	activeCalls map[string]*activeCall
}

// New constructs an Engine. Call Start to connect.
func New(p paths.Paths, logger *slog.Logger, debug bool, bus *app.Bus, st *store.Store, syn *syncer.Syncer) *Engine {
	return &Engine{
		paths:  p,
		logger: logger,
		debug:  debug,
		bus:    bus,
		store:  st,
		syncer: syn,
	}
}

// Start opens the session store and begins connection / QR flow in the background.
func (e *Engine) Start(ctx context.Context) error {
	if !e.started.CompareAndSwap(false, true) {
		return fmt.Errorf("engine already started")
	}

	ctx, e.cancel = context.WithCancel(ctx)

	if err := ensurePrivateFile(e.paths.SessionDB); err != nil {
		return err
	}

	dbLog := logging.NewWALogger(e.logger, "SessionDB", e.debug)
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", e.paths.SessionDB)
	container, err := sqlstore.New(ctx, sqliteDialect, dsn, dbLog)
	if err != nil {
		return fmt.Errorf("session sqlstore: %w", err)
	}
	e.container = container

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	clientLog := logging.NewWALogger(e.logger, "Client", e.debug)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.EnableAutoReconnect = true
	client.AutoReconnectHook = func(err error) bool {
		e.publishState(app.StateReconnecting, fmt.Sprintf("reconnect attempt: %v", err))
		return true
	}
	e.client = client
	if e.syncer != nil {
		e.syncer.SetClient(client)
	}

	client.AddEventHandler(e.handleEvent)

	go e.run(ctx)
	return nil
}

func (e *Engine) run(ctx context.Context) {
	client := e.client
	if client == nil {
		return
	}

	hasSession := client.Store.ID != nil
	if !hasSession {
		e.publishState(app.StateNeedsLogin, "no session")
		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			e.publishError(fmt.Errorf("qr channel: %w", err))
			return
		}
		e.publishState(app.StateConnecting, "connecting for QR")
		if err := client.Connect(); err != nil {
			e.publishError(fmt.Errorf("connect: %w", err))
			return
		}
		go e.consumeQR(ctx, qrChan)
		return
	}

	e.publishState(app.StateConnecting, "reusing session")
	if err := client.Connect(); err != nil {
		e.publishError(fmt.Errorf("connect: %w", err))
		e.publishState(app.StateReconnecting, err.Error())
		return
	}
}

func (e *Engine) consumeQR(ctx context.Context, qrChan <-chan whatsmeow.QRChannelItem) {
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-qrChan:
			if !ok {
				return
			}
			switch item.Event {
			case "code":
				e.bus.Publish(app.Event{
					Kind:   app.EventQR,
					State:  app.StateQR,
					QRCode: item.Code,
				})
				e.publishState(app.StateQR, "scan QR")
			case "success":
				e.publishState(app.StateConnected, "paired")
				return
			case "timeout":
				e.publishError(fmt.Errorf("QR login timed out"))
				e.publishState(app.StateNeedsLogin, "qr timeout")
				return
			default:
				if item.Error != nil {
					e.publishError(item.Error)
				} else {
					e.bus.Publish(app.Event{
						Kind:    app.EventInfo,
						Message: "login event: " + item.Event,
					})
				}
			}
		}
	}
}

func (e *Engine) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Connected:
		e.publishState(app.StateConnected, "connected")
		go e.sweepStaleIncomingCalls()
		if e.syncer != nil {
			go e.syncer.RefreshAllNames(context.Background())
		}
	case *events.Disconnected:
		e.publishState(app.StateReconnecting, "disconnected")
	case *events.LoggedOut:
		e.publishState(app.StateLoggedOut, "logged out")
	case *events.PairSuccess:
		e.publishState(app.StateConnected, "pair success")
	case *events.StreamReplaced:
		e.publishError(fmt.Errorf("stream replaced by another connection"))
	case *events.KeepAliveTimeout:
		e.publishState(app.StateReconnecting, "keepalive timeout")
	case *events.KeepAliveRestored:
		e.publishState(app.StateConnected, "keepalive restored")
	case *events.ClientOutdated:
		e.publishError(fmt.Errorf("whatsmeow client outdated — update dependency"))
	case *events.HistorySync:
		if e.syncer != nil {
			e.syncer.EnqueueHistory(v)
		}
	case *events.Message:
		if e.syncer != nil {
			e.syncer.EnqueueMessage(v)
		}
	case *events.Receipt:
		if e.syncer != nil {
			e.syncer.EnqueueReceipt(v)
		}
	case *events.GroupInfo:
		if e.syncer != nil && v.Name != nil {
			go e.syncer.ApplyGroupNameUpdate(context.Background(), v)
		}
	case *events.CallOffer:
		e.handleCallOffer(v)
	case *events.CallOfferNotice:
		e.handleCallOfferNotice(v)
	case *events.CallAccept:
		e.handleCallAccept(v)
	case *events.CallTerminate:
		e.handleCallTerminate(v)
	case *events.CallReject:
		e.handleCallReject(v)
	}
}

// SendText sends a text message asynchronously after optimistic local insert.
// Returns the local message immediately; network result arrives via events.
func (e *Engine) SendText(ctx context.Context, chatID, text string) (store.Message, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return store.Message{}, fmt.Errorf("empty message")
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return store.Message{}, fmt.Errorf("chat id: %w", err)
	}

	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return store.Message{}, fmt.Errorf("not connected")
	}

	id := client.GenerateMessageID()
	now := time.Now()
	msg := store.Message{
		ID:        string(id),
		ChatID:    chatID,
		Sender:    "",
		Timestamp: now.Unix(),
		Text:      text,
		Type:      store.TypeText,
		Status:    store.StatusSending,
		IsFromMe:  true,
	}
	if client.Store.ID != nil {
		msg.Sender = client.Store.ID.ToNonAD().String()
	}
	if e.store != nil {
		_ = e.store.EnsureChatExists(ctx, chatID, "", jidutil.IsGroup(jid))
		if err := e.store.UpsertMessage(ctx, msg); err != nil {
			return store.Message{}, err
		}
		_ = e.store.TouchChatPreview(ctx, chatID, msgutil.Preview(text, 80), msg.Timestamp, 0)
	}

	cp := msg
	e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: chatID})
	e.bus.Publish(app.Event{Kind: app.EventChatsDirty})

	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_, err := client.SendMessage(sendCtx, jid, &waE2E.Message{
			Conversation: proto.String(text),
		}, whatsmeow.SendRequestExtra{ID: id})
		if err != nil {
			e.logger.Warn("send failed", "err", err, "id", id)
			if e.store != nil {
				_ = e.store.UpdateMessageStatus(context.Background(), chatID, string(id), store.StatusFailed)
			}
			e.bus.Publish(app.Event{
				Kind:       app.EventMessageStatus,
				ChatID:     chatID,
				MessageIDs: []string{string(id)},
				Status:     store.StatusFailed,
				Message:    err.Error(),
			})
			return
		}
		if e.store != nil {
			_ = e.store.UpdateMessageStatus(context.Background(), chatID, string(id), store.StatusSent)
		}
		e.bus.Publish(app.Event{
			Kind:       app.EventMessageStatus,
			ChatID:     chatID,
			MessageIDs: []string{string(id)},
			Status:     store.StatusSent,
		})
	}()

	return msg, nil
}

// RequestPairingCode asks WhatsApp for a phone pairing code.
func (e *Engine) RequestPairingCode(ctx context.Context, phone string) (string, error) {
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil {
		return "", fmt.Errorf("client not ready")
	}
	phone = strings.TrimPrefix(strings.TrimSpace(phone), "+")
	phone = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if phone == "" {
		return "", fmt.Errorf("phone number required")
	}

	code, err := client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, pairDisplay)
	if err != nil {
		return "", err
	}
	e.bus.Publish(app.Event{
		Kind:     app.EventPairCode,
		State:    app.StatePairingCode,
		PairCode: code,
	})
	e.publishState(app.StatePairingCode, "enter code on phone")
	return code, nil
}

// HasSession reports whether a stored device ID exists.
func (e *Engine) HasSession() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.client != nil && e.client.Store != nil && e.client.Store.ID != nil
}

// Client exposes the underlying client for later phases.
func (e *Engine) Client() *whatsmeow.Client {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.client
}

// Logout disconnects and removes the linked device session remotely when possible.
func (e *Engine) Logout(ctx context.Context) error {
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil {
		return e.resetLocalSession()
	}
	if client.IsLoggedIn() {
		if err := client.Logout(ctx); err != nil {
			e.logger.Warn("logout remote failed; wiping local session", "err", err)
		}
	}
	client.Disconnect()
	return e.resetLocalSession()
}

// Reset wipes local session.
func (e *Engine) Reset() error {
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client != nil {
		client.Disconnect()
	}
	return e.resetLocalSession()
}

func (e *Engine) resetLocalSession() error {
	if e.container != nil {
		_ = e.container.Close()
		e.container = nil
	}
	e.client = nil
	if err := os.Remove(e.paths.SessionDB); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session db: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(e.paths.SessionDB + suffix)
	}
	return nil
}

// Close stops the engine.
func (e *Engine) Close() {
	e.CancelVoiceRecord()
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client != nil {
		e.client.Disconnect()
	}
	if e.container != nil {
		_ = e.container.Close()
	}
}

func (e *Engine) publishState(state app.ConnectionState, msg string) {
	e.bus.Publish(app.Event{
		Kind:    app.EventStateChanged,
		State:   state,
		Message: msg,
	})
}

func (e *Engine) publishError(err error) {
	e.logger.Error("engine error", "err", err)
	e.bus.Publish(app.Event{
		Kind:    app.EventError,
		State:   app.StateError,
		Err:     err,
		Message: err.Error(),
	})
}

func ensurePrivateFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("ensure session db: %w", err)
	}
	return f.Close()
}
