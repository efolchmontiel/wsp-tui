package paths_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/paths"
)

func TestResolveUsesXDG(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	p, err := paths.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if p.SessionDB != filepath.Join(root, "data", "whatstui", "session.db") {
		t.Fatalf("session db path: %s", p.SessionDB)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "whatstui", "media", "images")); err != nil {
		t.Fatalf("media dir: %v", err)
	}
}
