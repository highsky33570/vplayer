package maccms_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

func mkdirWrite(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestSiteOneArchiveIndexFixtureLayout(t *testing.T) {
	root := fixtureRoot(t)
	idx, err := maccms.BuildArchiveIndex(root, "https://maccms.local")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Files < 10 {
		t.Fatalf("files=%d", idx.Files)
	}
	rel, ok := idx.Resolve("https://maccms.local/index.php/vod/detail/id/10001.html")
	if !ok || rel == "" {
		t.Fatalf("resolve detail failed: %v %q", ok, rel)
	}
	f, err := maccms.NewSiteOneFetcher(root, "https://maccms.local")
	if err != nil {
		t.Fatal(err)
	}
	body, err := f.Fetch(context.Background(), "https://maccms.local/index.php/vod/type/id/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 50 {
		t.Fatal("empty body")
	}
	_ = filepath.Base(rel)
}

func TestSiteOnePreserveURLStructureMapping(t *testing.T) {
	dir := t.TempDir()
	// Simulate SiteOne --offline-export-preserve-url-structure file
	p := filepath.Join(dir, "index.php", "vod", "detail", "id", "555.html")
	if err := mkdirWrite(p, "<html><h1>X</h1></html>"); err != nil {
		t.Fatal(err)
	}
	f, err := maccms.NewSiteOneFetcher(dir, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	body, err := f.Fetch(context.Background(), "https://example.test/index.php/vod/detail/id/555.html")
	if err != nil {
		t.Fatal(err)
	}
	if body == "" {
		t.Fatal("empty")
	}
}
