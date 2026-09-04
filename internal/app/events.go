package app

import (
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

// ConnectionState is the high-level connection status shown in the UI.
type ConnectionState string

const (
	StateStarting     ConnectionState = "starting"
	StateNeedsLogin   ConnectionState = "needs_login"
	StateQR           ConnectionState = "qr"
	StatePairingCode  ConnectionState = "pairing_code"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateReconnecting ConnectionState = "reconnecting"
	StateLoggedOut    ConnectionState = "logged_out"
	StateError        ConnectionState = "error"
)

// Event is a UI-safe notification published by the engine/syncer.
type Event struct {
	Kind     EventKind
	State    ConnectionState
	QRCode   string
	PairCode string
	Message  string
	Err      error
	At       time.Time

	Chat       *store.Chat
	Msg        *store.Message
	ChatID     string
	MessageIDs []string
	Status     string
	SyncNote   string
	MediaID    string
	LocalPath  string
	Progress   int  // 0-100 for uploads
	IsReaction bool // reaction applied to an existing message — no unread/preview/notify
}

// EventKind classifies engine → UI messages.
type EventKind string

const (
	EventStateChanged     EventKind = "state"
	EventQR               EventKind = "qr"
	EventPairCode         EventKind = "pair_code"
	EventInfo             EventKind = "info"
	EventError            EventKind = "error"
	EventChatUpserted     EventKind = "chat_upserted"
	EventChatsDirty       EventKind = "chats_dirty"
	EventMessageUpserted  EventKind = "message_upserted"
	EventMessageStatus    EventKind = "message_status"
	EventSyncProgress     EventKind = "sync_progress"
	EventMediaUpdated     EventKind = "media_updated"
	EventUploadProgress   EventKind = "upload_progress"
	EventMediaPlaying     EventKind = "media_playing"
	EventMediaStopped     EventKind = "media_stopped"
)

// Bus is a non-blocking fan-out channel for UI consumers.
type Bus struct {
	ch chan Event
}

// NewBus creates a buffered event bus. Overflow drops events rather than
// blocking WhatsApp handlers.
func NewBus(size int) *Bus {
	if size < 16 {
		size = 16
	}
	return &Bus{ch: make(chan Event, size)}
}

// Publish attempts a non-blocking send.
func (b *Bus) Publish(evt Event) {
	if evt.At.IsZero() {
		evt.At = time.Now()
	}
	select {
	case b.ch <- evt:
	default:
		// Prefer keeping the bus moving: try to drop one stale event then retry once.
		select {
		case <-b.ch:
		default:
		}
		select {
		case b.ch <- evt:
		default:
		}
	}
}

// Events returns the receive-only channel.
func (b *Bus) Events() <-chan Event { return b.ch }
