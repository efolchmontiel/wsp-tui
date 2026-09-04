package media_test

import (
	"encoding/base64"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/media"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow"
)

func TestClassifyPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path    string
		msgType string
		waType  whatsmeow.MediaType
	}{
		{"/tmp/pic.jpg", store.TypeImage, whatsmeow.MediaImage},
		{"photo.PNG", store.TypeImage, whatsmeow.MediaImage},
		{"clip.mp4", store.TypeVideo, whatsmeow.MediaVideo},
		{"voice.ogg", store.TypeAudio, whatsmeow.MediaAudio},
		{"report.pdf", store.TypeDocument, whatsmeow.MediaDocument},
		{"archive.zip", store.TypeDocument, whatsmeow.MediaDocument},
	}
	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			gotType, gotWA, mime := media.ClassifyPath(tt.path)
			if gotType != tt.msgType {
				t.Fatalf("type: got %q want %q (mime=%s)", gotType, tt.msgType, mime)
			}
			if gotWA != tt.waType {
				t.Fatalf("wa type: got %q want %q", gotWA, tt.waType)
			}
			if mime == "" {
				t.Fatal("empty mime")
			}
		})
	}
}

func TestEncodeParseRefRoundTrip(t *testing.T) {
	t.Parallel()
	ref := media.DownloadRef{
		Kind:          "image",
		DirectPath:    "/v/t62/abc",
		URL:           "https://example.com/x",
		MediaKeyB64:   base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		FileSHAB64:    base64.StdEncoding.EncodeToString(make([]byte, 32)),
		FileEncSHAB64: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		Mime:          "image/jpeg",
		FileName:      "image.jpg",
		FileLength:    1234,
	}
	json := ref.EncodeJSON()
	got, ok := media.ParseRef(json)
	if !ok {
		t.Fatalf("parse failed on %s", json)
	}
	if got.Kind != ref.Kind || got.DirectPath != ref.DirectPath || got.FileLength != ref.FileLength {
		t.Fatalf("round-trip mismatch: %+v vs %+v", got, ref)
	}
	dl, err := got.ToDownloadable()
	if err != nil {
		t.Fatal(err)
	}
	if dl == nil {
		t.Fatal("nil downloadable")
	}
}

func TestFormatSize(t *testing.T) {
	t.Parallel()
	if media.FormatSize(500) != "500 B" {
		t.Fatal(media.FormatSize(500))
	}
	if media.FormatSize(2048) != "2.0 KB" {
		t.Fatal(media.FormatSize(2048))
	}
}

func TestSubdir(t *testing.T) {
	t.Parallel()
	if media.Subdir("image") != "images" {
		t.Fatal(media.Subdir("image"))
	}
	if media.Subdir(store.TypeVideo) != "videos" {
		t.Fatal(media.Subdir(store.TypeVideo))
	}
}
