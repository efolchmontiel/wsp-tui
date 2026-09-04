package msgutil

import (
	"strings"

	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// ExtractText returns the best-effort plain text body of a WhatsApp message.
func ExtractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if t := msg.GetConversation(); t != "" {
		return t
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		if t := ext.GetText(); t != "" {
			return t
		}
	}
	if img := msg.GetImageMessage(); img != nil {
		if c := img.GetCaption(); c != "" {
			return c
		}
		return "imagen"
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		if c := vid.GetCaption(); c != "" {
			return c
		}
		return "video"
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		if aud.GetPTT() {
			return "voz"
		}
		return "audio"
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		if n := doc.GetFileName(); n != "" {
			return n
		}
		return "documento"
	}
	if msg.GetStickerMessage() != nil {
		return "sticker"
	}
	if msg.GetContactMessage() != nil {
		return "contacto"
	}
	if msg.GetLocationMessage() != nil || msg.GetLiveLocationMessage() != nil {
		return "ubicación"
	}
	if r := msg.GetReactionMessage(); r != nil {
		return r.GetText()
	}
	if msg.GetEncReactionMessage() != nil {
		return ""
	}
	if cl := msg.GetCallLogMesssage(); cl != nil {
		kind := "voz"
		if cl.GetIsVideo() {
			kind = "video"
		}
		switch cl.GetCallOutcome() {
		case waE2E.CallLogMessage_MISSED, waE2E.CallLogMessage_REJECTED,
			waE2E.CallLogMessage_SILENCED_BY_DND, waE2E.CallLogMessage_SILENCED_UNKNOWN_CALLER:
			return "Llamada perdida · " + kind
		case waE2E.CallLogMessage_ONGOING:
			return "Llamada entrante · " + kind
		case waE2E.CallLogMessage_CONNECTED, waE2E.CallLogMessage_ACCEPTED_ELSEWHERE:
			return "Llamada · " + kind
		default:
			return "Llamada · " + kind
		}
	}
	return ""
}

// ClassifyType maps a proto message to our store type + preview text.
func ClassifyType(msg *waE2E.Message) (msgType, preview string) {
	if msg == nil {
		return store.TypeOther, ""
	}
	if cl := msg.GetCallLogMesssage(); cl != nil {
		text := ExtractText(msg)
		switch cl.GetCallOutcome() {
		case waE2E.CallLogMessage_MISSED, waE2E.CallLogMessage_REJECTED,
			waE2E.CallLogMessage_SILENCED_BY_DND, waE2E.CallLogMessage_SILENCED_UNKNOWN_CALLER:
			return store.TypeCallMissed, text
		case waE2E.CallLogMessage_ONGOING:
			return store.TypeCallIncoming, text
		default:
			return store.TypeOther, text
		}
	}
	if msg.GetReactionMessage() != nil || msg.GetEncReactionMessage() != nil {
		emoji := ExtractText(msg)
		if emoji == "" {
			return store.TypeReaction, "reacción"
		}
		return store.TypeReaction, emoji
	}
	text := ExtractText(msg)
	switch {
	case msg.GetConversation() != "" || msg.GetExtendedTextMessage() != nil:
		return store.TypeText, text
	case msg.GetImageMessage() != nil:
		return store.TypeImage, text
	case msg.GetVideoMessage() != nil:
		return store.TypeVideo, text
	case msg.GetAudioMessage() != nil:
		return store.TypeAudio, text
	case msg.GetDocumentMessage() != nil:
		return store.TypeDocument, text
	case msg.GetStickerMessage() != nil:
		return store.TypeSticker, text
	default:
		if text == "" {
			return store.TypeOther, "Unsupported message"
		}
		return store.TypeOther, text
	}
}

// IsReaction reports whether the proto is an emoji reaction (plain or encrypted).
func IsReaction(msg *waE2E.Message) bool {
	return msg != nil && (msg.GetReactionMessage() != nil || msg.GetEncReactionMessage() != nil)
}

// Preview truncates sidebar preview text.
func Preview(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	// rune-safe trim
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
