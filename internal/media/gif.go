package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func IsGIFPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".gif" {
		return true
	}
	return DetectMIME(path) == "image/gif"
}

func ConvertGIFToMP4(src, dst string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg no está instalado — hace falta para enviar GIF")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	cmd := exec.Command(
		"ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", src,
		"-movflags", "faststart",
		"-pix_fmt", "yuv420p",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-an",
		dst,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("convertir GIF: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func JPEGThumbFromMedia(src, dst string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return err
	}
	cmd := exec.Command(
		"ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", src,
		"-vframes", "1",
		"-q:v", "4",
		dst,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("miniatura: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
