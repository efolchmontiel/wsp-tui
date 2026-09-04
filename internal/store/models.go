package store

// Chat is a conversation row for the sidebar.
type Chat struct {
	ID            string
	Name          string
	IsGroup       bool
	IsPinned      bool
	IsMuted       bool
	IsArchived    bool
	IsCommunity   bool // parent community or linked community subgroup
	UnreadCount   int
	LastMessageAt int64 // unix seconds
	LastMessage   string
	UpdatedAt     int64
}

// ChatFilter selects which chats appear in the sidebar.
type ChatFilter int

const (
	FilterAll ChatFilter = iota
	FilterFavorites
	FilterGroups
	FilterEstados   // WhatsApp Status (status@broadcast)
	FilterNovedades // communities
	FilterArchived
)

// Message is a single chat message in the app DB.
type Message struct {
	ID              string
	ChatID          string
	Sender          string
	Timestamp       int64 // unix seconds
	Text            string
	Type            string
	Status          string
	QuotedMessageID string
	MediaID         string
	IsFromMe        bool
	IsDeleted       bool
	MetadataJSON    string
}

// Message status values used by the UI.
const (
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusFailed    = "failed"
	StatusReceived  = "received"
)

// Message types (Phase 2 focuses on text; others are placeholders).
const (
	TypeText         = "text"
	TypeImage        = "image"
	TypeVideo        = "video"
	TypeAudio        = "audio"
	TypeDocument     = "document"
	TypeSticker      = "sticker"
	TypeCallIncoming = "call_incoming"
	TypeCallMissed   = "call_missed"
	TypeReaction     = "reaction"
	TypeOther        = "other"
)
