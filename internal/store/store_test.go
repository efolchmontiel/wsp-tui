package store_test

import (
	"path/filepath"
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestOpenMigratesSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "whatstui.db")

	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	var version string
	err = s.DB().QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	if err != nil {
		t.Fatalf("schema_version: %v", err)
	}
	if version != "1" && version != "2" {
		t.Fatalf("want schema 1 or 2, got %q", version)
	}
}
