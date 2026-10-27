package catalogsync

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
)

func ptrID(v uint64) *uint64 { return &v }

func listingHTML(vodIDs ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul>`)
	for _, id := range vodIDs {
		b.WriteString(fmt.Sprintf(`<a href="/index.php/vod/detail/id/%s.html" title="t%s">x</a>`, id, id))
	}
	b.WriteString(`</ul></body></html>`)
	return b.String()
}

type leafMemStore struct {
	cats   []model.Category
	videos map[string]*model.Video // source_id -> video
	sets   []struct {
		videoID uint64
		leaf    string
	}
}

func (m *leafMemStore) ListCategories() ([]model.Category, error) { return m.cats, nil }

func (m *leafMemStore) GetVideoBySource(system, sourceID string) (*model.Video, error) {
	_ = system
	v := m.videos[sourceID]
	if v == nil {
		return nil, nil
	}
	cp := *v
	return &cp, nil
}

func (m *leafMemStore) SetVideoLeafSourceID(videoID uint64, leafSourceID string) error {
	m.sets = append(m.sets, struct {
		videoID uint64
		leaf    string
	}{videoID, leafSourceID})
	for _, v := range m.videos {
		if v.ID == videoID {
			v.LeafSourceID = leafSourceID
			return nil
		}
	}
	return fmt.Errorf("video %d not found", videoID)
}

type leafMemFetcher struct {
	pages map[string]string
	urls  []string
}

func (f *leafMemFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	_ = ctx
	f.urls = append(f.urls, pageURL)
	if strings.Contains(pageURL, "/vod/detail/") {
		return "", fmt.Errorf("detail pages must not be fetched")
	}
	if strings.Contains(pageURL, "/page/") {
		return "", fmt.Errorf("pagination beyond page 1 must not be fetched: %s", pageURL)
	}
	body, ok := f.pages[pageURL]
	if !ok {
		// also try path suffix match
		for k, v := range f.pages {
			if strings.HasSuffix(pageURL, k) || strings.Contains(pageURL, k) {
				return v, nil
			}
		}
		return "", fmt.Errorf("missing page %s", pageURL)
	}
	return body, nil
}

func baseCats() []model.Category {
	return []model.Category{
		{ID: 2, ParentID: nil, SourceID: "1", Name: "电影", Slug: "movie"},
		{ID: 16, ParentID: nil, SourceID: "5", Name: "午夜影院", Slug: "midnight"},
		{ID: 17, ParentID: nil, SourceID: "6", Name: "VIP蓝光影院", Slug: "vip-bluray"},
		{ID: 100, ParentID: ptrID(2), SourceID: "101", Name: "动作片", Slug: "type-101"},
		{ID: 300, ParentID: ptrID(16), SourceID: "506", Name: "AI真人", Slug: "type-506"},
		{ID: 200, ParentID: ptrID(17), SourceID: "601", Name: "动作片", Slug: "type-601"},
		// Noncanonical parent 8 child — must be skipped
		{ID: 8, ParentID: nil, SourceID: "8", Name: "电视直播", Slug: "tv-live"},
		{ID: 801, ParentID: ptrID(8), SourceID: "801", Name: "央视", Slug: "type-801"},
	}
}

func TestSelectLeafTargets_canonicalOnly(t *testing.T) {
	got := SelectLeafTargets(baseCats())
	if len(got) != 2 {
		t.Fatalf("len=%d want 2 (101,506; VIP 601 excluded)", len(got))
	}
	for _, leaf := range got {
		if leaf.SourceID == "801" {
			t.Fatal("noncanonical parent child must be excluded")
		}
		if leaf.SourceID == "601" || leaf.ParentSourceID == "6" {
			t.Fatal("VIP parent/leaf must be excluded from leaf sync")
		}
		if !olehdtv.IsCanonicalHierarchyParentSourceID(leaf.ParentSourceID) {
			t.Fatalf("parent %s not canonical", leaf.ParentSourceID)
		}
	}
}

func TestLeafMembership_movie101Assign(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"9001": {ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Assigned != 1 || st.videos["9001"].LeafSourceID != "101" {
		t.Fatalf("sum=%+v leaf=%q sets=%v", sum, st.videos["9001"].LeafSourceID, st.sets)
	}
	if st.videos["9001"].CategoryID != 2 || st.videos["9001"].SourceCategoryID != "1" {
		t.Fatal("ownership fields changed")
	}
}

func TestLeafMembership_missingLocalNoCreate(t *testing.T) {
	st := &leafMemStore{cats: baseCats(), videos: map[string]*model.Video{}}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9999"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.MissingLocal != 1 || sum.Assigned != 0 || len(st.videos) != 0 || len(st.sets) != 0 {
		t.Fatalf("sum=%+v videos=%d sets=%d", sum, len(st.videos), len(st.sets))
	}
}

func TestLeafMembership_101ListingVIPOwnedRejected(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"9001": {ID: 1, SourceID: "9001", CategoryID: 17, SourceCategoryID: "6"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.OwnershipMismatch != 1 || sum.Assigned != 0 || st.videos["9001"].LeafSourceID != "" {
		t.Fatalf("sum=%+v leaf=%q", sum, st.videos["9001"].LeafSourceID)
	}
}

func TestLeafMembership_midnight506(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"5001": {ID: 5, SourceID: "5001", CategoryID: 16, SourceCategoryID: "5"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML("5001"),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Assigned != 1 || st.videos["5001"].LeafSourceID != "506" {
		t.Fatalf("sum=%+v leaf=%q", sum, st.videos["5001"].LeafSourceID)
	}
}

func TestLeafMembership_midnight506MovieOwnedRejected(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"5001": {ID: 5, SourceID: "5001", CategoryID: 2, SourceCategoryID: "1"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML("5001"),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.OwnershipMismatch != 1 || st.videos["5001"].LeafSourceID != "" {
		t.Fatalf("sum=%+v", sum)
	}
}

func TestLeafMembership_vip601Skipped(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"6001": {ID: 6, SourceID: "6001", CategoryID: 17, SourceCategoryID: "6"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML("6001"),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Assigned != 0 || st.videos["6001"].LeafSourceID != "" {
		t.Fatalf("VIP leaf must not be enriched: sum=%+v leaf=%q", sum, st.videos["6001"].LeafSourceID)
	}
	if sum.LeavesSelected != 2 {
		t.Fatalf("LeavesSelected=%d want 2 (VIP excluded)", sum.LeavesSelected)
	}
}

func TestLeafMembership_101vs601Isolation(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"9001": {ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1"},
			"6001": {ID: 6, SourceID: "6001", CategoryID: 17, SourceCategoryID: "6"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML("6001"),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if st.videos["9001"].LeafSourceID != "101" {
		t.Fatalf("movie leaf=%q", st.videos["9001"].LeafSourceID)
	}
	if st.videos["6001"].LeafSourceID != "" {
		t.Fatalf("VIP must stay unenriched leaf=%q", st.videos["6001"].LeafSourceID)
	}
	if sum.Assigned != 1 {
		t.Fatalf("assigned=%d want 1", sum.Assigned)
	}
}

func TestLeafMembership_unchanged(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"9001": {ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1", LeafSourceID: "101"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Unchanged != 1 || sum.Assigned != 0 || len(st.sets) != 0 {
		t.Fatalf("sum=%+v sets=%v", sum, st.sets)
	}
}

func TestLeafMembership_existingDifferentConflict(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"9001": {ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1", LeafSourceID: "102"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.MembershipConflicts != 1 || st.videos["9001"].LeafSourceID != "102" || len(st.sets) != 0 {
		t.Fatalf("sum=%+v leaf=%q", sum, st.videos["9001"].LeafSourceID)
	}
}

func TestLeafMembership_multiLeafConflictNoLastWriterWins(t *testing.T) {
	st := &leafMemStore{
		cats: append(baseCats(), model.Category{
			ID: 102, ParentID: ptrID(2), SourceID: "102", Name: "喜剧片", Slug: "type-102",
		}),
		videos: map[string]*model.Video{
			"9001": {ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/102.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), false, 0)
	if err == nil {
		t.Fatal("expected architecture-review error")
	}
	if !sum.NeedsArchitectureReview || sum.MembershipConflicts < 1 {
		t.Fatalf("sum=%+v", sum)
	}
	if st.videos["9001"].LeafSourceID != "" || len(st.sets) != 0 {
		t.Fatalf("must not assign on multi-leaf: leaf=%q sets=%v", st.videos["9001"].LeafSourceID, st.sets)
	}
}

func TestLeafMembership_dryRunZeroUpdates(t *testing.T) {
	st := &leafMemStore{
		cats: baseCats(),
		videos: map[string]*model.Video{
			"9001": {ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1"},
		},
	}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("9001"),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Assigned != 1 || len(st.sets) != 0 || st.videos["9001"].LeafSourceID != "" {
		t.Fatalf("dry-run must not write: sum=%+v sets=%v leaf=%q", sum, st.sets, st.videos["9001"].LeafSourceID)
	}
}

func TestLeafMembership_onePagePerLeafNoDetail(t *testing.T) {
	st := &leafMemStore{cats: baseCats(), videos: map[string]*model.Video{}}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML("1") + `<a class="page-next" href="/index.php/vod/type/id/101/page/2.html">下一页</a>`,
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), true, 0)
	if err != nil {
		t.Fatal(err)
	}
	// VIP 601 is excluded from SelectLeafTargets → only 101 + 506.
	if sum.Requests != 2 || sum.DetailRequests != 0 {
		t.Fatalf("requests=%d detail=%d urls=%v", sum.Requests, sum.DetailRequests, f.urls)
	}
	for _, u := range f.urls {
		if strings.Contains(u, "/page/") || strings.Contains(u, "/detail/") {
			t.Fatalf("forbidden url fetched: %s", u)
		}
		if strings.Contains(u, "/id/601") {
			t.Fatalf("VIP leaf must not be fetched: %s", u)
		}
	}
	if len(f.urls) != 2 {
		t.Fatalf("urls=%v", f.urls)
	}
}

func TestLeafMembership_limitLeaves(t *testing.T) {
	st := &leafMemStore{cats: baseCats(), videos: map[string]*model.Video{}}
	f := &leafMemFetcher{pages: map[string]string{
		"https://maccms.local/index.php/vod/type/id/101.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/506.html": listingHTML(),
		"https://maccms.local/index.php/vod/type/id/601.html": listingHTML(),
	}}
	sum, err := NewLeafMembershipSync(st, f, "https://maccms.local").Run(context.Background(), true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.LeavesSelected != 1 || sum.Requests != 1 {
		t.Fatalf("sum=%+v urls=%v", sum, f.urls)
	}
}
