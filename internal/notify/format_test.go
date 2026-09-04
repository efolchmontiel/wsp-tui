package notify

import "testing"

func TestFormatIncoming(t *testing.T) {
	tests := []struct {
		name       string
		chatName   string
		senderName string
		isGroup    bool
		text       string
		wantTitle  string
		wantBody   string
	}{
		{
			name:       "dm uses chat name as title",
			chatName:   "Papito",
			senderName: "Christian",
			text:       "hola",
			wantTitle:  "Papito",
			wantBody:   "hola",
		},
		{
			name:       "dm falls back to sender when chat unnamed",
			senderName: "54911",
			text:       "RAM 1500 2023 falla solucionada",
			wantTitle:  "54911",
			wantBody:   "RAM 1500 2023 falla solucionada",
		},
		{
			name:      "dm empty identity falls back to app name",
			text:      "ping",
			wantTitle: "WhatsTUI",
			wantBody:  "ping",
		},
		{
			name:       "group prefixes sender in body",
			chatName:   "Familia",
			senderName: "Ana",
			isGroup:    true,
			text:       "llegamos",
			wantTitle:  "Familia",
			wantBody:   "Ana: llegamos",
		},
		{
			name:       "group without chat name",
			senderName: "Ana",
			isGroup:    true,
			text:       "ok",
			wantTitle:  "Grupo",
			wantBody:   "Ana: ok",
		},
		{
			name:      "group without sender keeps text",
			chatName:  "Trabajo",
			isGroup:   true,
			text:      "standup",
			wantTitle: "Trabajo",
			wantBody:  "standup",
		},
		{
			name:       "trims whitespace",
			chatName:   "  Papito  ",
			senderName: "  Ana  ",
			isGroup:    true,
			text:       "  hola  ",
			wantTitle:  "Papito",
			wantBody:   "Ana: hola",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body := FormatIncoming(tt.chatName, tt.senderName, tt.isGroup, tt.text)
			if title != tt.wantTitle || body != tt.wantBody {
				t.Fatalf("got title=%q body=%q want title=%q body=%q", title, body, tt.wantTitle, tt.wantBody)
			}
		})
	}
}
