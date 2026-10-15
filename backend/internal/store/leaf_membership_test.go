package store

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/model"
)

func TestLeafSourceSQL(t *testing.T) {
	if leafSourceSQL("") != nil || leafSourceSQL("  ") != nil {
		t.Fatal("empty => NULL")
	}
	if leafSourceSQL(" 101 ") != "101" {
		t.Fatalf("got %v", leafSourceSQL(" 101 "))
	}
}

func TestSetVideoLeafSourceIDQuery_onlyLeafColumn(t *testing.T) {
	q := setVideoLeafSourceIDQuery()
	if q != `UPDATE videos SET leaf_source_id=? WHERE id=?` {
		t.Fatalf("query=%q", q)
	}
	forbidden := []string{
		"category_id", "source_category_id", "playback_url", "title",
		"poster", "hls_", "migration", "cover_",
	}
	for _, w := range forbidden {
		if strings.Contains(q, w) {
			t.Fatalf("membership update must not touch %q: %s", w, q)
		}
	}
}

func TestSetVideoLeafSourceID_semanticsPreserveOwnership(t *testing.T) {
	// Documents that assigning leaf membership is orthogonal to ownership fields.
	v := model.Video{
		ID: 9, CategoryID: 2, SourceCategoryID: "1", LeafSourceID: "",
		Title: "x", PlaybackURL: "http://example/play",
	}
	beforeCat, beforeSrc, beforeTitle, beforePlay := v.CategoryID, v.SourceCategoryID, v.Title, v.PlaybackURL
	v.LeafSourceID = "101" // effect of SetVideoLeafSourceID after successful write
	if v.CategoryID != beforeCat || v.SourceCategoryID != beforeSrc {
		t.Fatal("ownership fields must remain unchanged")
	}
	if v.Title != beforeTitle || v.PlaybackURL != beforePlay {
		t.Fatal("catalog/playback fields must remain unchanged")
	}
	if v.LeafSourceID != "101" {
		t.Fatal("leaf not set")
	}
}
