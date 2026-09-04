package syncer

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/contacts"
	"github.com/efolchmontiel/wsp-tui/internal/jidutil"
	"github.com/efolchmontiel/wsp-tui/internal/media"
	"github.com/efolchmontiel/wsp-tui/internal/msgutil"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Syncer persists WhatsApp events into the app DB off the UI thread.
type Syncer struct {
	store  *store.Store
	bus    *app.Bus
	logger *slog.Logger
	media  *media.Manager

	mu     sync.Mutex
	client *whatsmeow.Client

	jobs   chan job
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type jobKind int

const (
	jobHistory jobKind = iota
	jobMessage
	jobReceipt
)

type job struct {
	kind    jobKind
	history *events.HistorySync
	message *events.Message
	receipt *events.Receipt
}

// New creates a syncer. Call Start after wiring the client.
func New(st *store.Store, bus *app.Bus, logger *slog.Logger) *Syncer {
	return &Syncer{
		store:  st,
		bus:    bus,
		logger: logger,
		jobs:   make(chan job, 256),
	}
}

// SetMedia wires the media manager for auto-download of small images.
func (s *Syncer) SetMedia(m *media.Manager) {
	s.media = m
}

// SetClient updates the whatsmeow client used for ParseWebMessage / contacts.
func (s *Syncer) SetClient(c *whatsmeow.Client) {
	s.mu.Lock()
	s.client = c
	s.mu.Unlock()
}

func (s *Syncer) clientOrNil() *whatsmeow.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

// Start launches the background worker.
func (s *Syncer) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.wg.Add(1)
	go s.loop(ctx)
}

// Stop stops the worker.
func (s *Syncer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// EnqueueHistory queues a history sync blob (never blocks the WhatsApp handler long).
func (s *Syncer) EnqueueHistory(evt *events.HistorySync) {
	s.tryEnqueue(job{kind: jobHistory, history: evt})
}

// EnqueueMessage queues a live message event.
func (s *Syncer) EnqueueMessage(evt *events.Message) {
	s.tryEnqueue(job{kind: jobMessage, message: evt})
}

// EnqueueReceipt queues delivery/read receipts.
func (s *Syncer) EnqueueReceipt(evt *events.Receipt) {
	s.tryEnqueue(job{kind: jobReceipt, receipt: evt})
}

func (s *Syncer) tryEnqueue(j job) {
	select {
	case s.jobs <- j:
	default:
		s.logger.Warn("syncer queue full; dropping job", "kind", j.kind)
	}
}

func (s *Syncer) loop(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-s.jobs:
			switch j.kind {
			case jobHistory:
				s.handleHistory(ctx, j.history)
			case jobMessage:
				s.handleMessage(ctx, j.message)
			case jobReceipt:
				s.handleReceipt(ctx, j.receipt)
			}
		}
	}
}

func (s *Syncer) handleHistory(ctx context.Context, evt *events.HistorySync) {
	if evt == nil || evt.Data == nil {
		return
	}
	convs := evt.Data.GetConversations()
	s.bus.Publish(app.Event{
		Kind:     app.EventSyncProgress,
		SyncNote: "history sync",
		Message:  "Applying history…",
	})

	dirty := false
	for _, conv := range convs {
		if ctx.Err() != nil {
			return
		}
		if err := s.persistConversation(ctx, conv); err != nil {
			s.logger.Warn("persist conversation", "err", err)
			continue
		}
		dirty = true
	}
	if dirty {
		s.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	}
	s.bus.Publish(app.Event{
		Kind:     app.EventSyncProgress,
		SyncNote: "history sync",
		Message:  "History applied",
	})
}

func (s *Syncer) persistConversation(ctx context.Context, conv *waHistorySync.Conversation) error {
	chatID := conv.GetID()
	if chatID == "" {
		return nil
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return err
	}

	client := s.clientOrNil()
	fallback := conv.GetName()
	if fallback == "" {
		fallback = conv.GetDisplayName()
	}

	var name string
	isGroup := jidutil.IsGroup(jid)
	switch {
	case jidutil.IsStatusBroadcast(jid):
		name = "Estados"
		isGroup = false
	case isGroup:
		// Prefer history subject; never trust a leftover push-name. JoinedGroups refresh fixes empties.
		name = fallback
		if name == "" {
			name = "Grupo"
		}
	default:
		resolved := contacts.ResolveDisplayName(ctx, client, jid, fallback)
		name = resolved.Name
	}

	lastTS := int64(conv.GetLastMsgTimestamp())
	if lastTS == 0 {
		lastTS = int64(conv.GetConversationTimestamp())
	}

	preview := ""
	msgs := conv.GetMessages()
	// Persist messages; keep UI responsive by batching notifications.
	for i, hm := range msgs {
		if hm == nil || hm.GetMessage() == nil || client == nil {
			continue
		}
		parsed, err := client.ParseWebMessage(jid, hm.GetMessage())
		if err != nil {
			s.logger.Debug("parse history msg", "err", err)
			continue
		}
		sm, err := s.storeParsed(ctx, parsed)
		if err != nil {
			s.logger.Debug("store history msg", "err", err)
			continue
		}
		if sm.Timestamp >= lastTS {
			lastTS = sm.Timestamp
			preview = msgutil.Preview(sm.Text, 80)
		}
		// Avoid flooding the bus during huge syncs.
		if i == len(msgs)-1 {
			cp := sm
			s.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: sm.ChatID})
		}
	}

	chat := store.Chat{
		ID:            chatID,
		Name:          name,
		IsGroup:       isGroup,
		IsPinned:      conv.GetPinned() > 0 && !jidutil.IsStatusBroadcast(jid),
		IsMuted:       conv.GetMuteEndTime() > uint64(time.Now().UnixMilli()),
		UnreadCount:   int(conv.GetUnreadCount()),
		LastMessageAt: lastTS,
		LastMessage:   preview,
		UpdatedAt:     time.Now().Unix(),
	}
	if err := s.store.UpsertChat(ctx, chat); err != nil {
		return err
	}
	cp := chat
	s.bus.Publish(app.Event{Kind: app.EventChatUpserted, Chat: &cp})
	return nil
}

func (s *Syncer) handleMessage(ctx context.Context, evt *events.Message) {
	if evt == nil {
		return
	}
	sm, err := s.storeParsed(ctx, evt)
	if err != nil {
		s.logger.Warn("store message", "err", err)
		return
	}

	unread := 0
	if !sm.IsFromMe && !jidutil.IsStatusBroadcast(evt.Info.Chat) {
		unread = 1
	}
	isGroup := jidutil.IsGroup(evt.Info.Chat)
	switch {
	case jidutil.IsStatusBroadcast(evt.Info.Chat):
		_ = s.store.EnsureChatExists(ctx, sm.ChatID, "Estados", false)
		_ = s.store.SetChatName(ctx, sm.ChatID, "Estados", false)
	case isGroup:
		// Do NOT use sender push names as the group title (that produced "e folchm").
		_ = s.store.EnsureChatExists(ctx, sm.ChatID, "", true)
	default:
		resolved := contacts.ResolveDisplayName(ctx, s.clientOrNil(), evt.Info.Chat, "")
		_ = s.store.EnsureChatExists(ctx, sm.ChatID, resolved.Name, false)
		if resolved.FromAddressBook && resolved.Name != "" {
			_ = s.store.SetChatName(ctx, sm.ChatID, resolved.Name, false)
		} else if resolved.Name != "" {
			_ = s.store.SetChatNameIfEmpty(ctx, sm.ChatID, resolved.Name, false)
		}
	}
	_ = s.store.TouchChatPreview(ctx, sm.ChatID, msgutil.Preview(sm.Text, 80), sm.Timestamp, unread)

	cp := sm
	s.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: sm.ChatID})
	// Sidebar patch is enough for live traffic; full reload is coalesced on ChatsDirty only.
}

func (s *Syncer) handleReceipt(ctx context.Context, evt *events.Receipt) {
	if evt == nil || len(evt.MessageIDs) == 0 {
		return
	}
	status := ""
	switch evt.Type {
	case types.ReceiptTypeDelivered:
		status = store.StatusDelivered
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		status = store.StatusRead
	default:
		return
	}
	chatID := evt.Chat.String()
	ids := make([]string, len(evt.MessageIDs))
	for i, id := range evt.MessageIDs {
		ids[i] = string(id)
	}
	if err := s.store.UpdateMessagesStatus(ctx, chatID, ids, status); err != nil {
		s.logger.Warn("update receipt status", "err", err)
		return
	}
	s.bus.Publish(app.Event{
		Kind:       app.EventMessageStatus,
		ChatID:     chatID,
		MessageIDs: ids,
		Status:     status,
	})
}

func (s *Syncer) storeParsed(ctx context.Context, evt *events.Message) (store.Message, error) {
	typ, text := msgutil.ClassifyType(evt.Message)
	if text == "" && typ == store.TypeOther {
		text = "Unsupported message"
	}
	status := store.StatusReceived
	if evt.Info.IsFromMe {
		status = store.StatusSent
	}
	sm := store.Message{
		ID:        string(evt.Info.ID),
		ChatID:    evt.Info.Chat.String(),
		Sender:    evt.Info.Sender.String(),
		Timestamp: evt.Info.Timestamp.Unix(),
		Text:      text,
		Type:      typ,
		Status:    status,
		IsFromMe:  evt.Info.IsFromMe,
	}

	ref, hasMedia := media.ExtractRef(evt.Message)
	if hasMedia {
		mediaID := sm.ChatID + "|" + sm.ID
		sm.MediaID = mediaID
		sm.MetadataJSON = ref.EncodeJSON()
		if text == "" || text == "imagen" || text == "video" || text == "audio" || text == "voz" ||
			text == "› imagen" || text == "› video" || text == "› audio" || text == "› voz" ||
			text == "🎤 Voice message" || text == "documento" || text == "sticker" ||
			text == "🖼 Image" || text == "🎥 Video" || text == "🎵 Audio" || text == "📄 Document" || text == "🖼 Sticker" {
			size := ""
			if ref.FileLength > 0 {
				size = " · " + media.FormatSize(ref.FileLength)
			}
			sm.Text = text + size
		}
	}

	if err := s.store.UpsertMessage(ctx, sm); err != nil {
		return store.Message{}, err
	}

	if hasMedia {
		_ = s.store.UpsertMedia(ctx, store.MediaRow{
			ID:            sm.MediaID,
			ChatID:        sm.ChatID,
			MessageID:     sm.ID,
			MimeType:      ref.Mime,
			FileName:      ref.FileName,
			SizeBytes:     int64(ref.FileLength),
			DownloadState: store.MediaPending,
		})
		if ref.Kind == "image" && ref.FileLength > 0 && ref.FileLength <= media.SmallImageAutoBytes && s.media != nil {
			go s.autoDownload(sm.MediaID, ref)
		}
	}
	return sm, nil
}

func (s *Syncer) autoDownload(mediaID string, ref media.DownloadRef) {
	if s.media == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_ = s.store.UpdateMediaState(ctx, mediaID, store.MediaDownloading, "")
	dest, err := s.media.AllocPath(ref.Kind, ref.FileName)
	if err != nil {
		_ = s.store.UpdateMediaState(ctx, mediaID, store.MediaFailed, "")
		return
	}
	if err := s.media.DownloadRefToFile(ctx, ref, dest); err != nil {
		s.logger.Debug("auto download failed", "err", err, "id", mediaID)
		_ = s.store.UpdateMediaState(ctx, mediaID, store.MediaFailed, "")
		s.bus.Publish(app.Event{Kind: app.EventMediaUpdated, MediaID: mediaID, Status: store.MediaFailed})
		return
	}
	_ = s.store.UpdateMediaState(ctx, mediaID, store.MediaReady, dest)
	s.bus.Publish(app.Event{Kind: app.EventMediaUpdated, MediaID: mediaID, Status: store.MediaReady, LocalPath: dest})
}

// RefreshAllNames corrects 1:1 address-book names, group subjects, and status label.
func (s *Syncer) RefreshAllNames(ctx context.Context) {
	s.RefreshChatNames(ctx)
	s.RefreshGroupNames(ctx)
	s.fixSpecialChats(ctx)
}

// RefreshChatNames re-resolves sidebar names from the address book (LID↔PN aware).
func (s *Syncer) RefreshChatNames(ctx context.Context) {
	client := s.clientOrNil()
	if client == nil {
		return
	}
	chats, err := s.store.ListChats(ctx, 500)
	if err != nil {
		s.logger.Warn("refresh chat names: list", "err", err)
		return
	}
	changed := false
	for _, c := range chats {
		if ctx.Err() != nil {
			return
		}
		jid, err := types.ParseJID(c.ID)
		if err != nil || jidutil.IsGroup(jid) || jidutil.IsBroadcastList(jid) {
			continue
		}
		resolved := contacts.ResolveDisplayName(ctx, client, jid, "")
		if !resolved.FromAddressBook || resolved.Name == "" || resolved.Name == c.Name {
			continue
		}
		if err := s.store.SetChatName(ctx, c.ID, resolved.Name, false); err != nil {
			s.logger.Debug("refresh chat name", "err", err, "id", c.ID)
			continue
		}
		changed = true
	}
	if changed {
		s.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	}
}

// RefreshGroupNames pulls official group subjects from WhatsApp (GetJoinedGroups).
func (s *Syncer) RefreshGroupNames(ctx context.Context) {
	client := s.clientOrNil()
	if client == nil || !client.IsConnected() {
		return
	}
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		s.logger.Warn("GetJoinedGroups", "err", err)
		return
	}
	changed := false
	for _, g := range groups {
		if g == nil || g.JID.IsEmpty() {
			continue
		}
		name := strings.TrimSpace(g.Name)
		id := g.JID.ToNonAD().String()
		isCommunity := g.IsParent || g.IsDefaultSubGroup || !g.LinkedParentJID.IsEmpty()
		_ = s.store.EnsureChatExists(ctx, id, name, true)
		if name != "" {
			if err := s.store.SetChatName(ctx, id, name, true); err != nil {
				s.logger.Debug("set group name", "err", err, "id", id)
			}
		}
		if err := s.store.SetChatCommunity(ctx, id, isCommunity); err != nil {
			s.logger.Debug("set community flag", "err", err, "id", id)
		} else {
			changed = true
		}
	}
	if changed {
		s.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	}
}

// ApplyGroupNameUpdate handles *events.GroupInfo name changes.
func (s *Syncer) ApplyGroupNameUpdate(ctx context.Context, evt *events.GroupInfo) {
	if evt == nil || evt.Name == nil {
		return
	}
	name := strings.TrimSpace(evt.Name.Name)
	if name == "" {
		return
	}
	id := evt.JID.ToNonAD().String()
	_ = s.store.EnsureChatExists(ctx, id, name, true)
	_ = s.store.SetChatName(ctx, id, name, true)
	s.bus.Publish(app.Event{Kind: app.EventChatsDirty})
}

func (s *Syncer) fixSpecialChats(ctx context.Context) {
	chats, err := s.store.ListChats(ctx, 500)
	if err != nil {
		return
	}
	changed := false
	for _, c := range chats {
		jid, err := types.ParseJID(c.ID)
		if err != nil {
			continue
		}
		if jidutil.IsStatusBroadcast(jid) {
			if c.Name != "Estados" || c.IsGroup {
				_ = s.store.SetChatName(ctx, c.ID, "Estados", false)
				changed = true
			}
		}
	}
	if changed {
		s.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	}
}
