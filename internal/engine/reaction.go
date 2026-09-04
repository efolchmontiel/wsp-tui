package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/app"
	"go.mau.fi/whatsmeow/types"
)

// SendReaction sends an emoji reaction to an existing message.
// Empty emoji removes the current user's reaction.
func (e *Engine) SendReaction(chatID, targetMsgID, emoji string) error {
	chatID = strings.TrimSpace(chatID)
	targetMsgID = strings.TrimSpace(targetMsgID)
	if chatID == "" || targetMsgID == "" {
		return fmt.Errorf("chat/mensaje requerido")
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("chat id: %w", err)
	}

	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	if e.store == nil {
		return fmt.Errorf("store unavailable")
	}

	ctx := context.Background()
	target, err := e.store.GetMessage(ctx, chatID, targetMsgID)
	if err != nil {
		return fmt.Errorf("mensaje objetivo: %w", err)
	}

	sender := types.EmptyJID
	if target.Sender != "" {
		sender, err = types.ParseJID(target.Sender)
		if err != nil {
			return fmt.Errorf("sender: %w", err)
		}
	}
	if target.IsFromMe && client.Store.ID != nil {
		sender = client.Store.ID.ToNonAD()
	}
	if sender.IsEmpty() {
		return fmt.Errorf("no se pudo resolver el autor del mensaje")
	}

	own := ""
	if client.Store.ID != nil {
		own = client.Store.ID.ToNonAD().String()
	}
	updated, applied, err := e.store.ApplyReaction(ctx, chatID, targetMsgID, own, emoji)
	if err != nil {
		return err
	}
	if applied {
		cp := updated
		e.bus.Publish(app.Event{
			Kind:       app.EventMessageUpserted,
			Msg:        &cp,
			ChatID:     chatID,
			IsReaction: true,
		})
	}

	waMsg := client.BuildReaction(jid, sender, types.MessageID(targetMsgID), emoji)
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, sendErr := client.SendMessage(sendCtx, jid, waMsg)
		if sendErr != nil {
			e.logger.Warn("send reaction", "err", sendErr, "target", targetMsgID)
			e.bus.Publish(app.Event{
				Kind:    app.EventError,
				Message: "No se pudo enviar la reacción: " + sendErr.Error(),
			})
		}
	}()
	return nil
}
