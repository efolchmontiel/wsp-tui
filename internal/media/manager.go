package media

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/efolchmontiel/wsp-tui/internal/paths"
	"github.com/efolchmontiel/wsp-tui/internal/store"
	"go.mau.fi/whatsmeow"
)

// Manager handles local media files (never blocks the UI by itself — callers run async).
type Manager struct {
	paths  paths.Paths
	store  *store.Store
	client func() *whatsmeow.Client
}

// New creates a media manager.
func New(p paths.Paths, st *store.Store, clientFn func() *whatsmeow.Client) *Manager {
	return &Manager{paths: p, store: st, client: clientFn}
}

// AllocPath returns a unique path under the right media subdirectory.
func (m *Manager) AllocPath(kind, fileName string) (string, error) {
	safe := sanitizeFileName(fileName)
	dir := filepath.Join(m.paths.MediaDir, Subdir(kind))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := fmt.Sprintf("%d_%s", time.Now().UnixNano(), safe)
	return filepath.Join(dir, base), nil
}

func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "file.bin"
	}
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		default:
			return r
		}
	}, name)
	return name
}

// DownloadRefToFile downloads using a stored ref into destPath.
func (m *Manager) DownloadRefToFile(ctx context.Context, ref DownloadRef, destPath string) error {
	client := m.client()
	if client == nil {
		return fmt.Errorf("client not ready")
	}
	dl, err := ref.ToDownloadable()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return client.DownloadToFile(ctx, dl, f)
}

// OpenExternal opens a local file with mpv (av) or xdg-open (docs/images).
// For audio/video it returns the running mpv process so the UI can animate.
func OpenExternal(path, kind string) (*exec.Cmd, error) {
	if path == "" {
		return nil, fmt.Errorf("no local file")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	switch kind {
	case "audio", "video":
		if _, err := exec.LookPath("mpv"); err != nil {
			return nil, fmt.Errorf("mpv no está instalado — instalalo para reproducir audio/video")
		}
		cmd := exec.Command("mpv", "--no-terminal", "--force-window=no", "--", path)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return nil, fmt.Errorf("xdg-open no disponible")
		}
		cmd := exec.Command("xdg-open", path)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
}

// Waveframe returns a braille/block waveform frame for the visualizer.
func Waveframe(t float64, width int) string {
	if width < 8 {
		width = 8
	}
	if width > 48 {
		width = 48
	}
	bars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var b strings.Builder
	for i := 0; i < width; i++ {
		// Layered sines → cava-ish motion without capturing system audio.
		v := 0.5 + 0.35*sinApprox(t*6+float64(i)*0.45) + 0.2*sinApprox(t*11+float64(i)*0.9)
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		idx := int(v * float64(len(bars)-1))
		b.WriteRune(bars[idx])
	}
	return b.String()
}

func sinApprox(x float64) float64 {
	// Enough for UI animation; avoids importing math in hot path concerns.
	_, frac := math.Modf(x / (2 * math.Pi))
	if frac < 0 {
		frac += 1
	}
	ang := frac * 2 * math.Pi
	if ang > math.Pi {
		ang -= 2 * math.Pi
	}
	return math.Sin(ang)
}

// FormatSize humanizes byte sizes.
func FormatSize(n uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
