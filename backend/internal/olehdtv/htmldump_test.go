package olehdtv_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv"
)

func TestHTMLDumpAdapterDiscoversCatalog(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "maccms_html")
	ad, err := olehdtv.NewHTMLDumpAdapter(root, "https://maccms.local")
	if err != nil {
		t.Fatal(err)
	}
	cats, err := ad.ListCategories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) < 3 {
		t.Fatalf("cats=%d", len(cats))
	}
	vids, err := ad.ListVideos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(vids) < 10 {
		t.Fatalf("videos=%d", len(vids))
	}
	seen := map[string]bool{}
	for _, v := range vids {
		if seen[v.SourceID] {
			t.Fatalf("duplicate %s", v.SourceID)
		}
		seen[v.SourceID] = true
	}
}

func TestNewAdapterHTMLDumpMode(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "maccms_html")
	ad, err := olehdtv.NewAdapter("html_dump", "", "", "", root, "https://maccms.local")
	if err != nil {
		t.Fatal(err)
	}
	if ad.Name() == "" {
		t.Fatal("empty name")
	}
}
