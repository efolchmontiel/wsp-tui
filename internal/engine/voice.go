package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/app"
	"github.com/efolchmontiel/wsp-tui/internal/jidutil"
	"github.com/efolchmontiel/wsp-tui/internal/media"
	"github.com/efolchmontiel/wsp-tui/internal/msgutil"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// StartVoiceRecord begins microphone capture for a PTT voice note.
func (e *Engine) StartVoiceRecord() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.voiceRec != nil && e.voiceRec.Running() {
		return fmt.Errorf("ya estás grabando — tocá v de nuevo para enviar")
	}
	dir := filepath.Join(e.paths.MediaDir, "audio")
	rec, err := media.StartVoiceRecord(dir)
	if err != nil {
		return err
	}
	e.voiceRec = rec
	e.bus.Publish(app.Event{Kind: app.EventInfo, Message: "Grabando voz… (v enviar · Esc cancelar)"})
	return nil
}

// CancelVoiceRecord aborts an in-progress recording.
func (e *Engine) CancelVoiceRecord() {
	e.mu.Lock()
	rec := e.voiceRec
	e.voiceRec = nil
	e.mu.Unlock()
	if rec != nil {
		rec.Cancel()
		e.bus.Publish(app.Event{Kind: app.EventInfo, Message: "Grabación cancelada"})
	}
}

// IsRecording reports active voice capture.
func (e *Engine) IsRecording() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.voiceRec != nil && e.voiceRec.Running()
}

// RecordingElapsed returns current take length.
func (e *Engine) RecordingElapsed() time.Duration {
	e.mu.Lock()
	rec := e.voiceRec
	e.mu.Unlock()
	if rec == nil {
		return 0
	}
	return rec.Elapsed()
}

// StopVoiceRecordAndSend finalizes the OGG and sends it as a WhatsApp voice note (PTT).
func (e *Engine) StopVoiceRecordAndSend(ctx context.Context, chatID string) (store.Message, error) {
	e.mu.Lock()
	rec := e.voiceRec
	e.voiceRec = nil
	e.mu.Unlock()
	if rec == nil {
		return store.Message{}, fmt.Errorf("no hay grabación activa")
	}
	path, seconds, err := rec.Stop()
	if err != nil {
		return store.Message{}, err
	}
	return e.SendVoiceNote(ctx, chatID, path, seconds)
}

// SendVoiceNote uploads an OGG/Opus file as a push-to-talk voice message.
func (e *Engine) SendVoiceNote(ctx context.Context, chatID, path string, seconds uint32) (store.Message, error) {
	path = filepath.Clean(path)
	st, err := os.Stat(path)
	if err != nil {
		return store.Message{}, err
	}
	if st.IsDir() || st.Size() == 0 {
		return store.Message{}, fmt.Errorf("archivo de voz inválido")
	}
	if seconds == 0 {
		seconds = 1
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

	mimeType := "audio/ogg; codecs=opus"
	fileName := filepath.Base(path)
	id := client.GenerateMessageID()
	mediaID := chatID + "|" + string(id)
	now := time.Now()
	label := fmt.Sprintf("voz · %ds · %s", seconds, media.FormatSize(uint64(st.Size())))

	msg := store.Message{
		ID:        string(id),
		ChatID:    chatID,
		Timestamp: now.Unix(),
		Text:      label,
		Type:      store.TypeAudio,
		Status:    store.StatusSending,
		IsFromMe:  true,
		MediaID:   mediaID,
	}
	if client.Store.ID != nil {
		msg.Sender = client.Store.ID.ToNonAD().String()
	}

	if e.store != nil {
		_ = e.store.EnsureChatExists(ctx, chatID, "", jidutil.IsGroup(jid))
		if err := e.store.UpsertMessage(ctx, msg); err != nil {
			return store.Message{}, err
		}
		_ = e.store.UpsertMedia(ctx, store.MediaRow{
			ID:            mediaID,
			ChatID:        chatID,
			MessageID:     string(id),
			MimeType:      mimeType,
			FileName:      fileName,
			SizeBytes:     st.Size(),
			LocalPath:     path,
			DownloadState: store.MediaReady,
		})
		_ = e.store.TouchChatPreview(ctx, chatID, msgutil.Preview(label, 80), msg.Timestamp, 0)
	}

	cp := msg
	e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: chatID})
	e.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 10, Message: "Enviando nota de voz…"})

	go e.uploadAndSendVoice(client, jid, chatID, string(id), mediaID, path, mimeType, seconds, st.Size())
	return msg, nil
}

func (e *Engine) uploadAndSendVoice(
	client *whatsmeow.Client,
	jid types.JID,
	chatID, msgID, mediaID, path, mimeType string,
	seconds uint32,
	size int64,
) {
	sendCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 30, Message: "Subiendo voz…"})

	var resp whatsmeow.UploadResponse
	var err error
	if size <= maxInlineUploadBytes {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			e.failSend(chatID, msgID, mediaID, readErr)
			return
		}
		resp, err = client.Upload(sendCtx, data, whatsmeow.MediaAudio)
	} else {
		f, openErr := os.Open(path)
		if openErr != nil {
			e.failSend(chatID, msgID, mediaID, openErr)
			return
		}
		resp, err = client.UploadReader(sendCtx, f, nil, whatsmeow.MediaAudio)
		_ = f.Close()
	}
	if err != nil {
		e.failSend(chatID, msgID, mediaID, err)
		return
	}

	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 80, Message: "Enviando voz…"})

	waMsg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		MediaKey:      resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    proto.Uint64(resp.FileLength),
		Mimetype:      proto.String(mimeType),
		Seconds:       proto.Uint32(seconds),
		PTT:           proto.Bool(true),
	}}
	_, err = client.SendMessage(sendCtx, jid, waMsg, whatsmeow.SendRequestExtra{ID: types.MessageID(msgID)})
	if err != nil {
		e.failSend(chatID, msgID, mediaID, err)
		return
	}
	if e.store != nil {
		_ = e.store.UpdateMessageStatus(context.Background(), chatID, msgID, store.StatusSent)
	}
	e.bus.Publish(app.Event{
		Kind:       app.EventMessageStatus,
		ChatID:     chatID,
		MessageIDs: []string{msgID},
		Status:     store.StatusSent,
	})
	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 100, Message: "Nota de voz enviada"})
}
