package catalogsync

import (
	"log/slog"
	"testing"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

type hierStore struct {
	bySource map[string]*model.Category
	bySlug   map[string]*model.Category
	nextID   uint64
}

func newHierStore() *hierStore {
	return &hierStore{
		bySource: map[string]*model.Category{},
		bySlug:   map[string]*model.Category{},
		nextID:   1,
	}
}

func (h *hierStore) seed(c model.Category) {
	cp := c
	h.bySource[c.SourceID] = &cp
	h.bySlug[c.Slug] = &cp
	if c.ID >= h.nextID {
		h.nextID = c.ID + 1
	}
}

func (h *hierStore) GetCategoryBySource(system, sourceID string) (*model.Category, error) {
	_ = system
	c := h.bySource[sourceID]
	if c == nil {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (h *hierStore) UpsertCategoryResult(in store.CategoryUpsert) (uint64, bool, bool, error) {
	if existing := h.bySource[in.SourceID]; existing != nil {
		changed := existing.Name != in.Name || existing.Sort != in.Sort || existing.Slug != in.Slug ||
			!parentPtrEqual(existing.ParentID, in.ParentID) || existing.IsActive != in.IsActive
		existing.Name = in.Name
		existing.Sort = in.Sort
		existing.Slug = in.Slug
		existing.ParentID = cloneParent(in.ParentID)
		existing.IsActive = in.IsActive
		return existing.ID, false, changed, nil
	}
	id := h.nextID
	h.nextID++
	row := &model.Category{
		ID: id, SourceSystem: in.SourceSystem, SourceID: in.SourceID,
		Name: in.Name, Slug: in.Slug, Sort: in.Sort, IsActive: in.IsActive,
		ParentID: cloneParent(in.ParentID),
	}
	h.bySource[in.SourceID] = row
	h.bySlug[in.Slug] = row
	return id, true, false, nil
}

func cloneParent(p *uint64) *uint64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func parentPtrEqual(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func TestUpsertCategoriesTwoPhase_parentResolutionAndIsolation(t *testing.T) {
	st := newHierStore()
	// Seed canonical tops with fixed ids like production seeds.
	st.seed(model.Category{ID: 2, SourceID: "1", Name: "电影", Slug: "movie", Sort: 10, IsActive: true})
	st.seed(model.Category{ID: 3, SourceID: "2", Name: "连续剧", Slug: "tv", Sort: 20, IsActive: true})
	st.seed(model.Category{ID: 4, SourceID: "4", Name: "动漫", Slug: "anime", Sort: 40, IsActive: true})
	st.seed(model.Category{ID: 16, SourceID: "5", Name: "午夜影院", Slug: "midnight", Sort: 50, IsActive: true})
	st.seed(model.Category{ID: 17, SourceID: "6", Name: "VIP蓝光影院", Slug: "vip-bluray", Sort: 60, IsActive: true})

	tops := []olehdtv.SourceCategory{
		{SourceID: "1", Name: "电影", Slug: "movie", Sort: 10},
		{SourceID: "2", Name: "连续剧", Slug: "tv", Sort: 20},
		{SourceID: "3", Name: "综艺", Slug: "variety", Sort: 30},
		{SourceID: "4", Name: "动漫", Slug: "anime", Sort: 40},
		{SourceID: "5", Name: "午夜影院", Slug: "midnight", Sort: 50},
		{SourceID: "6", Name: "VIP蓝光影院", Slug: "vip-bluray", Sort: 60},
	}
	catMap, stats, err := UpsertCategoriesTwoPhase(st, tops, false, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 0 {
		t.Fatalf("failed=%d", stats.Failed)
	}

	assertParent := func(childSrc string, wantParentLocal uint64, wantParentSrc string) {
		t.Helper()
		c := st.bySource[childSrc]
		if c == nil {
			t.Fatalf("missing child %s", childSrc)
		}
		if c.ParentID == nil || *c.ParentID != wantParentLocal {
			t.Fatalf("%s parent_id=%v want %d", childSrc, c.ParentID, wantParentLocal)
		}
		if c.Slug != "type-"+childSrc {
			t.Fatalf("%s slug=%q", childSrc, c.Slug)
		}
		if got := catMap[childSrc]; got != c.ID {
			t.Fatalf("catMap[%s]=%d want %d", childSrc, got, c.ID)
		}
		_ = wantParentSrc
	}

	assertParent("101", 2, "1")
	assertParent("202", 3, "2")
	assertParent("401", 4, "4")
	assertParent("501", 16, "5")
	assertParent("506", 16, "5")
	assertParent("601", 17, "6")
	assertParent("613", 17, "6")

	c101, c601 := st.bySource["101"], st.bySource["601"]
	if c101.ID == c601.ID || c101.Slug == c601.Slug {
		t.Fatal("101 and 601 must be distinct rows")
	}
	if c101.Name != "动作片" || c601.Name != "动作片" {
		t.Fatal("labels should both be 动作片")
	}
	if *c101.ParentID == *c601.ParentID {
		t.Fatal("101 and 601 must not share parent_id")
	}

	// Tops remain top-level.
	for _, sid := range []string{"1", "2", "4", "5", "6"} {
		if st.bySource[sid].ParentID != nil {
			t.Fatalf("top %s parent_id=%v want null", sid, st.bySource[sid].ParentID)
		}
	}
}

func TestUpsertCategoriesTwoPhase_unresolvedParentNotTopLevel(t *testing.T) {
	st := newHierStore()
	// Child without parent 1 in store or tops.
	cats := []olehdtv.SourceCategory{
		{SourceID: "101", Name: "动作片", Slug: "type-101", Sort: 11, ParentSourceID: "1"},
	}
	_, stats, err := UpsertCategoriesTwoPhase(st, cats, false, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed < 1 {
		t.Fatalf("want failed>=1 got %+v", stats)
	}
	if _, ok := st.bySource["101"]; ok {
		t.Fatal("unresolved child must not be inserted")
	}
	// Merge with only orphan child input: no tops, so Merge won't add verified kids from missing parents.
	// Explicit ParentSourceID path still fails closed.
}

func TestUpsertCategoriesTwoPhase_idempotent(t *testing.T) {
	st := newHierStore()
	st.seed(model.Category{ID: 2, SourceID: "1", Name: "电影", Slug: "movie", Sort: 10, IsActive: true})
	tops := []olehdtv.SourceCategory{
		{SourceID: "1", Name: "电影", Slug: "movie", Sort: 10},
	}
	m1, s1, err := UpsertCategoriesTwoPhase(st, tops, false, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	id101 := m1["101"]
	if id101 == 0 {
		t.Fatal("expected child 101")
	}
	m2, s2, err := UpsertCategoriesTwoPhase(st, tops, false, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if m2["101"] != id101 {
		t.Fatalf("id changed %d -> %d", id101, m2["101"])
	}
	if s2.Created != 0 {
		t.Fatalf("second run created=%d want 0", s2.Created)
	}
	if s1.Created == 0 {
		t.Fatalf("first run should create children: %+v", s1)
	}
	c := st.bySource["101"]
	if c.Slug != "type-101" || c.ParentID == nil || *c.ParentID != 2 || c.Sort != 11 {
		t.Fatalf("stable fields: %+v parent=%v", c, c.ParentID)
	}
}

func TestUpsertCategoriesTwoPhase_topLevelBehaviorUnchanged(t *testing.T) {
	st := newHierStore()
	st.seed(model.Category{ID: 2, SourceID: "1", Name: "电影", Slug: "movie", Sort: 10, IsActive: true})
	tops := []olehdtv.SourceCategory{
		{SourceID: "1", Name: "电影", Slug: "movie", Sort: 10},
	}
	catMap, _, err := UpsertCategoriesTwoPhase(st, tops, false, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if catMap["1"] != 2 {
		t.Fatalf("movie local id=%d want 2", catMap["1"])
	}
	movie := st.bySource["1"]
	if movie.Slug != "movie" || movie.Name != "电影" || movie.ParentID != nil {
		t.Fatalf("top changed: %+v", movie)
	}
}
