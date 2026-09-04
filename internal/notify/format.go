package notify

import "strings"

// FormatIncoming builds title and body for an incoming message toast.
// chatName is the conversation label; senderName is who wrote the message.
func FormatIncoming(chatName, senderName string, isGroup bool, text string) (title, body string) {
	chatName = strings.TrimSpace(chatName)
	senderName = strings.TrimSpace(senderName)
	text = strings.TrimSpace(text)

	if isGroup {
		title = chatName
		if title == "" {
			title = "Grupo"
		}
		switch {
		case senderName != "" && text != "":
			body = senderName + ": " + text
		case senderName != "":
			body = senderName
		default:
			body = text
		}
		return title, body
	}

	title = chatName
	if title == "" {
		title = senderName
	}
	if title == "" {
		title = "WhatsTUI"
	}
	return title, text
}
