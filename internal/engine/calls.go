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

// If CallTerminate never arrives (common on companion devices), flip to missed.
const callMissTimeout = 55 * time.Second

type activeCall struct {
	chatID string
	msgID  string
	media  string
	cancel context.CancelFunc
}

func callChatCandidates(meta types.BasicCallMeta) []string {
	out := make([]string, 0, 4)
	add := func(j types.JID) {
		if j.IsEmpty() {
			return
		}
		s := j.ToNonAD().String()
		if s == "" {
			return
		}
		for _, existing := range out {
			if existing == s {
				return
			}
		}
		out = append(out, s)
	}
	if !meta.GroupJID.IsEmpty() {
		add(meta.GroupJID)
		return out
	}
	add(meta.From)
	add(meta.CallCreator)
	add(meta.CallCreatorAlt)
	return out
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

func (e *Engine) resolveCallChatID(meta types.BasicCallMeta) string {
	cands := callChatCandidates(meta)
	if len(cands) == 0 {
		return ""
	}
	if e.store == nil {
		return cands[0]
	}
	return e.store.PreferExistingChatID(context.Background(), cands...)
}

func (e *Engine) handleCallOffer(evt *events.CallOffer) {
	if evt == nil {
		return
	}
	media := mediaFromOfferNode(evt.Data)
	chatID := e.resolveCallChatID(evt.BasicCallMeta)
	msgID := callMessageID(evt.CallID)
	e.upsertCallMessage(chatID, msgID, store.TypeCallIncoming,
		fmt.Sprintf("Llamada entrante · %s", media), evt.Timestamp)
	e.trackIncomingCall(evt.CallID, chatID, msgID, media)
	go notify.Desktop("WhatsTUI", fmt.Sprintf("Llamada entrante (%s)", media))
}

func (e *Engine) handleCallOfferNotice(evt *events.CallOfferNotice) {
	if evt == nil {
		return
	}
	media := callMediaLabel(evt.Media)
	chatID := e.resolveCallChatID(evt.BasicCallMeta)
	msgID := callMessageID(evt.CallID)
	e.upsertCallMessage(chatID, msgID, store.TypeCallIncoming,
		fmt.Sprintf("Llamada entrante · %s", media), evt.Timestamp)
	e.trackIncomingCall(evt.CallID, chatID, msgID, media)
	go notify.Desktop("WhatsTUI", fmt.Sprintf("Llamada entrante (%s)", media))
}

func (e *Engine) handleCallTerminate(evt *events.CallTerminate) {
	if evt == nil {
		return
	}
	reason := strings.ToLower(evt.Reason)
	chatID, msgID, media := e.lookupActiveCall(evt.CallID, evt.BasicCallMeta)
	e.untrackCall(evt.CallID)

	switch reason {
	case "accepted", "connected":
		e.markCallResolved(chatID, msgID, "Llamada contestada")
		return
	}
	text := missedCallText(media, chatID, msgID, e)
	e.logger.Info("call terminated", "chat", chatID, "id", msgID, "reason", reason)
	e.upsertCallMessage(chatID, msgID, store.TypeCallMissed, text, evt.Timestamp)
}

func (e *Engine) handleCallReject(evt *events.CallReject) {
	if evt == nil {
		return
	}
	chatID, msgID, media := e.lookupActiveCall(evt.CallID, evt.BasicCallMeta)
	e.untrackCall(evt.CallID)
	text := "Llamada rechazada"
	if media == "video" {
		text = "Llamada rechazada · video"
	} else if media == "voz" {
		text = "Llamada rechazada · voz"
	}
	e.upsertCallMessage(chatID, msgID, store.TypeCallMissed, text, evt.Timestamp)
}

func (e *Engine) handleCallAccept(evt *events.CallAccept) {
	if evt == nil {
		return
	}
	chatID, msgID, _ := e.lookupActiveCall(evt.CallID, evt.BasicCallMeta)
	e.untrackCall(evt.CallID)
	e.markCallResolved(chatID, msgID, "Llamada contestada")
}

func missedCallText(media, chatID, msgID string, e *Engine) string {
	if media == "video" {
		return "Llamada perdida · video"
	}
	if media == "voz" {
		return "Llamada perdida · voz"
	}
	if e != nil && e.store != nil && chatID != "" && msgID != "" {
		if prev, err := e.store.GetMessage(context.Background(), chatID, msgID); err == nil {
			if strings.Contains(prev.Text, "video") {
				return "Llamada perdida · video"
			}
			if strings.Contains(prev.Text, "voz") {
				return "Llamada perdida · voz"
			}
		}
	}
	return "Llamada perdida"
}

func (e *Engine) lookupActiveCall(callID string, meta types.BasicCallMeta) (chatID, msgID, media string) {
	e.callMu.Lock()
	if ac, ok := e.activeCalls[callID]; ok && ac != nil {
		chatID, msgID, media = ac.chatID, ac.msgID, ac.media
	}
	e.callMu.Unlock()
	if chatID == "" {
		chatID = e.resolveCallChatID(meta)
	}
	if msgID == "" {
		msgID = callMessageID(callID)
	}
	return chatID, msgID, media
}

func (e *Engine) trackIncomingCall(callID, chatID, msgID, media string) {
	if callID == "" || chatID == "" {
		return
	}
	e.callMu.Lock()
	defer e.callMu.Unlock()
	if e.activeCalls == nil {
		e.activeCalls = make(map[string]*activeCall)
	}
	if prev, ok := e.activeCalls[callID]; ok && prev != nil && prev.cancel != nil {
		prev.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.activeCalls[callID] = &activeCall{
		chatID: chatID,
		msgID:  msgID,
		media:  media,
		cancel: cancel,
	}
	go func(id string) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(callMissTimeout):
			e.missCallByTimeout(id)
		}
	}(callID)
}

func (e *Engine) untrackCall(callID string) {
	if callID == "" {
		return
	}
	e.callMu.Lock()
	defer e.callMu.Unlock()
	if ac, ok := e.activeCalls[callID]; ok && ac != nil {
		if ac.cancel != nil {
			ac.cancel()
		}
		delete(e.activeCalls, callID)
	}
}

func (e *Engine) missCallByTimeout(callID string) {
	e.callMu.Lock()
	ac, ok := e.activeCalls[callID]
	if !ok || ac == nil {
		e.callMu.Unlock()
		return
	}
	chatID, msgID, media := ac.chatID, ac.msgID, ac.media
	delete(e.activeCalls, callID)
	if ac.cancel != nil {
		ac.cancel()
	}
	e.callMu.Unlock()

	if e.store != nil {
		prev, err := e.store.GetMessage(context.Background(), chatID, msgID)
		if err != nil || prev.Type != store.TypeCallIncoming {
			return
		}
	}
	e.logger.Info("call missed by timeout", "chat", chatID, "id", msgID)
	e.upsertCallMessage(chatID, msgID, store.TypeCallMissed,
		missedCallText(media, chatID, msgID, e), time.Now())
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

	// Keep original timestamp so terminate/timeout don't reorder the timeline.
	unixTS := ts.Unix()
	if existing, err := e.store.GetMessage(ctx, chatID, msgID); err == nil && existing.Timestamp > 0 {
		unixTS = existing.Timestamp
	}

	sm := store.Message{
		ID:        msgID,
		ChatID:    chatID,
		Sender:    chatID,
		Timestamp: unixTS,
		Text:      text,
		Type:      typ,
		Status:    store.StatusReceived,
		IsFromMe:  false,
	}
	if err := e.store.UpsertMessage(ctx, sm); err != nil {
		e.logger.Warn("upsert call message", "err", err)
		return
	}
	_ = e.store.TouchChatPreview(ctx, chatID, text, unixTS, 1)
	cp := sm
	e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: chatID})
	e.bus.Publish(app.Event{Kind: app.EventChatsDirty})
}

func (e *Engine) markCallResolved(chatID, msgID, text string) {
	if chatID == "" || msgID == "" {
		return
	}
	ctx := context.Background()
	unixTS := time.Now().Unix()
	if existing, err := e.store.GetMessage(ctx, chatID, msgID); err == nil && existing.Timestamp > 0 {
		unixTS = existing.Timestamp
	}
	sm := store.Message{
		ID:        msgID,
		ChatID:    chatID,
		Sender:    chatID,
		Timestamp: unixTS,
		Text:      text,
		Type:      store.TypeOther,
		Status:    store.StatusReceived,
	}
	_ = e.store.UpsertMessage(ctx, sm)
	cp := sm
	e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: chatID})
}

func (e *Engine) sweepStaleIncomingCalls() {
	if e.store == nil {
		return
	}
	ctx := context.Background()
	cutoff := time.Now().Add(-callMissTimeout).Unix()
	resolved, err := e.store.SweepStaleIncomingCalls(ctx, cutoff)
	if err != nil {
		e.logger.Warn("sweep stale calls", "err", err)
		return
	}
	for i := range resolved {
		r := resolved[i]
		e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &r, ChatID: r.ChatID})
	}
	if len(resolved) > 0 {
		e.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	}
}
