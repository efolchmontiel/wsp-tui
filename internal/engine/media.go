package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

const maxInlineUploadBytes = 8 << 20 // 8 MiB — larger files use UploadReader

// SetMedia wires the filesystem media manager.
func (e *Engine) SetMedia(m *media.Manager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.media = m
}

// SendFile uploads a local file and sends it as image/video/audio/document.
// Returns an optimistic local message immediately; network work runs async.
func (e *Engine) SendFile(ctx context.Context, chatID, path, caption string) (store.Message, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return store.Message{}, fmt.Errorf("empty path")
	}
	st, err := os.Stat(path)
	if err != nil {
		return store.Message{}, err
	}
	if st.IsDir() {
		return store.Message{}, fmt.Errorf("path is a directory")
	}

	jid, err := types.ParseJID(chatID)
	if err != nil {
		return store.Message{}, fmt.Errorf("chat id: %w", err)
	}

	e.mu.Lock()
	client := e.client
	mgr := e.media
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return store.Message{}, fmt.Errorf("not connected")
	}
	if mgr == nil {
		return store.Message{}, fmt.Errorf("media manager not ready")
	}

	msgType, waType, mimeType := media.ClassifyPath(path)
	fileName := filepath.Base(path)
	size := st.Size()
	id := client.GenerateMessageID()
	mediaID := chatID + "|" + string(id)
	now := time.Now()
	caption = strings.TrimSpace(caption)

	label := mediaLabel(msgType, fileName, uint64(size), caption)
	msg := store.Message{
		ID:        string(id),
		ChatID:    chatID,
		Timestamp: now.Unix(),
		Text:      label,
		Type:      msgType,
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
			SizeBytes:     size,
			LocalPath:     path,
			DownloadState: store.MediaReady,
		})
		_ = e.store.TouchChatPreview(ctx, chatID, msgutil.Preview(label, 80), msg.Timestamp, 0)
	}

	cp := msg
	e.bus.Publish(app.Event{Kind: app.EventMessageUpserted, Msg: &cp, ChatID: chatID})
	e.bus.Publish(app.Event{Kind: app.EventChatsDirty})
	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 5, Message: "Subiendo " + fileName + "…"})

	go e.uploadAndSend(client, jid, chatID, string(id), mediaID, path, fileName, mimeType, caption, msgType, waType, size)

	return msg, nil
}

func (e *Engine) uploadAndSend(
	client *whatsmeow.Client,
	jid types.JID,
	chatID, msgID, mediaID, path, fileName, mimeType, caption, msgType string,
	waType whatsmeow.MediaType,
	size int64,
) {
	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 20, Message: "Subiendo…"})

	var resp whatsmeow.UploadResponse
	var err error
	if size <= maxInlineUploadBytes {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			e.failSend(chatID, msgID, mediaID, readErr)
			return
		}
		resp, err = client.Upload(sendCtx, data, waType)
	} else {
		f, openErr := os.Open(path)
		if openErr != nil {
			e.failSend(chatID, msgID, mediaID, openErr)
			return
		}
		resp, err = client.UploadReader(sendCtx, f, nil, waType)
		_ = f.Close()
	}
	if err != nil {
		e.failSend(chatID, msgID, mediaID, err)
		return
	}

	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 80, Message: "Enviando…"})

	waMsg := buildMediaMessage(msgType, mimeType, fileName, caption, resp)
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
	e.bus.Publish(app.Event{Kind: app.EventUploadProgress, ChatID: chatID, MediaID: mediaID, Progress: 100, Message: "Enviado"})
}

func (e *Engine) failSend(chatID, msgID, mediaID string, err error) {
	e.logger.Warn("send media failed", "err", err, "id", msgID)
	if e.store != nil {
		_ = e.store.UpdateMessageStatus(context.Background(), chatID, msgID, store.StatusFailed)
	}
	e.bus.Publish(app.Event{
		Kind:       app.EventMessageStatus,
		ChatID:     chatID,
		MessageIDs: []string{msgID},
		Status:     store.StatusFailed,
		Message:    err.Error(),
	})
	e.bus.Publish(app.Event{
		Kind:     app.EventUploadProgress,
		ChatID:   chatID,
		MediaID:  mediaID,
		Progress: -1,
		Message:  "Falló: " + err.Error(),
	})
}

func buildMediaMessage(msgType, mimeType, fileName, caption string, resp whatsmeow.UploadResponse) *waE2E.Message {
	switch msgType {
	case store.TypeImage:
		im := &waE2E.ImageMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(resp.FileLength),
			Mimetype:      proto.String(mimeType),
		}
		if caption != "" {
			im.Caption = proto.String(caption)
		}
		return &waE2E.Message{ImageMessage: im}
	case store.TypeVideo:
		vm := &waE2E.VideoMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(resp.FileLength),
			Mimetype:      proto.String(mimeType),
		}
		if caption != "" {
			vm.Caption = proto.String(caption)
		}
		return &waE2E.Message{VideoMessage: vm}
	case store.TypeAudio:
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(resp.FileLength),
			Mimetype:      proto.String(mimeType),
		}}
	default:
		dm := &waE2E.DocumentMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(resp.FileLength),
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(fileName),
		}
		if caption != "" {
			dm.Caption = proto.String(caption)
		}
		return &waE2E.Message{DocumentMessage: dm}
	}
}

// DownloadMedia downloads an attachment on demand (async). Returns immediately.
func (e *Engine) DownloadMedia(mediaID string) error {
	e.mu.Lock()
	mgr := e.media
	e.mu.Unlock()
	if mgr == nil {
		return fmt.Errorf("media manager not ready")
	}
	if mediaID == "" {
		return fmt.Errorf("empty media id")
	}
	go e.downloadMediaAsync(mediaID)
	return nil
}

func (e *Engine) downloadMediaAsync(mediaID string) {
	e.mu.Lock()
	mgr := e.media
	e.mu.Unlock()
	if mgr == nil || e.store == nil {
		e.bus.Publish(app.Event{Kind: app.EventError, Message: "media manager not ready"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	row, err := e.store.GetMedia(ctx, mediaID)
	if err != nil {
		e.bus.Publish(app.Event{Kind: app.EventError, Message: "media: " + err.Error()})
		return
	}
	if row.DownloadState == store.MediaReady && row.LocalPath != "" {
		if _, err := os.Stat(row.LocalPath); err == nil {
			e.bus.Publish(app.Event{Kind: app.EventMediaUpdated, MediaID: mediaID, Status: store.MediaReady, LocalPath: row.LocalPath, Message: "Ya descargado"})
			return
		}
	}

	msg, err := e.store.GetMessage(ctx, row.ChatID, row.MessageID)
	if err != nil {
		e.bus.Publish(app.Event{Kind: app.EventError, Message: err.Error()})
		return
	}
	ref, ok := media.ParseRef(msg.MetadataJSON)
	if !ok {
		e.bus.Publish(app.Event{Kind: app.EventError, Message: "sin metadatos de descarga para este adjunto"})
		return
	}

	_ = e.store.UpdateMediaState(ctx, mediaID, store.MediaDownloading, "")
	e.bus.Publish(app.Event{Kind: app.EventMediaUpdated, MediaID: mediaID, Status: store.MediaDownloading, Message: "Descargando…"})

	dest, err := mgr.AllocPath(ref.Kind, ref.FileName)
	if err != nil {
		_ = e.store.UpdateMediaState(ctx, mediaID, store.MediaFailed, "")
		e.bus.Publish(app.Event{Kind: app.EventMediaUpdated, MediaID: mediaID, Status: store.MediaFailed, Message: err.Error()})
		return
	}
	if err := mgr.DownloadRefToFile(ctx, ref, dest); err != nil {
		_ = e.store.UpdateMediaState(ctx, mediaID, store.MediaFailed, "")
		e.bus.Publish(app.Event{Kind: app.EventMediaUpdated, MediaID: mediaID, Status: store.MediaFailed, Message: err.Error()})
		return
	}
	_ = e.store.UpdateMediaState(ctx, mediaID, store.MediaReady, dest)
	e.bus.Publish(app.Event{Kind: app.EventMediaUpdated, MediaID: mediaID, Status: store.MediaReady, LocalPath: dest, Message: "Listo"})
}

// OpenMedia opens a ready local file externally (mpv / xdg-open). Downloads first if needed.
// Audio/video: exclusive — same clip while playing is a no-op; a different clip stops the previous.
func (e *Engine) OpenMedia(mediaID string) error {
	if mediaID == "" {
		return fmt.Errorf("empty media id")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		row, err := e.store.GetMedia(ctx, mediaID)
		if err != nil {
			e.bus.Publish(app.Event{Kind: app.EventError, Message: err.Error()})
			return
		}
		if row.DownloadState != store.MediaReady || row.LocalPath == "" {
			e.downloadMediaAsync(mediaID)
			row, err = e.store.GetMedia(ctx, mediaID)
			if err != nil || row.DownloadState != store.MediaReady || row.LocalPath == "" {
				e.bus.Publish(app.Event{Kind: app.EventError, Message: "no se pudo descargar el adjunto"})
				return
			}
		}
		kind := "document"
		switch {
		case strings.HasPrefix(row.MimeType, "image/"):
			kind = "image"
		case strings.HasPrefix(row.MimeType, "video/"):
			kind = "video"
		case strings.HasPrefix(row.MimeType, "audio/"):
			kind = "audio"
		}

		if kind == "audio" || kind == "video" {
			e.playMu.Lock()
			if e.playID == mediaID && e.playCmd != nil && e.playCmd.Process != nil {
				e.playMu.Unlock()
				e.bus.Publish(app.Event{Kind: app.EventInfo, Message: "Ya está reproduciendo"})
				return
			}
			e.stopPlaybackLocked()
			e.playGen++
			gen := e.playGen
			e.playID = mediaID
			e.playMu.Unlock()

			cmd, err := media.OpenExternal(row.LocalPath, kind)
			if err != nil {
				e.playMu.Lock()
				if e.playGen == gen {
					e.playID = ""
					e.playCmd = nil
				}
				e.playMu.Unlock()
				e.bus.Publish(app.Event{Kind: app.EventError, Message: err.Error()})
				e.bus.Publish(app.Event{Kind: app.EventMediaStopped, MediaID: mediaID})
				return
			}

			e.playMu.Lock()
			if e.playGen != gen {
				e.playMu.Unlock()
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
				return
			}
			e.playCmd = cmd
			e.playMu.Unlock()

			e.bus.Publish(app.Event{
				Kind: app.EventMediaPlaying, MediaID: mediaID, LocalPath: row.LocalPath,
				ChatID: row.ChatID, Message: row.MessageID, Status: kind,
			})
			go func(gen uint64, id string, c *exec.Cmd) {
				_ = c.Wait()
				e.playMu.Lock()
				if e.playGen == gen {
					e.playCmd = nil
					e.playID = ""
				}
				e.playMu.Unlock()
				e.bus.Publish(app.Event{Kind: app.EventMediaStopped, MediaID: id})
			}(gen, mediaID, cmd)
			return
		}

		_, err = media.OpenExternal(row.LocalPath, kind)
		if err != nil {
			e.bus.Publish(app.Event{Kind: app.EventError, Message: err.Error()})
			return
		}
		e.bus.Publish(app.Event{Kind: app.EventInfo, Message: "Abierto: " + filepath.Base(row.LocalPath)})
	}()
	return nil
}

// stopPlaybackLocked kills the current mpv process. Caller must hold playMu.
func (e *Engine) stopPlaybackLocked() {
	if e.playCmd == nil || e.playCmd.Process == nil {
		e.playCmd = nil
		e.playID = ""
		return
	}
	prev := e.playCmd
	prevID := e.playID
	e.playCmd = nil
	e.playID = ""
	_ = prev.Process.Kill()
	go func() {
		_ = prev.Wait()
		if prevID != "" {
			e.bus.Publish(app.Event{Kind: app.EventMediaStopped, MediaID: prevID})
		}
	}()
}

// StopPlayback stops any current audio/video playback (no-op if idle).
func (e *Engine) StopPlayback() {
	e.playMu.Lock()
	defer e.playMu.Unlock()
	e.playGen++
	e.stopPlaybackLocked()
}

func mediaLabel(msgType, fileName string, size uint64, caption string) string {
	prefix := "file"
	switch msgType {
	case store.TypeImage:
		prefix = "img"
	case store.TypeVideo:
		prefix = "vid"
	case store.TypeAudio:
		prefix = "audio"
	case store.TypeDocument:
		prefix = "doc"
	}
	base := fmt.Sprintf("%s %s · %s", prefix, fileName, media.FormatSize(size))
	if caption != "" {
		return base + "\n" + caption
	}
	return base
}

// RequestMessageResend asks the primary phone to re-send a message body
// (used when we stored a media placeholder without download keys).
func (e *Engine) RequestMessageResend(ctx context.Context, chatID, sender, messageID string) error {
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	chat, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("chat id: %w", err)
	}
	var from types.JID
	if sender != "" {
		from, err = types.ParseJID(sender)
		if err != nil {
			return fmt.Errorf("sender: %w", err)
		}
	} else {
		from = chat
	}
	_, err = client.SendPeerMessage(ctx, client.BuildUnavailableMessageRequest(chat, from, messageID))
	if err != nil {
		return fmt.Errorf("pedir mensaje: %w", err)
	}
	e.bus.Publish(app.Event{
		Kind:    app.EventInfo,
		Message: "Pedí el adjunto al teléfono… cuando llegue, tocá o de nuevo",
	})
	return nil
}
