package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsGIFPath(t *testing.T) {
	if !IsGIFPath("foo.gif") {
		t.Fatal("expected .gif")
	}
	if IsGIFPath("foo.mp4") {
		t.Fatal("mp4 is not gif")
	}
}

func TestConvertGIFToMP4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "tiny.gif")
	gen := exec.Command(
		"ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:d=0.2",
		src,
	)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate gif: %v (%s)", err, out)
	}
	dst := filepath.Join(dir, "out.mp4")
	if err := ConvertGIFToMP4(src, dst); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dst)
	if err != nil || st.Size() == 0 {
		t.Fatalf("mp4 missing or empty: %v", err)
	}
}
