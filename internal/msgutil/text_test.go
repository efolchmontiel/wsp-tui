package msgutil

import (
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestExtractTextConversation(t *testing.T) {
	msg := &waE2E.Message{Conversation: proto.String("hola")}
	if got := ExtractText(msg); got != "hola" {
		t.Fatalf("got %q", got)
	}
}

func TestClassifyTypeReaction(t *testing.T) {
	msg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Text: proto.String("😂"),
		},
	}
	typ, preview := ClassifyType(msg)
	if typ != store.TypeReaction {
		t.Fatalf("type %s", typ)
	}
	if preview != "😂" {
		t.Fatalf("preview %q", preview)
	}
	if !IsReaction(msg) {
		t.Fatal("expected IsReaction")
	}
}



func TestPreview(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hell…"},
		{"a\nb", 10, "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := Preview(tt.in, tt.max); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}
