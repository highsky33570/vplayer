package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

func importCats() []model.Category {
	return []model.Category{
		{ID: 2, ParentID: nil, SourceID: "1", Name: "电影", Slug: "movie"},
		{ID: 3, ParentID: nil, SourceID: "2", Name: "连续剧", Slug: "tv"},
		{ID: 16, ParentID: nil, SourceID: "5", Name: "午夜影院", Slug: "midnight"},
		{ID: 17, ParentID: nil, SourceID: "6", Name: "VIP蓝光影院", Slug: "vip-bluray"},
		{ID: 100, ParentID: ptrID(2), SourceID: "101", Name: "动作片", Slug: "type-101"},
		{ID: 102, ParentID: ptrID(2), SourceID: "102", Name: "喜剧片", Slug: "type-102"},
		{ID: 201, ParentID: ptrID(3), SourceID: "201", Name: "欧美剧", Slug: "type-201"},
		{ID: 300, ParentID: ptrID(16), SourceID: "506", Name: "AI真人", Slug: "type-506"},
		{ID: 200, ParentID: ptrID(17), SourceID: "601", Name: "动作片", Slug: "type-601"},
	}
}

func listingPage(vodIDs []string, nextHref string) string {
	var b strings.Builder
	b.WriteString(`<html><body><ul>`)
	for _, id := range vodIDs {
		b.WriteString(fmt.Sprintf(`<a href="/index.php/vod/detail/id/%s.html" title="t%s">x</a>`, id, id))
	}
	b.WriteString(`</ul>`)
	if nextHref != "" {
		b.WriteString(fmt.Sprintf(`<div class="page"><a class="page-next" href="%s">下一页</a></div>`, nextHref))
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

func detailHTML(vodID, typeID, title string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><title>%s</title></head><body>
<div class="content"><h1>%s</h1>
<span class="type"><a href="/index.php/vod/type/id/%s.html">Genre</a></span>
<ul class="play-list"><li><a href="/index.php/vod/play/id/%s/sid/1/nid/1.html">HD</a></li></ul>
</div></body></html>`, title, title, typeID, vodID)
}

type importMemStore struct {
	cats     []model.Category
	videos   map[string]*model.Video
	episodes map[string]*store.EpisodeSyncRow
	nextID   uint64
	sets     []struct {
		videoID uint64
		leaf    string
	}
	inserts           int
	updates           int
	failInsert        bool
	failEpisodeInsert bool
	failLeafSet       bool
}

func newImportMemStore(cats []model.Category) *importMemStore {
	return &importMemStore{
		cats:     cats,
		videos:   map[string]*model.Video{},
		episodes: map[string]*store.EpisodeSyncRow{},
		nextID:   1,
	}
}

func (m *importMemStore) ListCategories() ([]model.Category, error) { return m.cats, nil }

func (m *importMemStore) GetVideoBySource(system, sourceID string) (*model.Video, error) {
	_ = system
	v := m.videos[sourceID]
	if v == nil {
		return nil, nil
	}
	cp := *v
	return &cp, nil
}

func (m *importMemStore) GetCategoryBySource(system, sourceID string) (*model.Category, error) {
	_ = system
	for _, c := range m.cats {
		if c.SourceID == sourceID {
			cp := c
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *importMemStore) SetVideoLeafSourceID(videoID uint64, leafSourceID string) error {
	if m.failLeafSet {
		return fmt.Errorf("leaf assign failed")
	}
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

func (m *importMemStore) InsertVideoCatalog(in store.VideoUpsert) (uint64, error) {
	if m.failInsert {
		return 0, fmt.Errorf("insert failed")
	}
	m.inserts++
	id := m.nextID
	m.nextID++
	m.videos[in.SourceID] = &model.Video{
		ID:               id,
		SourceSystem:     in.SourceSystem,
		SourceID:         in.SourceID,
		CategoryID:       in.CategoryID,
		SourceCategoryID: in.SourceCategoryID,
		Title:            in.Title,
		Status:           in.Status,
		IsActive:         in.IsActive,
	}
	return id, nil
}

func (m *importMemStore) UpdateVideoCatalog(id uint64, in store.VideoUpsert) error {
	m.updates++
	for _, v := range m.videos {
		if v.ID == id {
			v.CategoryID = in.CategoryID
			v.SourceCategoryID = in.SourceCategoryID
			v.Title = in.Title
			return nil
		}
	}
	return fmt.Errorf("missing %d", id)
}

func (m *importMemStore) GetEpisodeForSync(system, sourceVideoID string, sid, nid int) (*store.EpisodeSyncRow, error) {
	_ = system
	key := fmt.Sprintf("%s:%d:%d", sourceVideoID, sid, nid)
	ep := m.episodes[key]
	if ep == nil {
		return nil, nil
	}
	cp := *ep
	return &cp, nil
}

func (m *importMemStore) InsertEpisodeCatalog(in store.EpisodeUpsert) error {
	if m.failEpisodeInsert {
		return fmt.Errorf("episode insert failed")
	}
	key := fmt.Sprintf("%s:%d:%d", in.SourceVideoID, in.SID, in.NID)
	m.episodes[key] = &store.EpisodeSyncRow{
		ID:             uint64(len(m.episodes) + 1),
		VideoID:        in.VideoID,
		SID:            in.SID,
		NID:            in.NID,
		Title:          in.Title,
		PlaybackSource: in.PlaybackSource,
		PlaybackURL:    in.PlaybackURL,
		PlayPageURL:    in.PlayPageURL,
		PlaybackStatus: in.PlaybackStatus,
		IsActive:       in.IsActive,
	}
	return nil
}

func (m *importMemStore) UpdateEpisodeCatalog(id uint64, in store.EpisodeUpsert) error {
	_ = id
	return m.InsertEpisodeCatalog(in)
}

type importMemFetcher struct {
	pages map[string]string
	urls  []string
}

func (f *importMemFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	_ = ctx
	f.urls = append(f.urls, pageURL)
	if strings.Contains(pageURL, "/vod/play/") {
		return "", fmt.Errorf("play pages must not be fetched: %s", pageURL)
	}
	body, ok := f.pages[pageURL]
	if !ok {
		for k, v := range f.pages {
			if strings.HasSuffix(pageURL, strings.TrimPrefix(k, "https://maccms.local")) || strings.Contains(pageURL, k) {
				return v, nil
			}
		}
		return "", fmt.Errorf("missing page %s", pageURL)
	}
	return body, nil
}

func (f *importMemFetcher) detailCount() int {
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, "/vod/detail/") {
			n++
		}
	}
	return n
}

func (f *importMemFetcher) playCount() int {
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, "/vod/play/") {
			n++
		}
	}
	return n
}

func baseImportFetcher() *importMemFetcher {
	base := "https://maccms.local"
	return &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html":        listingPage([]string{"9001"}, base+"/index.php/vod/type/id/101/page/2.html"),
		base + "/index.php/vod/type/id/101/page/2.html": listingPage([]string{"9002"}, base+"/index.php/vod/type/id/101/page/3.html"),
		base + "/index.php/vod/type/id/101/page/3.html": listingPage([]string{"9003"}, base+"/index.php/vod/type/id/101/page/4.html"),
		base + "/index.php/vod/type/id/101/page/4.html": listingPage([]string{"9004"}, ""),
		base + "/index.php/vod/type/id/102.html":        listingPage(nil, ""),
		base + "/index.php/vod/type/id/201.html":        listingPage(nil, ""),
		base + "/index.php/vod/type/id/506.html":        listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html":        listingPage(nil, ""),
		base + "/index.php/vod/detail/id/9001.html":     detailHTML("9001", "999", "Action One"), // hostile TypeID
		base + "/index.php/vod/detail/id/9002.html":     detailHTML("9002", "101", "Action Two"),
		base + "/index.php/vod/detail/id/9003.html":     detailHTML("9003", "101", "Action Three"),
		base + "/index.php/vod/detail/id/9004.html":     detailHTML("9004", "101", "Action Four"),
	}}
}

func TestLeafImport_paginationExactMaxPages(t *testing.T) {
	st := newImportMemStore(importCats())
	f := baseImportFetcher()
	opt := LeafImportOptions{DryRun: true, MaxPagesPerLeaf: 3, MaxNewPerLeaf: 8, MaxNewTotal: 150, MaxRequests: 400, LimitLeaves: 1}
	sum, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.PagesScanned != 3 {
		t.Fatalf("pages_scanned=%d want 3", sum.PagesScanned)
	}
	for _, u := range f.urls {
		if strings.Contains(u, "/page/4") {
			t.Fatalf("must not fetch page 4: %s", u)
		}
	}
	if sum.DetailRequests != 0 || f.detailCount() != 0 {
		t.Fatalf("dry-run detail_requests=%d fetched=%d", sum.DetailRequests, f.detailCount())
	}
}

func TestLeafImport_stopWhenNoNext(t *testing.T) {
	st := newImportMemStore(importCats())
	base := "https://maccms.local"
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html": listingPage([]string{"1"}, ""),
		base + "/index.php/vod/type/id/102.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/201.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/506.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html": listingPage(nil, ""),
	}}
	opt := DefaultLeafImportOptions()
	opt.DryRun = true
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if findLeafStat(sum, "101").PagesScanned != 1 {
		t.Fatalf("expected single page for 101: %+v", sum.PerLeaf)
	}
}

func TestLeafImport_requestBudgetStop(t *testing.T) {
	st := newImportMemStore(importCats())
	f := baseImportFetcher()
	opt := LeafImportOptions{DryRun: true, MaxPagesPerLeaf: 10, MaxNewPerLeaf: 8, MaxNewTotal: 150, MaxRequests: 2}
	sum, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.RequestBudgetExhausted {
		t.Fatal("expected request_budget_exhausted")
	}
	if sum.ListingRequests > 2 {
		t.Fatalf("listing_requests=%d", sum.ListingRequests)
	}
}

func TestLeafImport_dryRunZeroWritesAndDetails(t *testing.T) {
	st := newImportMemStore(importCats())
	st.videos["9001"] = &model.Video{ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1"}
	f := baseImportFetcher()
	opt := DefaultLeafImportOptions()
	opt.DryRun = true
	opt.LimitLeaves = 1
	sum, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.DetailRequests != 0 || f.detailCount() != 0 || f.playCount() != 0 {
		t.Fatalf("dry-run must not fetch detail/play: detail=%d play=%d", f.detailCount(), f.playCount())
	}
	if st.inserts != 0 || len(st.sets) != 0 {
		t.Fatalf("dry-run writes inserts=%d sets=%d", st.inserts, len(st.sets))
	}
	if sum.MembershipWouldAssign != 1 {
		t.Fatalf("would_assign=%d", sum.MembershipWouldAssign)
	}
	if sum.ImportsWouldSelect < 1 {
		t.Fatalf("would_select=%d", sum.ImportsWouldSelect)
	}
}

func TestLeafImport_existingCases(t *testing.T) {
	st := newImportMemStore(importCats())
	st.videos["9001"] = &model.Video{ID: 1, SourceID: "9001", CategoryID: 2, SourceCategoryID: "1"}                      // assign
	st.videos["9002"] = &model.Video{ID: 2, SourceID: "9002", CategoryID: 2, SourceCategoryID: "1", LeafSourceID: "101"} // unchanged
	st.videos["9003"] = &model.Video{ID: 3, SourceID: "9003", CategoryID: 16, SourceCategoryID: "5"}                     // mismatch
	base := "https://maccms.local"
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html": listingPage([]string{"9001", "9002", "9003", "9005"}, ""),
		base + "/index.php/vod/type/id/102.html": listingPage([]string{"9006"}, ""), // 9006 has different leaf conflict setup below
		base + "/index.php/vod/type/id/201.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/506.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html": listingPage(nil, ""),
	}}
	st.videos["9006"] = &model.Video{ID: 6, SourceID: "9006", CategoryID: 2, SourceCategoryID: "1", LeafSourceID: "101"}
	opt := DefaultLeafImportOptions()
	opt.DryRun = true
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.MembershipWouldAssign != 1 {
		t.Fatalf("assign=%d", sum.MembershipWouldAssign)
	}
	if sum.Unchanged != 1 {
		t.Fatalf("unchanged=%d", sum.Unchanged)
	}
	if sum.OwnershipMismatch != 1 {
		t.Fatalf("mismatch=%d", sum.OwnershipMismatch)
	}
	if sum.MembershipConflicts < 1 {
		t.Fatalf("conflicts=%d", sum.MembershipConflicts)
	}
}

func TestLeafImport_sameParentMultiLeafConflict(t *testing.T) {
	st := newImportMemStore(importCats())
	base := "https://maccms.local"
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html":    listingPage([]string{"9001"}, ""),
		base + "/index.php/vod/type/id/102.html":    listingPage([]string{"9001"}, ""),
		base + "/index.php/vod/type/id/201.html":    listingPage(nil, ""),
		base + "/index.php/vod/type/id/506.html":    listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html":    listingPage(nil, ""),
		base + "/index.php/vod/detail/id/9001.html": detailHTML("9001", "101", "X"),
	}}
	opt := DefaultLeafImportOptions()
	opt.DryRun = false
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if !errors.Is(err, ErrLeafArchitectureReview) || !sum.NeedsArchitectureReview {
		t.Fatalf("err=%v sum=%+v", err, sum)
	}
	if st.inserts != 0 || len(st.sets) != 0 {
		t.Fatalf("must not write on multi-leaf: inserts=%d sets=%v", st.inserts, st.sets)
	}
	if sum.ImportsSelected != 0 && sum.ImportsSucceeded != 0 {
		t.Fatalf("conflicted missing must not import: selected=%d ok=%d", sum.ImportsSelected, sum.ImportsSucceeded)
	}
	if f.detailCount() != 0 {
		t.Fatal("must not detail-fetch conflicted vod")
	}
}

func TestLeafImport_crossParentMultiLeafConflict(t *testing.T) {
	st := newImportMemStore(importCats())
	base := "https://maccms.local"
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html": listingPage([]string{"9001"}, ""),
		base + "/index.php/vod/type/id/102.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/201.html": listingPage([]string{"9001"}, ""), // TV vs movie cross-parent
		base + "/index.php/vod/type/id/506.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html": listingPage(nil, ""), // VIP leaf ignored by SelectLeafTargets
	}}
	opt := DefaultLeafImportOptions()
	opt.DryRun = true
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if !errors.Is(err, ErrLeafArchitectureReview) {
		t.Fatalf("err=%v", err)
	}
	if sum.ImportsWouldSelect != 0 {
		t.Fatalf("would_select=%d", sum.ImportsWouldSelect)
	}
}

func TestLeafImport_perLeafAndGlobalCaps(t *testing.T) {
	st := newImportMemStore(importCats())
	base := "https://maccms.local"
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html": listingPage([]string{"9101", "9102", "9103"}, ""),
		base + "/index.php/vod/type/id/102.html": listingPage([]string{"9201", "9202"}, ""),
		base + "/index.php/vod/type/id/201.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/506.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html": listingPage(nil, ""),
	}}
	for _, id := range []string{"9101", "9102", "9103", "9201", "9202"} {
		f.pages[base+"/index.php/vod/detail/id/"+id+".html"] = detailHTML(id, "101", id)
	}
	opt := LeafImportOptions{DryRun: true, MaxPagesPerLeaf: 1, MaxNewPerLeaf: 1, MaxNewTotal: 2, MaxRequests: 400}
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ImportsWouldSelect != 2 {
		t.Fatalf("would_select=%d want 2", sum.ImportsWouldSelect)
	}
	if sum.ImportsSkippedPerLeafCap < 1 {
		t.Fatalf("per-leaf skips=%d", sum.ImportsSkippedPerLeafCap)
	}
	// With fair RR + per-leaf=1, leftovers are per-leaf capped (not total-starved).
	if findLeafStat(sum, "101").ImportsSelected != 1 || findLeafStat(sum, "102").ImportsSelected != 1 {
		t.Fatalf("fair RR per-leaf select: %+v", sum.PerLeaf)
	}
}

func TestLeafImport_forcedParentOwnershipAndLeaf(t *testing.T) {
	st := newImportMemStore(importCats())
	f := baseImportFetcher()
	opt := LeafImportOptions{
		DryRun: false, LimitLeaves: 1, MaxPagesPerLeaf: 1, MaxNewPerLeaf: 8, MaxNewTotal: 10, MaxRequests: 50,
	}
	sum, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ImportsSucceeded != 1 {
		t.Fatalf("succeeded=%d sum=%+v", sum.ImportsSucceeded, sum)
	}
	v := st.videos["9001"]
	if v == nil {
		t.Fatal("missing import")
	}
	if v.CategoryID != 2 {
		t.Fatalf("category_id=%d want parent 2 (not child 100)", v.CategoryID)
	}
	if v.SourceCategoryID != "1" {
		t.Fatalf("source_category_id=%q want 1 (detail TypeID must not win)", v.SourceCategoryID)
	}
	if v.LeafSourceID != "101" {
		t.Fatalf("leaf=%q", v.LeafSourceID)
	}
	if f.playCount() != 0 || sum.PlayRequests != 0 {
		t.Fatal("play enrichment forbidden")
	}
	ep := st.episodes["9001:1:1"]
	if ep == nil || ep.PlaybackStatus != "pending_enrichment" {
		t.Fatalf("episode=%+v", ep)
	}
}

func TestLeafImport_insertFailureNoLeafAssign(t *testing.T) {
	st := newImportMemStore(importCats())
	st.failInsert = true
	f := baseImportFetcher()
	opt := LeafImportOptions{DryRun: false, LimitLeaves: 1, MaxPagesPerLeaf: 1, MaxNewPerLeaf: 5, MaxNewTotal: 5, MaxRequests: 20}
	sum, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt)
	if err == nil {
		t.Fatal("expected failure")
	}
	if sum.ImportsFailed < 1 {
		t.Fatalf("imports_failed=%d", sum.ImportsFailed)
	}
	if len(st.sets) != 0 {
		t.Fatalf("leaf assign must not happen after insert failure: %v", st.sets)
	}
}

func TestLeafImport_idempotentSecondRun(t *testing.T) {
	st := newImportMemStore(importCats())
	f := baseImportFetcher()
	opt := LeafImportOptions{DryRun: false, LimitLeaves: 1, MaxPagesPerLeaf: 1, MaxNewPerLeaf: 5, MaxNewTotal: 5, MaxRequests: 20}
	if _, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt); err != nil {
		t.Fatal(err)
	}
	f2 := baseImportFetcher()
	sum, err := NewLeafImportSync(st, f2, "https://maccms.local").Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ImportsSucceeded != 0 || sum.Unchanged != 1 {
		t.Fatalf("second run: succeeded=%d unchanged=%d", sum.ImportsSucceeded, sum.Unchanged)
	}
	if f2.detailCount() != 0 {
		t.Fatal("idempotent run should not re-detail existing")
	}
}

func TestSelectFairImportCandidates_roundRobin(t *testing.T) {
	missing := []leafPlan{
		{vodID: "a1", leafSourceID: "101", parentLocalID: 2, parentSourceID: "1"},
		{vodID: "a2", leafSourceID: "101", parentLocalID: 2, parentSourceID: "1"},
		{vodID: "a3", leafSourceID: "101", parentLocalID: 2, parentSourceID: "1"},
		{vodID: "b1", leafSourceID: "102", parentLocalID: 2, parentSourceID: "1"},
		{vodID: "b2", leafSourceID: "102", parentLocalID: 2, parentSourceID: "1"},
		{vodID: "b3", leafSourceID: "102", parentLocalID: 2, parentSourceID: "1"},
		{vodID: "c1", leafSourceID: "201", parentLocalID: 3, parentSourceID: "2"},
		{vodID: "c2", leafSourceID: "201", parentLocalID: 3, parentSourceID: "2"},
		{vodID: "c3", leafSourceID: "201", parentLocalID: 3, parentSourceID: "2"},
	}
	got := selectFairImportCandidates(missing, 3, 5, 0, 400)
	if len(got.Selected) != 5 {
		t.Fatalf("selected=%d %+v", len(got.Selected), got.Selected)
	}
	counts := map[string]int{}
	for _, p := range got.Selected {
		counts[p.leafSourceID]++
	}
	if counts["101"] != 2 || counts["102"] != 2 || counts["201"] != 1 {
		t.Fatalf("want A2 B2 C1, got %v selected=%+v", counts, got.Selected)
	}
	// Deterministic order: round1 101,102,201 then round2 101,102
	wantOrder := []string{"a1", "b1", "c1", "a2", "b2"}
	for i, id := range wantOrder {
		if got.Selected[i].vodID != id {
			t.Fatalf("order[%d]=%s want %s", i, got.Selected[i].vodID, id)
		}
	}
}

func TestLeafImport_fairnessLaterLeavesNotStarved(t *testing.T) {
	st := newImportMemStore(importCats())
	base := "https://maccms.local"
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html": listingPage([]string{"8101", "8102", "8103"}, ""),
		base + "/index.php/vod/type/id/102.html": listingPage([]string{"8201", "8202", "8203"}, ""),
		base + "/index.php/vod/type/id/201.html": listingPage([]string{"8301", "8302", "8303"}, ""),
		base + "/index.php/vod/type/id/506.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html": listingPage(nil, ""),
	}}
	opt := LeafImportOptions{DryRun: true, MaxPagesPerLeaf: 1, MaxNewPerLeaf: 3, MaxNewTotal: 5, MaxRequests: 400}
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if findLeafStat(sum, "201").ImportsSelected != 1 {
		t.Fatalf("later leaf 201 starved: %+v", sum.PerLeaf)
	}
	if findLeafStat(sum, "101").ImportsSelected != 2 || findLeafStat(sum, "102").ImportsSelected != 2 {
		t.Fatalf("expected 2/2/1: %+v", sum.PerLeaf)
	}
}

func TestLeafImport_paginationLoopStops(t *testing.T) {
	st := newImportMemStore(importCats())
	base := "https://maccms.local"
	// A -> A
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html": listingPage([]string{"1"}, base+"/index.php/vod/type/id/101.html"),
		base + "/index.php/vod/type/id/102.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/201.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/506.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html": listingPage(nil, ""),
	}}
	opt := LeafImportOptions{DryRun: true, MaxPagesPerLeaf: 5, MaxNewPerLeaf: 1, MaxNewTotal: 1, MaxRequests: 50, LimitLeaves: 1}
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.PaginationLoops < 1 {
		t.Fatalf("expected pagination loop stop: %+v", sum)
	}
	if sum.PagesScanned != 1 {
		t.Fatalf("pages=%d want 1 (no repeat fetch)", sum.PagesScanned)
	}

	// A -> B -> A
	f2 := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html":        listingPage([]string{"1"}, base+"/index.php/vod/type/id/101/page/2.html"),
		base + "/index.php/vod/type/id/101/page/2.html": listingPage([]string{"2"}, base+"/index.php/vod/type/id/101.html"),
		base + "/index.php/vod/type/id/102.html":        listingPage(nil, ""),
		base + "/index.php/vod/type/id/201.html":        listingPage(nil, ""),
		base + "/index.php/vod/type/id/506.html":        listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html":        listingPage(nil, ""),
	}}
	sum2, err := NewLeafImportSync(st, f2, base).Run(context.Background(), LeafImportOptions{
		DryRun: true, MaxPagesPerLeaf: 10, MaxNewPerLeaf: 1, MaxNewTotal: 1, MaxRequests: 50, LimitLeaves: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum2.PaginationLoops < 1 || sum2.PagesScanned != 2 {
		t.Fatalf("A->B->A: pages=%d loops=%d", sum2.PagesScanned, sum2.PaginationLoops)
	}
	if len(f2.urls) != 2 {
		t.Fatalf("fetched=%v want exactly 2 listing URLs", f2.urls)
	}
}

func TestLeafImport_splitMultiLeafMetrics(t *testing.T) {
	st := newImportMemStore(importCats())
	base := "https://maccms.local"
	f := &importMemFetcher{pages: map[string]string{
		base + "/index.php/vod/type/id/101.html": listingPage([]string{"9001", "9002", "9003"}, ""),
		base + "/index.php/vod/type/id/102.html": listingPage([]string{"9001"}, ""), // same-parent multi
		base + "/index.php/vod/type/id/201.html": listingPage([]string{"9002"}, ""), // cross-parent with 101
		base + "/index.php/vod/type/id/506.html": listingPage(nil, ""),
		base + "/index.php/vod/type/id/601.html": listingPage(nil, ""), // VIP leaf ignored
	}}
	st.videos["9003"] = &model.Video{ID: 3, SourceID: "9003", CategoryID: 2, SourceCategoryID: "1", LeafSourceID: "102"}

	opt := DefaultLeafImportOptions()
	opt.DryRun = true
	sum, err := NewLeafImportSync(st, f, base).Run(context.Background(), opt)
	if !errors.Is(err, ErrLeafArchitectureReview) {
		t.Fatalf("err=%v", err)
	}
	if sum.MultiLeafConflicts != 2 {
		t.Fatalf("multi=%d want 2", sum.MultiLeafConflicts)
	}
	if sum.SameParentMultiLeafConflicts != 1 || sum.CrossParentMultiLeafConflicts != 1 {
		t.Fatalf("same=%d cross=%d", sum.SameParentMultiLeafConflicts, sum.CrossParentMultiLeafConflicts)
	}
	if sum.ExistingLeafConflicts != 1 {
		t.Fatalf("existing_leaf=%d", sum.ExistingLeafConflicts)
	}
	if sum.MembershipConflicts != sum.MultiLeafConflicts+sum.ExistingLeafConflicts {
		t.Fatalf("aggregate mismatch: %+v", sum)
	}
	if sum.DistinctVideosDiscovered == 0 || float64(sum.MultiLeafConflicts)/float64(sum.DistinctVideosDiscovered) <= 0 {
		t.Fatal("dry-run ratio not computable")
	}
}

func TestLeafImport_episodeFailNoLeafAssign(t *testing.T) {
	st := newImportMemStore(importCats())
	st.failEpisodeInsert = true
	f := baseImportFetcher()
	opt := LeafImportOptions{DryRun: false, LimitLeaves: 1, MaxPagesPerLeaf: 1, MaxNewPerLeaf: 5, MaxNewTotal: 5, MaxRequests: 20}
	sum, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt)
	if err == nil {
		t.Fatal("expected failure")
	}
	if st.videos["9001"] == nil {
		t.Fatal("video should remain after episode failure")
	}
	if st.videos["9001"].LeafSourceID != "" || len(st.sets) != 0 {
		t.Fatalf("no leaf assign: leaf=%q sets=%v", st.videos["9001"].LeafSourceID, st.sets)
	}
	if sum.ImportsFailed < 1 {
		t.Fatalf("imports_failed=%d", sum.ImportsFailed)
	}
}

func TestLeafImport_leafAssignFailCatalogRemainsAndRerunRecovers(t *testing.T) {
	st := newImportMemStore(importCats())
	st.failLeafSet = true
	f := baseImportFetcher()
	opt := LeafImportOptions{DryRun: false, LimitLeaves: 1, MaxPagesPerLeaf: 1, MaxNewPerLeaf: 5, MaxNewTotal: 5, MaxRequests: 20}
	sum, err := NewLeafImportSync(st, f, "https://maccms.local").Run(context.Background(), opt)
	if err == nil || sum.ImportsFailed < 1 {
		t.Fatalf("err=%v sum=%+v", err, sum)
	}
	v := st.videos["9001"]
	if v == nil || v.CategoryID != 2 || v.LeafSourceID != "" {
		t.Fatalf("catalog without leaf: %+v", v)
	}

	st.failLeafSet = false
	f2 := baseImportFetcher()
	sum2, err := NewLeafImportSync(st, f2, "https://maccms.local").Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	// Existing correct-parent + empty leaf → membership assign (no re-import).
	if sum2.MembershipAssigned != 1 {
		t.Fatalf("rerun should assign leaf via membership: %+v", sum2)
	}
	if st.videos["9001"].LeafSourceID != "101" {
		t.Fatalf("recovered leaf=%q", st.videos["9001"].LeafSourceID)
	}
}

func TestValidateLeafImportOptions_hardCaps(t *testing.T) {
	o := DefaultLeafImportOptions()
	o.MaxPagesPerLeaf = LeafImportHardMaxPagesPerLeaf + 1
	if err := ValidateLeafImportOptions(&o); err == nil {
		t.Fatal("expected hard cap error")
	}
}

func findLeafStat(sum LeafImportSummary, leaf string) LeafImportLeafStat {
	for _, st := range sum.PerLeaf {
		if st.LeafSourceID == leaf {
			return st
		}
	}
	return LeafImportLeafStat{}
}

func TestParentCatMap_excludesChildren(t *testing.T) {
	m := parentCatMap(importCats())
	if m["101"] != 0 {
		t.Fatal("child source id must not be in ownership map")
	}
	if m["1"] != 2 {
		t.Fatalf("parent map=%v", m)
	}
	_ = olehdtv.SourceSystem
}
