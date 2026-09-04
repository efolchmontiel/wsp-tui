package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeReactionIntoMetadataPreservesMedia(t *testing.T) {
	base := `{"media":{"kind":"image","direct_path":"/x","media_key":"YQ==","file_sha256":"YQ==","file_enc_sha256":"YQ==","mime":"image/jpeg","file_name":"a.jpg","file_length":1}}`
	got, err := MergeReactionIntoMetadata(base, "s1", "😂")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"kind":"image"`, `"emoji":"😂"`, `"sender":"s1"`} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %s in %s", part, got)
		}
	}
	got, err = MergeReactionIntoMetadata(got, "s1", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `"emoji"`) {
		t.Fatalf("expected reaction removed: %s", got)
	}
	if !strings.Contains(got, `"kind":"image"`) {
		t.Fatalf("media lost: %s", got)
	}
}

func TestFormatReactions(t *testing.T) {
	meta, err := MergeReactionIntoMetadata("{}", "a", "😂")
	if err != nil {
		t.Fatal(err)
	}
	meta, err = MergeReactionIntoMetadata(meta, "b", "😂")
	if err != nil {
		t.Fatal(err)
	}
	meta, err = MergeReactionIntoMetadata(meta, "c", "👍")
	if err != nil {
		t.Fatal(err)
	}
	got := FormatReactions(meta)
	if got != "😂×2 👍" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyReaction(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	if err := s.UpsertMessage(ctx, Message{
		ID: "m1", ChatID: "c1", Text: "hola", Type: TypeText, Status: StatusReceived,
		MetadataJSON: "{}",
	}); err != nil {
		t.Fatal(err)
	}
	updated, ok, err := s.ApplyReaction(ctx, "c1", "m1", "peer@s.whatsapp.net", "😂")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if FormatReactions(updated.MetadataJSON) != "😂" {
		t.Fatalf("meta %s", updated.MetadataJSON)
	}
	_, ok, err = s.ApplyReaction(ctx, "c1", "missing", "peer@s.whatsapp.net", "😂")
	if err != nil || ok {
		t.Fatalf("missing target should soft-skip ok=%v err=%v", ok, err)
	}
}
