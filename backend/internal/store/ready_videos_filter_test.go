package store

import (
	"strings"
	"testing"
)

func TestNormalizeExcludeCategorySlug(t *testing.T) {
	if got := NormalizeExcludeCategorySlug("  midnight  "); got != "midnight" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeExcludeCategorySlug(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := NormalizeExcludeCategorySlug("   "); got != "" {
		t.Fatalf("ws: %q", got)
	}
	long := strings.Repeat("a", maxCategorySlugLen+1)
	if got := NormalizeExcludeCategorySlug(long); got != "" {
		t.Fatalf("overlong must fail closed: %q", got)
	}
	ok := strings.Repeat("b", maxCategorySlugLen)
	if got := NormalizeExcludeCategorySlug(ok); got != ok {
		t.Fatalf("max len: %q", got)
	}
}

func TestReadyVideoOrderSQL(t *testing.T) {
	if got := ReadyVideoOrderSQL("hot"); got != ` ORDER BY view_count DESC, id DESC` {
		t.Fatalf("hot order: %q", got)
	}
	if got := ReadyVideoOrderSQL(""); !strings.Contains(got, "created_at DESC") {
		t.Fatalf("default order: %q", got)
	}
	if got := ReadyVideoOrderSQL("nope"); !strings.Contains(got, "created_at DESC") {
		t.Fatalf("unknown sort must stay newest: %q", got)
	}
}

func TestAppendReadyVideosWhere_excludeBeforeLimit(t *testing.T) {
	base := `SELECT id FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	q, args := AppendReadyVideosWhere(base, nil, CategoryBrowseFilter{}, "", "midnight", false)
	q += ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, 12, 0)

	if !strings.Contains(q, `category_id NOT IN (SELECT id FROM categories WHERE slug=?)`) {
		t.Fatalf("missing slug exclusion: %s", q)
	}
	if strings.Contains(q, "午夜") {
		t.Fatal("must not filter by Chinese category name")
	}
	exclIdx := strings.Index(q, "NOT IN (SELECT id FROM categories WHERE slug=?)")
	orderIdx := strings.Index(q, "ORDER BY")
	limitIdx := strings.Index(q, "LIMIT")
	if exclIdx < 0 || orderIdx < 0 || limitIdx < 0 || !(exclIdx < orderIdx && orderIdx < limitIdx) {
		t.Fatalf("exclusion must precede ORDER BY/LIMIT: %s", q)
	}
	if len(args) != 3 || args[0] != "midnight" || args[1] != 12 || args[2] != 0 {
		t.Fatalf("args=%v", args)
	}
	if strings.Contains(q, "'midnight'") {
		t.Fatal("slug must not be interpolated into SQL")
	}
}

func TestAppendReadyVideosWhere_noExcludeByDefault(t *testing.T) {
	base := `SELECT COUNT(*) FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	q, args := AppendReadyVideosWhere(base, nil, CategoryBrowseFilter{}, "", "", false)
	if strings.Contains(q, "NOT IN") || strings.Contains(q, "categories") {
		t.Fatalf("empty exclude must not alter SQL: %s", q)
	}
	if len(args) != 0 {
		t.Fatalf("args=%v", args)
	}
}

func TestAppendReadyVideosWhere_categoryAndSearchUnchanged(t *testing.T) {
	base := `SELECT id FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	browse := CategoryBrowseFilter{CanonicalCategoryID: 16}
	q, args := AppendReadyVideosWhere(base, nil, browse, "foo", "", false)
	if !strings.Contains(q, "category_id=?") || !strings.Contains(q, "title LIKE ?") {
		t.Fatalf("%s", q)
	}
	if strings.Contains(q, "leaf_source_id") {
		t.Fatal("top-level must not filter leaf_source_id")
	}
	if strings.Contains(q, "NOT IN") {
		t.Fatal("search/category without exclude must not exclude midnight")
	}
	if len(args) != 4 || args[0] != uint64(16) {
		t.Fatalf("args=%v", args)
	}
}

func TestAppendReadyVideosWhere_countSharesExclusion(t *testing.T) {
	base := `SELECT COUNT(*) FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	q, args := AppendReadyVideosWhere(base, nil, CategoryBrowseFilter{}, "", "midnight", false)
	if !strings.Contains(q, `category_id NOT IN (SELECT id FROM categories WHERE slug=?)`) {
		t.Fatalf("%s", q)
	}
	if len(args) != 1 || args[0] != "midnight" {
		t.Fatalf("args=%v", args)
	}
}

func TestAppendReadyVideosWhere_childLeafAndParent(t *testing.T) {
	base := `SELECT id FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	browse := CategoryBrowseFilter{CanonicalCategoryID: 2, LeafSourceID: "101"}
	q, args := AppendReadyVideosWhere(base, nil, browse, "", "", false)
	if !strings.Contains(q, `category_id=? AND leaf_source_id=?`) {
		t.Fatalf("%s", q)
	}
	if len(args) != 2 || args[0] != uint64(2) || args[1] != "101" {
		t.Fatalf("args=%v", args)
	}
}

func TestAppendReadyVideosWhere_failClosed(t *testing.T) {
	base := `SELECT id FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	q, args := AppendReadyVideosWhere(base, nil, CategoryBrowseFilter{FailClosed: true}, "", "", false)
	if !strings.Contains(q, "AND 1=0") {
		t.Fatalf("%s", q)
	}
	if strings.Contains(q, "category_id=?") || strings.Contains(q, "leaf_source_id") {
		t.Fatal("fail-closed must not broaden to category/leaf match")
	}
	if len(args) != 0 {
		t.Fatalf("args=%v", args)
	}
}
