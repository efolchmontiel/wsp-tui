package preview

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"sync"

	"github.com/blacktop/go-termimg"
	"github.com/charmbracelet/x/mosaic"
	"github.com/efolchmontiel/wsp-tui/internal/config"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	DefaultWidth  = 56
	DefaultHeight = 22
	PickerWidth   = 48
	PickerHeight  = 18
)

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

func Halfblocks() termimg.Protocol {
	return termimg.Halfblocks
}

var (
	detectOnce sync.Once
	detected   termimg.Protocol = termimg.Halfblocks

	cacheMu sync.Mutex
	cache   = map[string]string{}

	gifMu    sync.Mutex
	gifCache = map[string]*gif.GIF{}
)

// TUIProtocol picks Kitty when supported, else mosaic.
func TUIProtocol(cfg config.PreviewProtocol) termimg.Protocol {
	switch config.ParsePreviewProtocol(string(cfg)) {
	case config.PreviewHalfblocks:
		return termimg.Halfblocks
	case config.PreviewKitty:
		return termimg.Kitty
	case config.PreviewITerm2:
		return termimg.ITerm2
	case config.PreviewSixel:
		return termimg.Sixel
	default:
		detectOnce.Do(func() {
			if termimg.KittySupported() {
				detected = termimg.Kitty
			} else {
				detected = termimg.Halfblocks
			}
		})
		return detected
	}
}

func IsNative(proto termimg.Protocol) bool {
	return proto == termimg.Kitty || proto == termimg.ITerm2 || proto == termimg.Sixel
}

func cacheKey(path string, w, h, frame int, proto termimg.Protocol) string {
	return fmt.Sprintf("%s|%d|%d|%d|%d", path, w, h, frame, proto)
}

func ClearCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache = map[string]string{}
	gifMu.Lock()
	defer gifMu.Unlock()
	gifCache = map[string]*gif.GIF{}
}

func ClearGraphics() {
	ClearCache()
	_ = termimg.ClearAll()
}

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
	out, err := renderImage(img, width, height, proto)
	if err != nil {
		return "", err
	}
	cacheMu.Lock()
	cache[key] = out
	cacheMu.Unlock()
	return out, nil
}

func RenderGIFFrame(path string, width, height, frame int, proto termimg.Protocol) (string, error) {
	if width <= 0 {
		width = DefaultWidth
	}
	if height <= 0 {
		height = DefaultHeight
	}
	g, err := loadGIFCached(path)
	if err != nil {
		return RenderFile(path, width, height, proto)
	}
	if len(g.Image) == 0 {
		return "", fmt.Errorf("gif vacío")
	}
	// Native graphics: still frame only.
	if IsNative(proto) {
		frame = 0
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

	img := compositeGIFFrame(g, frame)
	out, err := renderImage(img, width, height, proto)
	if err != nil {
		return "", err
	}
	cacheMu.Lock()
	cache[key] = out
	cacheMu.Unlock()
	return out, nil
}

func renderImage(img image.Image, width, height int, proto termimg.Protocol) (string, error) {
	if img == nil {
		return "", fmt.Errorf("imagen vacía")
	}
	switch proto {
	case termimg.Kitty:
		ti := termimg.New(img).
			Protocol(termimg.Kitty).
			UseUnicode(true).
			Width(width).
			Height(height).
			Scale(termimg.ScaleFit)
		out, err := ti.Render()
		if err == nil && hasVisibleCells(out) {
			return out, nil
		}
	case termimg.ITerm2, termimg.Sixel:
		widget := termimg.NewImageWidgetFromImage(img).
			SetSize(width, height).
			SetProtocol(proto)
		out, err := widget.Render()
		if err == nil && hasVisibleCells(out) {
			return out, nil
		}
	}

	m := mosaic.New().
		Width(width).
		Height(height).
		Symbol(mosaic.All).
		Dither(true)
	out := m.Render(img)
	if !hasVisibleCells(out) {
		return "", fmt.Errorf("render vacío")
	}
	return out, nil
}

// hasVisibleCells reports printable glyphs after stripping escapes.
func hasVisibleCells(s string) bool {
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 0x40 && r <= 0x7e) || r == '\a' {
				inEsc = false
			}
			continue
		}
		if r > ' ' && r != '\u00a0' {
			return true
		}
	}
	return false
}

func GIFFrameCount(path string) int {
	g, err := loadGIFCached(path)
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

func loadGIFCached(path string) (*gif.GIF, error) {
	gifMu.Lock()
	if g, ok := gifCache[path]; ok {
		gifMu.Unlock()
		return g, nil
	}
	gifMu.Unlock()

	g, err := loadGIF(path)
	if err != nil {
		return nil, err
	}
	gifMu.Lock()
	gifCache[path] = g
	gifMu.Unlock()
	return g, nil
}

func loadGIF(path string) (*gif.GIF, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return gif.DecodeAll(f)
}

func compositeGIFFrame(g *gif.GIF, frame int) image.Image {
	if g == nil || len(g.Image) == 0 {
		return nil
	}
	if frame < 0 {
		frame = 0
	}
	if frame >= len(g.Image) {
		frame = len(g.Image) - 1
	}

	w, h := g.Config.Width, g.Config.Height
	if w <= 0 || h <= 0 {
		b := g.Image[0].Bounds()
		w, h = b.Max.X, b.Max.Y
	}
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	var backup *image.RGBA

	bg := color.RGBA{}
	if pal, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(pal) {
		if c, ok := pal[g.BackgroundIndex].(color.RGBA); ok {
			bg = c
		} else {
			r, g2, b, a := pal[g.BackgroundIndex].RGBA()
			bg = color.RGBA{uint8(r >> 8), uint8(g2 >> 8), uint8(b >> 8), uint8(a >> 8)}
		}
	}

	for i := 0; i <= frame; i++ {
		fr := g.Image[i]
		fb := fr.Bounds()

		disposal := byte(0)
		if i > 0 && g.Disposal != nil && i-1 < len(g.Disposal) {
			disposal = g.Disposal[i-1]
		}
		if i > 0 {
			prev := g.Image[i-1]
			pb := prev.Bounds()
			switch disposal {
			case gif.DisposalBackground:
				draw.Draw(canvas, pb, &image.Uniform{bg}, image.Point{}, draw.Src)
			case gif.DisposalPrevious:
				if backup != nil {
					draw.Draw(canvas, canvas.Bounds(), backup, image.Point{}, draw.Src)
				}
			}
		}

		if g.Disposal != nil && i < len(g.Disposal) && g.Disposal[i] == gif.DisposalPrevious {
			if backup == nil {
				backup = image.NewRGBA(canvas.Bounds())
			}
			draw.Draw(backup, canvas.Bounds(), canvas, image.Point{}, draw.Src)
		}

		draw.Draw(canvas, fb, fr, fb.Min, draw.Over)
	}
	return canvas
}

func WriteJPEGThumb(path string, jpegBytes []byte) error {
	if len(jpegBytes) == 0 {
		return fmt.Errorf("empty thumb")
	}
	if _, _, err := image.Decode(bytes.NewReader(jpegBytes)); err != nil {
		return err
	}
	return os.WriteFile(path, jpegBytes, 0o600)
}

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
