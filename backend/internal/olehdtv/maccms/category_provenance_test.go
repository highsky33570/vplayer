package maccms_test

import (
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

func TestParseDetailPage_doesNotInventTypeIDFromNavigation(t *testing.T) {
	html := `
<html><body>
<ul class="nav">
  <a href="/index.php/vod/type/id/1.html">电影</a>
  <a href="/index.php/vod/type/id/5.html">午夜影院</a>
</ul>
<div class="myui-content__detail">
  <h1 class="title">日本少女初尝巨根</h1>
  <p>类型：未知</p>
</div>
<div class="playlist">
  <a href="/index.php/vod/play/id/85551/sid/1/nid/1.html">正片</a>
</div>
</body></html>`
	meta := maccms.ParseDetailPage(html, "https://www.olehdtv.com/index.php/vod/detail/id/85551.html")
	if meta.TypeID != "" {
		t.Fatalf("TypeID=%q; nav chrome / 类型：未知 must not invent type", meta.TypeID)
	}
}

func TestMergeDetailWithCatalog_listingTypeIDAuthoritative(t *testing.T) {
	detail := maccms.ParseDetailPage(`
<html><body>
<ul class="nav">
  <a href="/index.php/vod/type/id/1.html">电影</a>
  <a href="/index.php/vod/type/id/5.html">午夜影院</a>
</ul>
<div class="myui-content__detail"><h1 class="title">T</h1><p>类型：未知</p></div>
</body></html>`, "https://www.olehdtv.com/index.php/vod/detail/id/85551.html")
	if detail.TypeID != "" {
		t.Fatalf("precondition: detail TypeID=%q", detail.TypeID)
	}
	card := maccms.CatalogItem{VodID: "85551", Title: "T", TypeID: "5"}
	merged := maccms.MergeDetailWithCatalog(card, detail)
	if merged.TypeID != "5" {
		t.Fatalf("merged TypeID=%q want listing provenance 5", merged.TypeID)
	}
	// Contaminated detail TypeID must not override listing.
	detail.TypeID = "1"
	merged = maccms.MergeDetailWithCatalog(card, detail)
	if merged.TypeID != "5" {
		t.Fatalf("listing must win over detail chrome TypeID: got %q", merged.TypeID)
	}
}

func TestSelectInterleaved_midnightOutranksMovie(t *testing.T) {
	// Type 1 first in scan order (selected first), type 5 later — must upgrade to 5.
	perCat := [][]maccms.CatalogItem{
		{{VodID: "85551", Title: "X", TypeID: "1"}, {VodID: "101", Title: "A", TypeID: "1"}},
		{{VodID: "85551", Title: "X", TypeID: "5"}},
	}
	got := maccms.SelectInterleavedForTest(perCat, 10)
	var found *maccms.CatalogItem
	for i := range got {
		if got[i].VodID == "85551" {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatal("85551 not selected")
	}
	if found.TypeID != "5" {
		t.Fatalf("TypeID=%q want 5 after midnight listing", found.TypeID)
	}
}

func TestSelectInterleaved_midnightFirstThenMovieKeepsMidnight(t *testing.T) {
	perCat := [][]maccms.CatalogItem{
		{{VodID: "85551", Title: "X", TypeID: "5"}},
		{{VodID: "85551", Title: "X", TypeID: "1"}},
	}
	got := maccms.SelectInterleavedForTest(perCat, 5)
	if len(got) != 1 || got[0].TypeID != "5" {
		t.Fatalf("got=%+v", got)
	}
}

func TestSelectInterleaved_movieOnlyUnchanged(t *testing.T) {
	perCat := [][]maccms.CatalogItem{
		{{VodID: "101", Title: "A", TypeID: "1"}},
		{{VodID: "201", Title: "B", TypeID: "2"}},
	}
	got := maccms.SelectInterleavedForTest(perCat, 10)
	byID := map[string]string{}
	for _, c := range got {
		byID[c.VodID] = c.TypeID
	}
	if byID["101"] != "1" || byID["201"] != "2" {
		t.Fatalf("%v", byID)
	}
}

func TestSelectInterleaved_midnightUpgradeAfterLimitFilled(t *testing.T) {
	// Fill limit with type-1 cards including 85551; type-5 listing still upgrades.
	perCat := [][]maccms.CatalogItem{
		{
			{VodID: "85551", Title: "X", TypeID: "1"},
			{VodID: "102", Title: "A", TypeID: "1"},
		},
		{
			{VodID: "201", Title: "B", TypeID: "2"},
			{VodID: "85551", Title: "X", TypeID: "5"},
		},
	}
	got := maccms.SelectInterleavedForTest(perCat, 2)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	for _, c := range got {
		if c.VodID == "85551" && c.TypeID != "5" {
			t.Fatalf("limit-filled selection must still upgrade TypeID: %+v", got)
		}
	}
}

func TestPreferListingTypeID(t *testing.T) {
	if !maccms.PreferListingTypeID("5", "1") {
		t.Fatal("midnight should outrank movie")
	}
	if maccms.PreferListingTypeID("1", "5") {
		t.Fatal("movie must not outrank midnight")
	}
	if maccms.PreferListingTypeID("1", "2") {
		t.Fatal("equal-rank general types: keep current")
	}
}
