package preview

import (
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/blacktop/go-termimg"
	"github.com/efolchmontiel/wsp-tui/internal/config"
)

func TestProtocolFromConfig(t *testing.T) {
	if ProtocolFromConfig(config.PreviewKitty) != termimg.Kitty {
		t.Fatal("kitty")
	}
	if ProtocolFromConfig(config.PreviewAuto) != termimg.Auto {
		t.Fatal("auto")
	}
}

func TestRenderFileHalfblocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.png")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	out, err := RenderFile(path, 10, 5, termimg.Halfblocks)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty render")
	}
}

func TestGIFFrameCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.gif")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	pal := color.Palette{color.Black, color.White}
	g := &gif.GIF{
		Image: []*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 4, 4), pal),
			image.NewPaletted(image.Rect(0, 0, 4, 4), pal),
		},
		Delay: []int{10, 10},
	}
	if err := gif.EncodeAll(f, g); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if n := GIFFrameCount(path); n != 2 {
		t.Fatalf("frames %d", n)
	}
}
