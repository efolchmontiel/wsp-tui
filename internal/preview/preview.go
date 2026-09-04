package preview

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"sync"

	"github.com/blacktop/go-termimg"
	"github.com/efolchmontiel/wsp-tui/internal/config"
	"golang.org/x/image/draw"
)

// Defaults for inline previews in the transcript.
const (
	DefaultWidth  = 36
	DefaultHeight = 12
)

// ProtocolFromConfig maps config → termimg protocol.
func ProtocolFromConfig(p config.PreviewProtocol) termimg.Protocol {
	switch config.ParsePreviewProtocol(string(p)) {
	case config.PreviewKitty:
		return termimg.Kitty
	case config.PreviewITerm2:
		return termimg.ITerm2
	case config.PreviewSixel:
		return termimg.Sixel
	case config.PreviewHalfblocks:
		return termimg.Halfblocks
	default:
		return termimg.Auto
	}
}

var (
	cacheMu sync.Mutex
	cache   = map[string]string{} // key → rendered
)

func cacheKey(path string, w, h, frame int, proto termimg.Protocol) string {
	return fmt.Sprintf("%s|%d|%d|%d|%d", path, w, h, frame, proto)
}

// ClearCache drops rendered previews (call on theme/protocol change).
func ClearCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache = map[string]string{}
}

// RenderFile renders a still image (or GIF frame 0) as terminal graphics.
func RenderFile(path string, width, height int, proto termimg.Protocol) (string, error) {
	if width <= 0 {
		width = DefaultWidth
	}
	if height <= 0 {
		height = DefaultHeight
	}
	key := cacheKey(path, width, height, 0, proto)
	cacheMu.Lock()
	if s, ok := cache[key]; ok {
		cacheMu.Unlock()
		return s, nil
	}
	cacheMu.Unlock()

	img, err := loadStill(path)
	if err != nil {
		return "", err
	}
	img = fitImage(img, width*2, height*2) // rough pixel budget for protocols
	widget := termimg.NewImageWidgetFromImage(img).
		SetSize(width, height).
		SetProtocol(proto)
	out, err := widget.Render()
	if err != nil {
		// Last resort: halfblocks
		widget = termimg.NewImageWidgetFromImage(img).
			SetSize(width, height).
			SetProtocol(termimg.Halfblocks)
		out, err = widget.Render()
		if err != nil {
			return "", err
		}
	}
	cacheMu.Lock()
	cache[key] = out
	cacheMu.Unlock()
	return out, nil
}

// RenderGIFFrame renders one GIF frame (for animation). frame wraps by modulo.
func RenderGIFFrame(path string, width, height, frame int, proto termimg.Protocol) (string, error) {
	if width <= 0 {
		width = DefaultWidth
	}
	if height <= 0 {
		height = DefaultHeight
	}
	g, err := loadGIF(path)
	if err != nil {
		// Not a GIF — still image path
		return RenderFile(path, width, height, proto)
	}
	if len(g.Image) == 0 {
		return "", fmt.Errorf("gif vacío")
	}
	if frame < 0 {
		frame = 0
	}
	frame = frame % len(g.Image)
	key := cacheKey(path, width, height, frame, proto)
	cacheMu.Lock()
	if s, ok := cache[key]; ok {
		cacheMu.Unlock()
		return s, nil
	}
	cacheMu.Unlock()

	img := fitImage(g.Image[frame], width*2, height*2)
	widget := termimg.NewImageWidgetFromImage(img).
		SetSize(width, height).
		SetProtocol(proto)
	out, err := widget.Render()
	if err != nil {
		widget = termimg.NewImageWidgetFromImage(img).
			SetSize(width, height).
			SetProtocol(termimg.Halfblocks)
		out, err = widget.Render()
		if err != nil {
			return "", err
		}
	}
	cacheMu.Lock()
	cache[key] = out
	cacheMu.Unlock()
	return out, nil
}

// GIFFrameCount returns how many frames a GIF has (0 if not a GIF).
func GIFFrameCount(path string) int {
	g, err := loadGIF(path)
	if err != nil || g == nil {
		return 0
	}
	return len(g.Image)
}

func loadStill(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func loadGIF(path string) (*gif.GIF, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return gif.DecodeAll(f)
}

func fitImage(src image.Image, maxW, maxH int) image.Image {
	if src == nil || maxW <= 0 || maxH <= 0 {
		return src
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= maxW && sh <= maxH {
		return src
	}
	scale := float64(maxW) / float64(sw)
	if s2 := float64(maxH) / float64(sh); s2 < scale {
		scale = s2
	}
	dw := int(float64(sw) * scale)
	dh := int(float64(sh) * scale)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// WriteJPEGThumb writes raw JPEG bytes to path (0644 → 0600).
func WriteJPEGThumb(path string, jpegBytes []byte) error {
	if len(jpegBytes) == 0 {
		return fmt.Errorf("empty thumb")
	}
	// Validate it's an image.
	if _, _, err := image.Decode(bytes.NewReader(jpegBytes)); err != nil {
		return err
	}
	return os.WriteFile(path, jpegBytes, 0o600)
}

// FormatLinkCard builds the text block for a link embed (without image).
func FormatLinkCard(title, desc, url string, mutedStyle func(string) string) string {
	var b strings.Builder
	b.WriteString("│ ")
	if title != "" {
		b.WriteString(title)
	} else if url != "" {
		b.WriteString(url)
	} else {
		b.WriteString("enlace")
	}
	b.WriteString("\n")
	if desc != "" {
		d := desc
		if len([]rune(d)) > 120 {
			d = string([]rune(d)[:119]) + "…"
		}
		if mutedStyle != nil {
			b.WriteString("│ " + mutedStyle(d) + "\n")
		} else {
			b.WriteString("│ " + d + "\n")
		}
	}
	if url != "" && url != title {
		if mutedStyle != nil {
			b.WriteString("│ " + mutedStyle(url))
		} else {
			b.WriteString("│ " + url)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
