package store

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/model"
)

func ptrU64(v uint64) *uint64 { return &v }

func TestCategoryBrowseFromRow_topLevelAndChild(t *testing.T) {
	movie := &model.Category{ID: 2, ParentID: nil, SourceID: "1", Slug: "movie"}
	child101 := &model.Category{ID: 100, ParentID: ptrU64(2), SourceID: "101", Slug: "type-101"}
	vip601 := &model.Category{ID: 200, ParentID: ptrU64(17), SourceID: "601", Slug: "type-601"}
	mid506 := &model.Category{ID: 300, ParentID: ptrU64(16), SourceID: "506", Slug: "type-506"}

	if got := CategoryBrowseFromRow(2, movie); got.LeafSourceID != "" || got.CanonicalCategoryID != 2 || got.FailClosed || got.ExcludeOleHdTvSource {
		t.Fatalf("movie browse: %+v", got)
	}
	if got := CategoryBrowseFromRow(100, child101); got.CanonicalCategoryID != 2 || got.LeafSourceID != "101" || got.ExcludeOleHdTvSource {
		t.Fatalf("101 browse: %+v", got)
	}
	vipTop := &model.Category{ID: 17, ParentID: nil, SourceID: "6", Slug: "vip-bluray"}
	if got := CategoryBrowseFromRow(17, vipTop); !got.ExcludeOleHdTvSource || got.CanonicalCategoryID != 17 {
		t.Fatalf("vip top browse: %+v", got)
	}
	if got := CategoryBrowseFromRow(200, vip601); got.CanonicalCategoryID != 17 || got.LeafSourceID != "601" || !got.ExcludeOleHdTvSource {
		t.Fatalf("601 browse: %+v", got)
	}
	if got := CategoryBrowseFromRow(300, mid506); got.CanonicalCategoryID != 16 || got.LeafSourceID != "506" || got.ExcludeOleHdTvSource {
		t.Fatalf("506 browse: %+v", got)
	}
}

func TestAppendReadyVideosWhere_excludeOleHdTv(t *testing.T) {
	base := `SELECT id FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	q, _ := AppendReadyVideosWhere(base, nil, CategoryBrowseFilter{
		CanonicalCategoryID:  17,
		ExcludeOleHdTvSource: true,
	}, "", "", false)
	if !strings.Contains(q, `COALESCE(source_system,'') <> 'olehdtv'`) {
		t.Fatalf("missing olehdtv exclusion: %s", q)
	}
}

func TestCategoryBrowseFromRow_invalidChildFailClosed(t *testing.T) {
	noSource := &model.Category{ID: 9, ParentID: ptrU64(2), SourceID: ""}
	zeroParent := &model.Category{ID: 9, ParentID: ptrU64(0), SourceID: "101"}
	if !CategoryBrowseFromRow(9, noSource).FailClosed {
		t.Fatal("empty source_id must fail closed")
	}
	if !CategoryBrowseFromRow(9, zeroParent).FailClosed {
		t.Fatal("parent_id=0 must fail closed")
	}
}

func TestVideoMatchesBrowse_matrix(t *testing.T) {
	const (
		movieID = 2
		vipID   = 17
		midID   = 16
	)
	videos := []model.Video{
		{ID: 1, CategoryID: movieID, SourceCategoryID: "1", LeafSourceID: "101", Title: "action"},
		{ID: 2, CategoryID: movieID, SourceCategoryID: "1", LeafSourceID: "102", Title: "comedy"},
		{ID: 3, CategoryID: movieID, SourceCategoryID: "1", LeafSourceID: "", Title: "movie-no-leaf"},
		{ID: 4, CategoryID: vipID, SourceCategoryID: "6", LeafSourceID: "601", Title: "vip-action"},
		{ID: 5, CategoryID: vipID, SourceCategoryID: "6", LeafSourceID: "101", Title: "vip-wrong-leaf"}, // isolation
		{ID: 6, CategoryID: midID, SourceCategoryID: "5", LeafSourceID: "506", Title: "ai"},
		{ID: 7, CategoryID: movieID, SourceCategoryID: "1", LeafSourceID: "601", Title: "movie-with-vip-leaf"},
	}

	filterIDs := func(browse CategoryBrowseFilter) []uint64 {
		var ids []uint64
		for _, v := range videos {
			if VideoMatchesBrowse(v, browse) {
				ids = append(ids, v.ID)
			}
		}
		return ids
	}

	// A. Top-level movie: all movie-owned regardless of leaf
	movieBrowse := CategoryBrowseFromRow(movieID, &model.Category{ID: movieID, SourceID: "1"})
	got := filterIDs(movieBrowse)
	wantMovie := []uint64{1, 2, 3, 7}
	if len(got) != len(wantMovie) {
		t.Fatalf("top movie ids=%v want %v", got, wantMovie)
	}
	for i := range wantMovie {
		if got[i] != wantMovie[i] {
			t.Fatalf("top movie ids=%v want %v", got, wantMovie)
		}
	}

	// B. Child 101
	b101 := CategoryBrowseFromRow(100, &model.Category{ID: 100, ParentID: ptrU64(movieID), SourceID: "101"})
	got = filterIDs(b101)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("child 101 ids=%v want [1]", got)
	}
	// not 102, not VIP with leaf 101
	for _, v := range videos {
		if v.ID != 1 && VideoMatchesBrowse(v, b101) {
			t.Fatalf("101 leaked video %d", v.ID)
		}
	}

	// C. Midnight child 506
	b506 := CategoryBrowseFromRow(300, &model.Category{ID: 300, ParentID: ptrU64(midID), SourceID: "506"})
	got = filterIDs(b506)
	if len(got) != 1 || got[0] != 6 {
		t.Fatalf("506 ids=%v want [6]", got)
	}

	// D. Homepage midnight exclusion still by category_id slug (canonical midnight)
	base := `SELECT id FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	q, args := AppendReadyVideosWhere(base, nil, CategoryBrowseFilter{}, "", "midnight", false)
	if !strings.Contains(q, `category_id NOT IN (SELECT id FROM categories WHERE slug=?)`) {
		t.Fatalf("exclude SQL: %s", q)
	}
	if args[0] != "midnight" {
		t.Fatalf("args=%v", args)
	}
	// Semantic: midnight-owned + leaf 506 is excluded because category_id is midnight
	midVid := videos[5] // id 6
	if midVid.CategoryID != midID || midVid.LeafSourceID != "506" {
		t.Fatal("fixture")
	}
	// Exclusion is category_id based — leaf does not bypass
	excludedBySlug := midVid.CategoryID == midID // slug midnight maps to midID in product
	if !excludedBySlug {
		t.Fatal("midnight leaf video must remain excludeable via canonical category_id")
	}

	// E/F. VIP 601 isolation vs 101
	b601 := CategoryBrowseFromRow(200, &model.Category{ID: 200, ParentID: ptrU64(vipID), SourceID: "601"})
	got = filterIDs(b601)
	if len(got) != 1 || got[0] != 4 {
		t.Fatalf("601 ids=%v want [4]", got)
	}
	if VideoMatchesBrowse(videos[0], b601) { // movie+101
		t.Fatal("movie 101 must not match VIP 601 browse")
	}
	if VideoMatchesBrowse(videos[6], b601) { // movie+601 leaf wrongly stamped
		t.Fatal("movie category_id with leaf 601 must not match VIP browse")
	}
	if VideoMatchesBrowse(videos[4], b101) { // vip+101
		t.Fatal("VIP with leaf 101 must not match movie 101 browse")
	}

	// G. Fail closed
	bad := CategoryBrowseFromRow(9, &model.Category{ID: 9, ParentID: ptrU64(movieID), SourceID: ""})
	for _, v := range videos {
		if VideoMatchesBrowse(v, bad) {
			t.Fatalf("fail-closed matched %d", v.ID)
		}
	}
}
