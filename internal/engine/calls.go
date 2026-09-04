package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/jidutil"
	"github.com/efolchmontiel/wsp-tui/internal/notify"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func callChatID(meta types.BasicCallMeta) string {
	if !meta.GroupJID.IsEmpty() {
		return meta.GroupJID.ToNonAD().String()
	}
	from := meta.From.ToNonAD()
	if from.IsEmpty() {
		from = meta.CallCreator.ToNonAD()
	}
	return from.String()
}

func callMessageID(callID string) string {
	if callID == "" {
		return fmt.Sprintf("call|%d", time.Now().UnixNano())
	}
	return "call|" + callID
}

func callMediaLabel(media string) string {
	media = strings.ToLower(strings.TrimSpace(media))
	switch media {
	case "video":
		return "video"
	default:
		return "voz"
	}
}

func mediaFromOfferNode(data *waBinary.Node) string {
	if data == nil {
		return "voz"
	}
	if m := data.AttrGetter().OptionalString("media"); m != "" {
		return callMediaLabel(m)
	}
	for _, child := range data.GetChildren() {
		switch child.Tag {
		case "video", "encvideo":
			return "video"
		}
	}
	return "voz"
}

func (e *Engine) handleCallOffer(evt *events.CallOffer) {
	if evt == nil {
		return
	}
	media := mediaFromOfferNode(evt.Data)
	e.upsertCallMessage(callChatID(evt.BasicCallMeta), callMessageID(evt.CallID),
		store.TypeCallIncoming, fmt.Sprintf("Llamada entrante · %s", media), evt.Timestamp)
	go notify.Desktop("WhatsTUI", fmt.Sprintf("Llamada entrante (%s)", media))
}

func (e *Engine) handleCallOfferNotice(evt *events.CallOfferNotice) {
	if evt == nil {
		return
	}
	media := callMediaLabel(evt.Media)
	e.upsertCallMessage(callChatID(evt.BasicCallMeta), callMessageID(evt.CallID),
		store.TypeCallIncoming, fmt.Sprintf("Llamada entrante · %s", media), evt.Timestamp)
	go notify.Desktop("WhatsTUI", fmt.Sprintf("Llamada entrante (%s)", media))
}

func (e *Engine) handleCallTerminate(evt *events.CallTerminate) {
	if evt == nil {
		return
	}
	reason := strings.ToLower(evt.Reason)
	chatID := callChatID(evt.BasicCallMeta)
	msgID := callMessageID(evt.CallID)
	switch reason {
	case "accepted", "connected":
		e.markCallResolved(chatID, msgID, "Llamada contestada")
		return
	}
	text := "Llamada perdida"
	if prev, err := e.store.GetMessage(context.Background(), chatID, msgID); err == nil {
		if strings.Contains(prev.Text, "video") {
			text = "Llamada perdida · video"
		} else if strings.Contains(prev.Text, "voz") {
			text = "Llamada perdida · voz"
		}
	}
	e.logger.Info("call terminated", "chat", chatID, "id", msgID, "reason", reason)
	e.upsertCallMessage(chatID, msgID, store.TypeCallMissed, text, evt.Timestamp)
}

func (e *Engine) handleCallReject(evt *events.CallReject) {
	if evt == nil {
		return
	}
	e.upsertCallMessage(callChatID(evt.BasicCallMeta), callMessageID(evt.CallID),
		store.TypeCallMissed, "Llamada rechazada", evt.Timestamp)
}

func (e *Engine) upsertCallMessage(chatID, msgID, typ, text string, ts time.Time) {
	if chatID == "" || msgID == "" {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	ctx := context.Background()
	isGroup := false
	if jid, err := types.ParseJID(chatID); err == nil {
		isGroup = jidutil.IsGroup(jid)
	}
	_ = e.store.EnsureChatExists(ctx, chatID, "", isGroup)
	sm := store.Message{
		ID:        msgID,
		ChatID:    chatID,
		Sender:    chatID,
		Timestamp: ts.Unix(),
		Text:      text,
		Type:      typ,
		Status:    store.StatusReceived,
		IsFromMe:  false,
	}
	if err := e.store.UpsertMessage(ctx, sm); err != nil {
		e.logger.Warn("upsert call message", "err", err)
		return
	}
	_ = e.store.TouchChatPreview(ctx, chatID, text, ts.Unix(), 1)
	cp := sm
	e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: chatID})
	e.bus.Publish(app.Event{Kind: app.EventChatsDirty})
}

func (e *Engine) markCallResolved(chatID, msgID, text string) {
	ctx := context.Background()
	sm := store.Message{
		ID:        msgID,
		ChatID:    chatID,
		Sender:    chatID,
		Timestamp: time.Now().Unix(),
		Text:      text,
		Type:      store.TypeOther,
		Status:    store.StatusReceived,
	}
	_ = e.store.UpsertMessage(ctx, sm)
	cp := sm
	e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: chatID})
}
