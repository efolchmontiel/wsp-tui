package media

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// DownloadRef holds enough fields to re-download without keeping the full WA event.
type DownloadRef struct {
	Kind          string `json:"kind"` // image|video|audio|document|sticker
	DirectPath    string `json:"direct_path"`
	URL           string `json:"url,omitempty"`
	MediaKeyB64   string `json:"media_key"`
	FileSHAB64    string `json:"file_sha256"`
	FileEncSHAB64 string `json:"file_enc_sha256"`
	Mime          string `json:"mime"`
	FileName      string `json:"file_name"`
	FileLength    uint64 `json:"file_length"`
}

// ExtractRef builds a DownloadRef from a WhatsApp message.
func ExtractRef(msg *waE2E.Message) (DownloadRef, bool) {
	if msg == nil {
		return DownloadRef{}, false
	}
	switch {
	case msg.GetImageMessage() != nil:
		m := msg.GetImageMessage()
		return refFrom("image", m.GetDirectPath(), m.GetURL(), m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetMimetype(), "image.jpg", m.GetFileLength()), true
	case msg.GetVideoMessage() != nil:
		m := msg.GetVideoMessage()
		return refFrom("video", m.GetDirectPath(), m.GetURL(), m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetMimetype(), "video.mp4", m.GetFileLength()), true
	case msg.GetAudioMessage() != nil:
		m := msg.GetAudioMessage()
		name := "audio.ogg"
		if m.GetPTT() {
			name = "voice.ogg"
		}
		return refFrom("audio", m.GetDirectPath(), m.GetURL(), m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetMimetype(), name, m.GetFileLength()), true
	case msg.GetDocumentMessage() != nil:
		m := msg.GetDocumentMessage()
		name := m.GetFileName()
		if name == "" {
			name = "document.bin"
		}
		return refFrom("document", m.GetDirectPath(), m.GetURL(), m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetMimetype(), name, m.GetFileLength()), true
	case msg.GetStickerMessage() != nil:
		m := msg.GetStickerMessage()
		return refFrom("sticker", m.GetDirectPath(), m.GetURL(), m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetMimetype(), "sticker.webp", m.GetFileLength()), true
	default:
		return DownloadRef{}, false
	}
}

func refFrom(kind, direct, url string, key, sha, encSHA []byte, mimeType, fileName string, length uint64) DownloadRef {
	return DownloadRef{
		Kind:          kind,
		DirectPath:    direct,
		URL:           url,
		MediaKeyB64:   base64.StdEncoding.EncodeToString(key),
		FileSHAB64:    base64.StdEncoding.EncodeToString(sha),
		FileEncSHAB64: base64.StdEncoding.EncodeToString(encSHA),
		Mime:          mimeType,
		FileName:      fileName,
		FileLength:    length,
	}
}

// EncodeJSON stores the ref as message metadata JSON (may wrap other fields later).
func (r DownloadRef) EncodeJSON() string {
	type wrap struct {
		Media DownloadRef `json:"media"`
	}
	b, err := json.Marshal(wrap{Media: r})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ParseRef reads media ref from message metadata_json.
func ParseRef(metadataJSON string) (DownloadRef, bool) {
	if strings.TrimSpace(metadataJSON) == "" || metadataJSON == "{}" {
		return DownloadRef{}, false
	}
	var wrap struct {
		Media DownloadRef `json:"media"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &wrap); err != nil {
		return DownloadRef{}, false
	}
	if wrap.Media.DirectPath == "" || wrap.Media.Kind == "" {
		return DownloadRef{}, false
	}
	return wrap.Media, true
}

// ToDownloadable rebuilds a whatsmeow DownloadableMessage from the ref.
func (r DownloadRef) ToDownloadable() (whatsmeow.DownloadableMessage, error) {
	key, err := base64.StdEncoding.DecodeString(r.MediaKeyB64)
	if err != nil {
		return nil, fmt.Errorf("media key: %w", err)
	}
	sha, err := base64.StdEncoding.DecodeString(r.FileSHAB64)
	if err != nil {
		return nil, fmt.Errorf("file sha: %w", err)
	}
	enc, err := base64.StdEncoding.DecodeString(r.FileEncSHAB64)
	if err != nil {
		return nil, fmt.Errorf("enc sha: %w", err)
	}
	switch r.Kind {
	case "image", "sticker":
		if r.Kind == "sticker" {
			return &waE2E.StickerMessage{
				URL: proto.String(r.URL), DirectPath: proto.String(r.DirectPath),
				MediaKey: key, FileSHA256: sha, FileEncSHA256: enc,
				Mimetype: proto.String(r.Mime), FileLength: proto.Uint64(r.FileLength),
			}, nil
		}
		return &waE2E.ImageMessage{
			URL: proto.String(r.URL), DirectPath: proto.String(r.DirectPath),
			MediaKey: key, FileSHA256: sha, FileEncSHA256: enc,
			Mimetype: proto.String(r.Mime), FileLength: proto.Uint64(r.FileLength),
		}, nil
	case "video":
		return &waE2E.VideoMessage{
			URL: proto.String(r.URL), DirectPath: proto.String(r.DirectPath),
			MediaKey: key, FileSHA256: sha, FileEncSHA256: enc,
			Mimetype: proto.String(r.Mime), FileLength: proto.Uint64(r.FileLength),
		}, nil
	case "audio":
		return &waE2E.AudioMessage{
			URL: proto.String(r.URL), DirectPath: proto.String(r.DirectPath),
			MediaKey: key, FileSHA256: sha, FileEncSHA256: enc,
			Mimetype: proto.String(r.Mime), FileLength: proto.Uint64(r.FileLength),
		}, nil
	case "document":
		return &waE2E.DocumentMessage{
			URL: proto.String(r.URL), DirectPath: proto.String(r.DirectPath),
			MediaKey: key, FileSHA256: sha, FileEncSHA256: enc,
			Mimetype: proto.String(r.Mime), FileLength: proto.Uint64(r.FileLength),
			FileName: proto.String(r.FileName),
		}, nil
	default:
		return nil, fmt.Errorf("unknown media kind %q", r.Kind)
	}
}

// MediaTypeForKind maps our kind to whatsmeow MediaType.
func MediaTypeForKind(kind string) whatsmeow.MediaType {
	switch kind {
	case "image", "sticker":
		return whatsmeow.MediaImage
	case "video":
		return whatsmeow.MediaVideo
	case "audio":
		return whatsmeow.MediaAudio
	default:
		return whatsmeow.MediaDocument
	}
}

// DetectMIME guesses MIME from path; falls back to octet-stream.
func DetectMIME(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" {
		if m := mime.TypeByExtension(ext); m != "" {
			return m
		}
	}
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".ogg", ".opus":
		return "audio/ogg; codecs=opus"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".rar":
		return "application/vnd.rar"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// ClassifyPath picks store type + whatsmeow media type from MIME/path.
func ClassifyPath(path string) (msgType string, waType whatsmeow.MediaType, mimeType string) {
	mimeType = DetectMIME(path)
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return store.TypeImage, whatsmeow.MediaImage, mimeType
	case strings.HasPrefix(mimeType, "video/"):
		return store.TypeVideo, whatsmeow.MediaVideo, mimeType
	case strings.HasPrefix(mimeType, "audio/"):
		return store.TypeAudio, whatsmeow.MediaAudio, mimeType
	default:
		return store.TypeDocument, whatsmeow.MediaDocument, mimeType
	}
}

// Subdir for filesystem layout under media/.
func Subdir(kind string) string {
	switch kind {
	case "image", "sticker":
		return "images"
	case "video":
		return "videos"
	case "audio":
		return "audio"
	case "document":
		return "documents"
	default:
		return "documents"
	}
}

// SmallImageAutoBytes: images at or below this size auto-download.
const SmallImageAutoBytes = 512 * 1024
