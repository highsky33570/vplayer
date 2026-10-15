package olehdtv

import (
	"testing"
)

func TestMergeVerifiedHierarchy_parentsAndSlugs(t *testing.T) {
	tops := []SourceCategory{
		{SourceID: "1", Name: "电影", Slug: "movie", Sort: 10},
		{SourceID: "2", Name: "连续剧", Slug: "tv", Sort: 20},
		{SourceID: "4", Name: "动漫", Slug: "anime", Sort: 40},
		{SourceID: "5", Name: "午夜影院", Slug: "midnight", Sort: 50},
		{SourceID: "6", Name: "VIP蓝光影院", Slug: "vip-bluray", Sort: 60},
	}
	merged := MergeVerifiedHierarchy(tops)
	byID := map[string]SourceCategory{}
	for _, c := range merged {
		byID[c.SourceID] = c
	}

	checks := []struct {
		child, parent, slug, name string
	}{
		{"101", "1", "type-101", "动作片"},
		{"202", "2", "type-202", "国产剧"},
		{"401", "4", "type-401", "日本"},
		{"501", "5", "type-501", "欧美"},
		{"506", "5", "type-506", "AI真人"},
		{"601", "6", "type-601", "动作片"},
		{"613", "6", "type-613", "犯罪片"},
	}
	for _, tc := range checks {
		c, ok := byID[tc.child]
		if !ok {
			t.Fatalf("missing child %s", tc.child)
		}
		if c.ParentSourceID != tc.parent {
			t.Fatalf("%s ParentSourceID=%q want %q", tc.child, c.ParentSourceID, tc.parent)
		}
		if c.Slug != tc.slug {
			t.Fatalf("%s Slug=%q want %q", tc.child, c.Slug, tc.slug)
		}
		if c.Name != tc.name {
			t.Fatalf("%s Name=%q want %q", tc.child, c.Name, tc.name)
		}
	}

	if byID["101"].Name != byID["601"].Name {
		t.Fatal("101 and 601 should share label 动作片")
	}
	if byID["101"].SourceID == byID["601"].SourceID || byID["101"].Slug == byID["601"].Slug {
		t.Fatal("101 and 601 must remain distinct ids/slugs")
	}
	if byID["101"].ParentSourceID != "1" || byID["601"].ParentSourceID != "6" {
		t.Fatal("101/601 parent isolation failed")
	}
	if byID["501"].ParentSourceID != "5" || byID["506"].ParentSourceID != "5" {
		t.Fatal("midnight children must parent to 5")
	}
}

func TestMergeVerifiedHierarchy_skipsMissingParentsAndNonCanonical(t *testing.T) {
	// Only movie present — no tv/vip children; no inventing 7/8/13/14.
	merged := MergeVerifiedHierarchy([]SourceCategory{
		{SourceID: "1", Name: "电影", Slug: "movie", Sort: 10},
	})
	byID := map[string]SourceCategory{}
	for _, c := range merged {
		byID[c.SourceID] = c
	}
	if _, ok := byID["101"]; !ok {
		t.Fatal("expected movie children")
	}
	if _, ok := byID["202"]; ok {
		t.Fatal("must not add tv children without parent 2")
	}
	if _, ok := byID["7"]; ok {
		t.Fatal("must not invent source 7")
	}
	if _, ok := byID["13"]; ok {
		t.Fatal("must not invent source 13")
	}
}

func TestMergeVerifiedHierarchy_sortOrderStable(t *testing.T) {
	merged := MergeVerifiedHierarchy([]SourceCategory{
		{SourceID: "1", Name: "电影", Slug: "movie", Sort: 10},
	})
	var kids []SourceCategory
	for _, c := range merged {
		if c.ParentSourceID == "1" {
			kids = append(kids, c)
		}
	}
	if len(kids) != 13 {
		t.Fatalf("movie children=%d want 13", len(kids))
	}
	want := []string{"101", "102", "103", "104", "105", "106", "107", "108", "109", "110", "111", "112", "113"}
	for i, id := range want {
		if kids[i].SourceID != id {
			t.Fatalf("order[%d]=%s want %s", i, kids[i].SourceID, id)
		}
		if kids[i].Sort != 10+i+1 {
			t.Fatalf("sort[%d]=%d want %d", i, kids[i].Sort, 10+i+1)
		}
	}
}

func TestChildCategorySlug(t *testing.T) {
	if got := ChildCategorySlug("101"); got != "type-101" {
		t.Fatalf("got %q", got)
	}
	if got := ChildCategorySlug("1207"); got != "type-1207" {
		t.Fatalf("got %q", got)
	}
}

func TestIsCanonicalHierarchyParentSourceID(t *testing.T) {
	for _, id := range []string{"1", "2", "3", "4", "5", "6"} {
		if !IsCanonicalHierarchyParentSourceID(id) {
			t.Fatalf("%s should be canonical", id)
		}
	}
	for _, id := range []string{"7", "8", "13", "14", "101", ""} {
		if IsCanonicalHierarchyParentSourceID(id) {
			t.Fatalf("%s must not be canonical parent", id)
		}
	}
}

func TestValidateChildParentRef(t *testing.T) {
	if err := ValidateChildParentRef("101", "1"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChildParentRef("601", "1"); err == nil {
		t.Fatal("601 must not validate under movie")
	}
	if err := ValidateChildParentRef("501", "5"); err != nil {
		t.Fatal(err)
	}
}
