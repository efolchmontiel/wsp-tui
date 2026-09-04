package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// CycleDisappearingTimer advances Off → 24h → 7d → 90d → Off for the chat (1:1 or group).
func (e *Engine) CycleDisappearingTimer(ctx context.Context, chatID string) (string, error) {
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return "", err
	}
	cur := e.store.GetChatEphemeralSeconds(ctx, chatID)
	next, label := store.EphemeralCycle(cur)
	if err := client.SetDisappearingTimer(ctx, jid, next, time.Time{}); err != nil {
		return "", err
	}
	_ = e.store.SetChatEphemeralSeconds(ctx, chatID, int(next.Seconds()))
	return label, nil
}

// Keep a compile-time link to official timer constants.
var (
	_ = whatsmeow.DisappearingTimerOff
	_ = whatsmeow.DisappearingTimer24Hours
	_ = whatsmeow.DisappearingTimer7Days
	_ = whatsmeow.DisappearingTimer90Days
)
