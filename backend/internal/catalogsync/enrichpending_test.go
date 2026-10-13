package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

type enrichMemStore struct {
	mu       sync.Mutex
	locked   bool
	episodes map[uint64]*store.PendingEnrichmentEpisode
	writes   int
	okCounts map[uint64]int // videoID -> ok count override
	opRuns   []store.OperationalRunRow
	nextOpID uint64
}

func newEnrichMemStore() *enrichMemStore {
	return &enrichMemStore{
		episodes: map[uint64]*store.PendingEnrichmentEpisode{},
		okCounts: map[uint64]int{},
	}
}

func (m *enrichMemStore) AcquireAdvisoryLock(ctx context.Context, name string) (*store.AdvisoryHold, error) {
	_ = ctx
	_ = name
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked {
		return nil, store.ErrOperationLocked
	}
	m.locked = true
	return store.NewTestAdvisoryHold(func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.locked = false
		return nil
	}), nil
}

func (m *enrichMemStore) StartOperationalRun(in store.StartOperationalRunInput) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextOpID++
	id := m.nextOpID
	now := time.Now()
	m.opRuns = append(m.opRuns, store.OperationalRunRow{
		ID: id, JobType: in.JobType, TriggerType: in.TriggerType,
		Status: store.OpStatusRunning, RequestedByUserID: in.RequestedByUserID,
		RequestedAt: now, StartedAt: &now,
	})
	return id, nil
}

func (m *enrichMemStore) FinishOperationalRun(in store.FinishOperationalRunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.opRuns {
		if m.opRuns[i].ID == in.ID {
			now := time.Now()
			m.opRuns[i].Status = in.Status
			m.opRuns[i].FinishedAt = &now
			m.opRuns[i].SelectedCount = in.SelectedCount
			m.opRuns[i].SucceededCount = in.SucceededCount
			m.opRuns[i].SkippedCount = in.SkippedCount
			m.opRuns[i].UnavailableCount = in.UnavailableCount
			m.opRuns[i].FailedCount = in.FailedCount
			m.opRuns[i].ErrorCode = in.ErrorCode
			m.opRuns[i].HasError = in.HasError
			return nil
		}
	}
	return fmt.Errorf("op run %d not found", in.ID)
}

func (m *enrichMemStore) RejectOperationalRun(in store.RejectOperationalRunInput) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextOpID++
	id := m.nextOpID
	now := time.Now()
	m.opRuns = append(m.opRuns, store.OperationalRunRow{
		ID: id, JobType: in.JobType, TriggerType: in.TriggerType,
		Status: store.OpStatusRejected, RequestedByUserID: in.RequestedByUserID,
		RequestedAt: now, StartedAt: &now, FinishedAt: &now,
		ErrorCode: in.ErrorCode, HasError: true,
	})
	return id, nil
}

func (m *enrichMemStore) ListPendingEnrichmentEpisodes(system string, finalLimit int) ([]store.PendingEnrichmentEpisode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.PendingEnrichmentEpisode
	for _, e := range m.episodes {
		if e.SourceSystem != system {
			continue
		}
		if strings.ToLower(e.PlaybackStatus) != "pending_enrichment" {
			continue
		}
		if strings.TrimSpace(e.PlaybackURL) != "" {
			continue
		}
		if strings.TrimSpace(e.PlayPageURL) == "" {
			continue
		}
		cp := *e
		if n, ok := m.okCounts[e.VideoID]; ok {
			cp.OkEpisodeCount = n
		}
		out = append(out, cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.OkEpisodeCount != b.OkEpisodeCount {
			return a.OkEpisodeCount < b.OkEpisodeCount
		}
		if a.VideoID != b.VideoID {
			return a.VideoID < b.VideoID
		}
		if a.NID != b.NID {
			return a.NID < b.NID
		}
		return a.ID < b.ID
	})
	// Mirror SQL ROW_NUMBER() PARTITION BY video_id: keep ≤3 pending per video
	// before applying the global LIMIT so one series cannot fill the window alone.
	perVideo := map[uint64]int{}
	diversified := make([]store.PendingEnrichmentEpisode, 0, len(out))
	for _, e := range out {
		if perVideo[e.VideoID] >= EnrichPendingMaxPerVideo {
			continue
		}
		perVideo[e.VideoID]++
		diversified = append(diversified, e)
	}
	out = diversified
	fetchLimit := finalLimit + 6
	if finalLimit <= 0 {
		fetchLimit = 20
	}
	if fetchLimit > 60 {
		fetchLimit = 60
	}
	if len(out) > fetchLimit {
		out = out[:fetchLimit]
	}
	return out, nil
}

func (m *enrichMemStore) ApplyPlaybackEnrichment(id uint64, playbackURL, playbackSource, status string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.episodes[id]
	if e == nil {
		return false, nil
	}
	if strings.ToLower(e.PlaybackStatus) != "pending_enrichment" || strings.TrimSpace(e.PlaybackURL) != "" {
		return false, nil
	}
	e.PlaybackURL = playbackURL
	if playbackSource != "" {
		e.PlaybackSource = playbackSource
	}
	e.PlaybackStatus = status
	// Match store.ApplyPlaybackEnrichment: activate only on ok + non-empty URL.
	if strings.EqualFold(status, "ok") && strings.TrimSpace(playbackURL) != "" {
		e.IsActive = true
	}
	m.writes++
	return true, nil
}

type mapFetcher struct {
	mu    sync.Mutex
	pages map[string]string
	err   map[string]error
	hits  int
}

func (f *mapFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits++
	if err, ok := f.err[pageURL]; ok {
		return "", err
	}
	body, ok := f.pages[pageURL]
	if !ok {
		return "", fmt.Errorf("missing %s", pageURL)
	}
	return body, nil
}

func playHTML(url string) string {
	return `var player_aaaa={"flag":"play","encrypt":0,"url":"` + strings.ReplaceAll(url, `/`, `\/`) + `","from":"plyr","id":"1","sid":1,"nid":1}</script>`
}

func TestValidateEnrichPendingLimit(t *testing.T) {
	if err := ValidateEnrichPendingLimit(0); err == nil {
		t.Fatal("expected reject")
	}
	if err := ValidateEnrichPendingLimit(21); err == nil {
		t.Fatal("expected reject >20")
	}
	if err := ValidateEnrichPendingLimit(15); err != nil {
		t.Fatal(err)
	}
}

func TestSelectFairPendingBatch_prioritizesZeroOk(t *testing.T) {
	cands := []store.PendingEnrichmentEpisode{
		{ID: 10, VideoID: 2, OkEpisodeCount: 5, NID: 1, SourceVideoID: "v2"},
		{ID: 11, VideoID: 2, OkEpisodeCount: 5, NID: 2, SourceVideoID: "v2"},
		{ID: 1, VideoID: 1, OkEpisodeCount: 0, NID: 1, SourceVideoID: "v1"},
		{ID: 2, VideoID: 1, OkEpisodeCount: 0, NID: 2, SourceVideoID: "v1"},
		{ID: 3, VideoID: 1, OkEpisodeCount: 0, NID: 3, SourceVideoID: "v1"},
		{ID: 4, VideoID: 1, OkEpisodeCount: 0, NID: 4, SourceVideoID: "v1"},
	}
	got := SelectFairPendingBatch(cands, 4)
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	// first slots should be video 1 (zero ok), capped at 3 per video, then video 2
	zero := 0
	for _, g := range got {
		if g.VideoID == 1 {
			zero++
		}
	}
	if zero != 3 {
		t.Fatalf("expected 3 from zero-ok video, got %d (%+v)", zero, got)
	}
}

func TestSelectFairPendingBatch_fillsLimitAcrossVideos(t *testing.T) {
	// Reproduce production bug shape: video A has many pending (zero-ok), B/C also eligible.
	var cands []store.PendingEnrichmentEpisode
	id := uint64(1)
	for nid := 1; nid <= 100; nid++ {
		cands = append(cands, store.PendingEnrichmentEpisode{
			ID: id, VideoID: 10, OkEpisodeCount: 0, NID: nid, SourceVideoID: "A",
		})
		id++
	}
	for nid := 1; nid <= 50; nid++ {
		cands = append(cands, store.PendingEnrichmentEpisode{
			ID: id, VideoID: 20, OkEpisodeCount: 0, NID: nid, SourceVideoID: "B",
		})
		id++
	}
	for nid := 1; nid <= 20; nid++ {
		cands = append(cands, store.PendingEnrichmentEpisode{
			ID: id, VideoID: 30, OkEpisodeCount: 0, NID: nid, SourceVideoID: "C",
		})
		id++
	}

	got5 := SelectFairPendingBatch(cands, 5)
	if len(got5) != 5 {
		t.Fatalf("limit=5: want 5 got %d (%+v)", len(got5), got5)
	}
	counts5 := map[uint64]int{}
	for _, g := range got5 {
		counts5[g.VideoID]++
	}
	for vid, n := range counts5 {
		if n > EnrichPendingMaxPerVideo {
			t.Fatalf("limit=5: video %d contributed %d > %d", vid, n, EnrichPendingMaxPerVideo)
		}
	}
	if counts5[10] != 3 || counts5[20] != 2 {
		t.Fatalf("limit=5: want A:3 B:2, got %v", counts5)
	}

	// Add more videos so limit=15 can be filled under the ≤3/video rule.
	for vid := uint64(40); vid <= 60; vid++ {
		for nid := 1; nid <= 10; nid++ {
			cands = append(cands, store.PendingEnrichmentEpisode{
				ID: id, VideoID: vid, OkEpisodeCount: 0, NID: nid, SourceVideoID: fmt.Sprintf("V%d", vid),
			})
			id++
		}
	}
	got15 := SelectFairPendingBatch(cands, 15)
	if len(got15) != 15 {
		t.Fatalf("limit=15: want 15 got %d", len(got15))
	}
	counts15 := map[uint64]int{}
	for _, g := range got15 {
		counts15[g.VideoID]++
		if counts15[g.VideoID] > EnrichPendingMaxPerVideo {
			t.Fatalf("limit=15: video %d contributed >%d", g.VideoID, EnrichPendingMaxPerVideo)
		}
	}
	if counts15[10] != 3 || counts15[20] != 3 || counts15[30] != 3 {
		t.Fatalf("limit=15: zero-ok priority should take A/B/C first at 3 each; got %v", counts15)
	}
}

func TestSelectFairPendingBatch_noVideoAboveMaxPerVideo(t *testing.T) {
	var cands []store.PendingEnrichmentEpisode
	for i := 1; i <= 40; i++ {
		cands = append(cands, store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64((i-1)/10 + 1), OkEpisodeCount: 0, NID: i,
		})
	}
	got := SelectFairPendingBatch(cands, 20)
	if len(got) != 12 { // 4 videos × 3
		t.Fatalf("want 12 got %d", len(got))
	}
	per := map[uint64]int{}
	for _, g := range got {
		per[g.VideoID]++
	}
	for vid, n := range per {
		if n > EnrichPendingMaxPerVideo {
			t.Fatalf("video %d has %d", vid, n)
		}
	}
}

func TestEnrichPending_listDiversifiesAndFillsLimit(t *testing.T) {
	st := newEnrichMemStore()
	pages := map[string]string{}
	// Video A: 100 pending zero-ok; B: 50; C: 20 — all eligible.
	add := func(videoID uint64, source string, n int) {
		st.okCounts[videoID] = 0
		for nid := 1; nid <= n; nid++ {
			id := videoID*1000 + uint64(nid)
			url := fmt.Sprintf("https://src/%s/%d", source, nid)
			st.episodes[id] = &store.PendingEnrichmentEpisode{
				ID: id, VideoID: videoID, SourceSystem: olehdtv.SourceSystem, SourceVideoID: source,
				SID: 1, NID: nid, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
			}
			pages[url] = playHTML(fmt.Sprintf("https://cdn.example/%s/%d.m3u8", source, nid))
		}
	}
	add(10, "A", 100)
	add(20, "B", 50)
	add(30, "C", 20)

	list, err := st.ListPendingEnrichmentEpisodes(olehdtv.SourceSystem, 5)
	if err != nil {
		t.Fatal(err)
	}
	// After ≤3/video diversification, fetchLimit=11 → many videos present; none >3.
	per := map[uint64]int{}
	for _, e := range list {
		per[e.VideoID]++
	}
	for vid, n := range per {
		if n > EnrichPendingMaxPerVideo {
			t.Fatalf("list returned %d from video %d", n, vid)
		}
	}
	if per[10] == 0 || per[20] == 0 {
		t.Fatalf("list must include multiple videos, got %v", per)
	}

	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).Run(context.Background(), 5, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Selected != 5 {
		t.Fatalf("selected=%d want 5 actions=%+v", sum.Selected, sum.Actions)
	}
	selPer := map[string]int{}
	for _, a := range sum.Actions {
		selPer[a.SourceVideoID]++
	}
	for _, n := range selPer {
		if n > EnrichPendingMaxPerVideo {
			t.Fatalf("selected per-video >3: %v", selPer)
		}
	}
	if selPer["A"] != 3 || selPer["B"] != 2 {
		t.Fatalf("want A:3 B:2, got %v", selPer)
	}

	add(40, "D", 20)
	add(50, "E", 20)
	sum15, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).Run(context.Background(), 15, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum15.Selected != 15 {
		t.Fatalf("limit=15 selected=%d want 15", sum15.Selected)
	}
	sel15 := map[string]int{}
	for _, a := range sum15.Actions {
		sel15[a.SourceVideoID]++
		if sel15[a.SourceVideoID] > EnrichPendingMaxPerVideo {
			t.Fatalf("limit=15 per-video >3: %v", sel15)
		}
	}
}

func TestEnrichPending_successActivatesPlayableEpisode(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
		IsActive: false,
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")}}
	sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Enriched != 1 {
		t.Fatalf("%+v", sum)
	}
	if !st.episodes[1].IsActive {
		t.Fatal("successful enrich with usable URL must set is_active=1")
	}
	if st.episodes[1].PlaybackStatus != "ok" || st.episodes[1].PlaybackURL == "" {
		t.Fatalf("playback fields not updated: %+v", st.episodes[1])
	}
}

func TestEnrichPending_sourceRestrictedBecomesUnavailable(t *testing.T) {
	restricted := `<html><body><div>系统提示<br/>您没有权限访问此数据，请升级会员</div></body></html>`
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 10, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "84839",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
		IsActive: false,
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": restricted}}
	sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Unavailable != 1 || sum.Enriched != 0 || sum.StillPending != 0 {
		t.Fatalf("%+v", sum)
	}
	ep := st.episodes[1]
	if ep.PlaybackStatus != "unavailable" || ep.PlaybackURL != "" || ep.IsActive {
		t.Fatalf("restricted episode: %+v", ep)
	}
	if len(sum.Actions) != 1 || sum.Actions[0].Detail != "source_restricted" {
		t.Fatalf("actions=%+v", sum.Actions)
	}
	list, err := st.ListPendingEnrichmentEpisodes(olehdtv.SourceSystem, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("still pending-eligible: %+v", list)
	}
}

func TestEnrichPending_sourceRestrictedDryRun(t *testing.T) {
	restricted := `<html><body>系统提示 您没有权限访问此数据，请升级会员</body></html>`
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 10, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "84839",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": restricted}}
	sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Unavailable != 1 || st.writes != 0 {
		t.Fatalf("dry-run writes=%d sum=%+v", st.writes, sum)
	}
	if st.episodes[1].PlaybackStatus != "pending_enrichment" {
		t.Fatal("dry-run must not persist")
	}
	if len(sum.Actions) != 1 || sum.Actions[0].Action != "WOULD_UNAVAILABLE" || sum.Actions[0].Detail != "source_restricted" {
		t.Fatalf("%+v", sum.Actions)
	}
}

func TestEnrichPending_successAndIdempotent(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 9, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "37782",
		SID: 1, NID: 4, PlaybackStatus: "pending_enrichment",
		PlayPageURL: "https://src/p4", HlsObjectKey: "", MigrationStatus: "",
	}
	st.okCounts[9] = 3
	f := &mapFetcher{pages: map[string]string{
		"https://src/p4": playHTML("https://cdn.example/4.m3u8"),
	}}
	svc := NewEnrichPendingWorker(st, f)
	sum, err := svc.Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Enriched != 1 || sum.RequestsMade != 1 || !sum.LockAcquired {
		t.Fatalf("%+v", sum)
	}
	ep := st.episodes[1]
	if ep.PlaybackURL != "https://cdn.example/4.m3u8" || ep.PlaybackStatus != "ok" || !ep.IsActive {
		t.Fatalf("%+v", ep)
	}
	writes := st.writes
	sum2, err := svc.Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum2.Selected != 0 || st.writes != writes {
		t.Fatalf("idempotent failed: sum=%+v writes=%d", sum2, st.writes)
	}
}

func TestEnrichPending_fetchFailRemainsPending(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{err: map[string]error{"https://src/p1": errors.New("timeout")}}
	sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
	if err == nil {
		t.Fatal("expected failure return when only failures")
	}
	if sum.StillPending != 1 || sum.Enriched != 0 {
		t.Fatalf("%+v", sum)
	}
	if st.episodes[1].PlaybackStatus != "pending_enrichment" || st.episodes[1].PlaybackURL != "" {
		t.Fatalf("%+v", st.episodes[1])
	}
	if st.writes != 0 {
		t.Fatal("pending must not write on fetch fail")
	}
}

func TestEnrichPending_parseFailRemainsPending(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": `<html>no player</html>`}}
	sum, _ := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
	if sum.StillPending != 1 || st.writes != 0 || st.episodes[1].PlaybackStatus != "pending_enrichment" {
		t.Fatalf("%+v ep=%+v", sum, st.episodes[1])
	}
}

func TestEnrichPending_neverOverwritesGoodOrMigration(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "ok", PlaybackURL: "https://keep/a.m3u8",
		PlayPageURL:  "https://src/p1",
		HlsObjectKey: "videos/1/1/index.m3u8", StorageProvider: "r2", MigrationStatus: "done",
	}
	// also a pending that becomes skipped if we race-set URL before apply — simulate Apply no-op
	st.episodes[2] = &store.PendingEnrichmentEpisode{
		ID: 2, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 2, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p2",
		HlsObjectKey: "keep-key", MigrationStatus: "done", StorageProvider: "r2",
	}
	f := &mapFetcher{pages: map[string]string{
		"https://src/p2": playHTML("https://cdn.example/2.m3u8"),
	}}
	sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Enriched != 1 {
		t.Fatalf("%+v", sum)
	}
	if st.episodes[1].PlaybackURL != "https://keep/a.m3u8" || st.episodes[1].HlsObjectKey != "videos/1/1/index.m3u8" {
		t.Fatalf("good row touched: %+v", st.episodes[1])
	}
	if st.episodes[2].HlsObjectKey != "keep-key" || st.episodes[2].MigrationStatus != "done" || st.episodes[2].StorageProvider != "r2" {
		t.Fatalf("migration wiped: %+v", st.episodes[2])
	}
	if st.episodes[2].PlaybackURL != "https://cdn.example/2.m3u8" {
		t.Fatalf("pending not enriched: %+v", st.episodes[2])
	}
}

func TestEnrichPending_dryRunZeroWrites(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")}}
	sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.DryRun || sum.Enriched != 1 || st.writes != 0 || st.episodes[1].PlaybackURL != "" {
		t.Fatalf("sum=%+v writes=%d ep=%+v", sum, st.writes, st.episodes[1])
	}
	if f.hits != 1 {
		t.Fatalf("expected HTTP in dry-run, hits=%d", f.hits)
	}
}

func TestEnrichPending_lockBusyZeroWork(t *testing.T) {
	st := newEnrichMemStore()
	st.locked = true
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://x/1.m3u8")}}
	w := NewEnrichPendingWorker(st, f)
	w.TriggerType = store.TriggerTimer
	sum, err := w.Run(context.Background(), 5, false, 1)
	if err == nil || !errors.Is(err, store.ErrOperationLocked) || sum.LockAcquired || sum.Attempted != 0 || f.hits != 0 {
		t.Fatalf("sum=%+v err=%v hits=%d", sum, err, f.hits)
	}
	if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusRejected ||
		st.opRuns[0].ErrorCode != store.ErrCodeLockBusy || st.opRuns[0].JobType != store.JobEnrichment {
		t.Fatalf("opRuns=%+v", st.opRuns)
	}
}

func TestEnrichPending_operationalSuccess(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")}}
	w := NewEnrichPendingWorker(st, f)
	w.TriggerType = store.TriggerCLI
	sum, err := w.Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Enriched != 1 || len(st.opRuns) != 1 {
		t.Fatalf("sum=%+v ops=%+v", sum, st.opRuns)
	}
	op := st.opRuns[0]
	if op.Status != store.OpStatusSucceeded || op.HasError ||
		op.SelectedCount == nil || *op.SelectedCount != 1 ||
		op.SucceededCount == nil || *op.SucceededCount != 1 {
		t.Fatalf("%+v", op)
	}
	blob := fmt.Sprintf("%#v", op)
	for _, bad := range []string{"https://", "http://", "ADMIN_API_TOKEN", "play_page"} {
		if strings.Contains(blob, bad) {
			t.Fatalf("unsafe %q in %#v", bad, op)
		}
	}
}

func TestEnrichPending_invalidTrigger(t *testing.T) {
	st := newEnrichMemStore()
	w := NewEnrichPendingWorker(st, &mapFetcher{})
	w.TriggerType = "weird"
	_, err := w.Run(context.Background(), 5, false, 1)
	if err == nil {
		t.Fatal("expected invalid trigger")
	}
	if st.locked || len(st.opRuns) != 0 {
		t.Fatal("must not lock/write")
	}
}

func TestEnrichPending_adminRequesterRulesAndAudit(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")}}

	w0 := NewEnrichPendingWorker(st, f)
	w0.TriggerType = store.TriggerAdmin
	w0.RequestedByUserID = 0
	if _, err := w0.Run(context.Background(), 5, false, 1); err == nil || st.locked || len(st.opRuns) != 0 {
		t.Fatal("admin without requester must fail early")
	}

	wCLI := NewEnrichPendingWorker(st, f)
	wCLI.TriggerType = store.TriggerCLI
	wCLI.RequestedByUserID = 3
	if _, err := wCLI.Run(context.Background(), 5, false, 1); err == nil {
		t.Fatal("cli with requester must fail")
	}

	wOK := NewEnrichPendingWorker(st, f)
	wOK.TriggerType = store.TriggerAdmin
	wOK.RequestedByUserID = 55
	sum, err := wOK.Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Enriched != 1 || len(st.opRuns) != 1 ||
		st.opRuns[0].RequestedByUserID == nil || *st.opRuns[0].RequestedByUserID != 55 {
		t.Fatalf("sum=%+v ops=%+v", sum, st.opRuns)
	}

	stBusy := newEnrichMemStore()
	stBusy.locked = true
	wBusy := NewEnrichPendingWorker(stBusy, f)
	wBusy.TriggerType = store.TriggerAdmin
	wBusy.RequestedByUserID = 88
	_, err = wBusy.Run(context.Background(), 5, false, 1)
	if err == nil || !errors.Is(err, store.ErrOperationLocked) {
		t.Fatalf("expected lock busy: %v", err)
	}
	if len(stBusy.opRuns) != 1 || stBusy.opRuns[0].RequestedByUserID == nil ||
		*stBusy.opRuns[0].RequestedByUserID != 88 || stBusy.opRuns[0].ErrorCode != store.ErrCodeLockBusy {
		t.Fatalf("reject audit: %+v", stBusy.opRuns)
	}
}

// cancelAfterFetch cancels the parent context after a successful body return so
// ctx.Err() is non-nil when Run finishes aggregating — must not invent a timeout return.
type cancelAfterFetch struct {
	inner  *mapFetcher
	cancel context.CancelFunc
}

func (f *cancelAfterFetch) Fetch(ctx context.Context, pageURL string) (string, error) {
	body, err := f.inner.Fetch(ctx, pageURL)
	if err == nil && f.cancel != nil {
		f.cancel()
	}
	return body, err
}

func TestEnrichPending_returnSemanticsAndAudit(t *testing.T) {
	t.Run("case1_all_success", func(t *testing.T) {
		st := newEnrichMemStore()
		st.episodes[1] = &store.PendingEnrichmentEpisode{
			ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
		}
		f := &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")}}
		sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
		if err != nil {
			t.Fatalf("pre-4A returns nil on success: %v", err)
		}
		if sum.Failures != 0 || sum.Enriched != 1 {
			t.Fatalf("%+v", sum)
		}
		if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusSucceeded || st.opRuns[0].HasError {
			t.Fatalf("%+v", st.opRuns)
		}
	})

	t.Run("case2_mixed_success_failure_nil_return", func(t *testing.T) {
		st := newEnrichMemStore()
		st.episodes[1] = &store.PendingEnrichmentEpisode{
			ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
		}
		st.episodes[2] = &store.PendingEnrichmentEpisode{
			ID: 2, VideoID: 2, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "b",
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p2",
		}
		f := &mapFetcher{
			pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")},
			err:   map[string]error{"https://src/p2": fmt.Errorf("fetch boom")},
		}
		sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
		if err != nil {
			t.Fatalf("pre-4A returns nil when Enriched>0 even with Failures: %v sum=%+v", err, sum)
		}
		if sum.Enriched < 1 || sum.Failures < 1 {
			t.Fatalf("expected mixed: %+v", sum)
		}
		if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusFailed ||
			st.opRuns[0].ErrorCode != store.ErrCodePartialFailure || !st.opRuns[0].HasError {
			t.Fatalf("audit must be partial_failure: %+v", st.opRuns)
		}
	})

	t.Run("case3_all_failed_nonzero_error", func(t *testing.T) {
		st := newEnrichMemStore()
		st.episodes[1] = &store.PendingEnrichmentEpisode{
			ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
		}
		f := &mapFetcher{pages: map[string]string{"https://src/p1": `<html>no player</html>`}}
		sum, err := NewEnrichPendingWorker(st, f).Run(context.Background(), 5, false, 1)
		if err == nil {
			t.Fatalf("pre-4A returns error when Failures>0 Enriched==0 Error==\"\": %+v", sum)
		}
		if sum.Enriched != 0 || sum.Failures < 1 {
			t.Fatalf("%+v", sum)
		}
		if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusFailed ||
			st.opRuns[0].ErrorCode != store.ErrCodePartialFailure || !st.opRuns[0].HasError {
			t.Fatalf("%+v", st.opRuns)
		}
	})

	t.Run("case4_post_work_ctx_done_no_invented_timeout", func(t *testing.T) {
		st := newEnrichMemStore()
		st.episodes[1] = &store.PendingEnrichmentEpisode{
			ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
		}
		ctx, cancel := context.WithCancel(context.Background())
		f := &cancelAfterFetch{
			inner:  &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")}},
			cancel: cancel,
		}
		sum, err := NewEnrichPendingWorker(st, f).Run(ctx, 5, false, 1)
		if err != nil {
			t.Fatalf("must not invent timeout/error after successful enrichment: %v sum=%+v", err, sum)
		}
		if sum.Enriched != 1 || sum.Failures != 0 {
			t.Fatalf("%+v", sum)
		}
		if ctx.Err() == nil {
			t.Fatal("expected ctx canceled after fetch for this case")
		}
		if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusSucceeded ||
			st.opRuns[0].ErrorCode == store.ErrCodeTimeout {
			t.Fatalf("must not classify post-work cancel as timeout: %+v", st.opRuns)
		}
	})

	t.Run("case5_deadline_during_dispatch_audit_only", func(t *testing.T) {
		st := newEnrichMemStore()
		st.episodes[1] = &store.PendingEnrichmentEpisode{
			ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
		}
		st.episodes[2] = &store.PendingEnrichmentEpisode{
			ID: 2, VideoID: 2, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "b",
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p2",
		}
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		f := &mapFetcher{pages: map[string]string{
			"https://src/p1": playHTML("https://cdn.example/1.m3u8"),
			"https://src/p2": playHTML("https://cdn.example/2.m3u8"),
		}}
		sum, err := NewEnrichPendingWorker(st, f).Run(ctx, 5, false, 1)
		// Pre-4A: ctx.Done sets Error+Failures; with Enriched==0 and Error!="", return is nil.
		if err != nil {
			t.Fatalf("pre-4A returns nil when Error already set from ctx.Done: %v sum=%+v", err, sum)
		}
		if sum.Enriched != 0 || sum.Failures < 1 || sum.Error == "" {
			t.Fatalf("%+v", sum)
		}
		if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusFailed ||
			st.opRuns[0].ErrorCode != store.ErrCodeTimeout {
			t.Fatalf("audit timeout only: %+v", st.opRuns)
		}
	})
}

func TestEnrichPending_selectsOnlyEligible(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	st.episodes[2] = &store.PendingEnrichmentEpisode{
		ID: 2, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 2, PlaybackStatus: "ok", PlaybackURL: "https://x", PlayPageURL: "https://src/p2",
	}
	st.episodes[3] = &store.PendingEnrichmentEpisode{
		ID: 3, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 3, PlaybackStatus: "pending_enrichment", PlaybackURL: "", PlayPageURL: "",
	}
	st.episodes[4] = &store.PendingEnrichmentEpisode{
		ID: 4, VideoID: 1, SourceSystem: "other", SourceVideoID: "a",
		SID: 1, NID: 4, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p4",
	}
	list, err := st.ListPendingEnrichmentEpisodes(olehdtv.SourceSystem, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != 1 {
		t.Fatalf("%+v", list)
	}
}

func TestClampEnrichPendingHTTPSettings(t *testing.T) {
	d, c, r := ClampEnrichPendingHTTPSettings(100, 9, 0)
	if d < 1000 || c > 2 || c < 1 || r <= 0 {
		t.Fatalf("%d %d %d", d, c, r)
	}
}

func TestSelectFairPendingBatch_enforcesMaxLimit(t *testing.T) {
	var cands []store.PendingEnrichmentEpisode
	for i := 1; i <= 50; i++ {
		cands = append(cands, store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), OkEpisodeCount: 0, NID: 1,
		})
	}
	got := SelectFairPendingBatch(cands, 100)
	if len(got) > EnrichPendingMaxLimit {
		t.Fatalf("len=%d", len(got))
	}
}

func TestEnrichPending_skipsRaceAlreadyFilled(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{pages: map[string]string{"https://src/p1": playHTML("https://cdn.example/1.m3u8")}}
	// Pretend another writer filled the row before Apply: wrap store
	wrapped := &raceStore{enrichMemStore: st}
	sum, err := NewEnrichPendingWorker(wrapped, f).Run(context.Background(), 5, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.SkippedExisting != 1 || sum.Enriched != 0 {
		t.Fatalf("%+v", sum)
	}
}

type raceStore struct{ *enrichMemStore }

func (r *raceStore) ApplyPlaybackEnrichment(id uint64, playbackURL, playbackSource, status string) (bool, error) {
	r.mu.Lock()
	if e := r.episodes[id]; e != nil {
		e.PlaybackURL = "https://already/set.m3u8"
		e.PlaybackStatus = "ok"
	}
	r.mu.Unlock()
	return r.enrichMemStore.ApplyPlaybackEnrichment(id, playbackURL, playbackSource, status)
}

func TestEnrichPending_prioritizeZeroOkInRun(t *testing.T) {
	st := newEnrichMemStore()
	// video 100 has ok episodes; video 200 has none
	st.okCounts[100] = 2
	st.okCounts[200] = 0
	for nid := 1; nid <= 5; nid++ {
		id := uint64(1000 + nid)
		st.episodes[id] = &store.PendingEnrichmentEpisode{
			ID: id, VideoID: 100, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "old",
			SID: 1, NID: nid, PlaybackStatus: "pending_enrichment",
			PlayPageURL: fmt.Sprintf("https://src/old/%d", nid),
		}
	}
	st.episodes[2001] = &store.PendingEnrichmentEpisode{
		ID: 2001, VideoID: 200, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "new",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/new/1",
	}
	pages := map[string]string{"https://src/new/1": playHTML("https://cdn.example/new1.m3u8")}
	for nid := 1; nid <= 5; nid++ {
		pages[fmt.Sprintf("https://src/old/%d", nid)] = playHTML(fmt.Sprintf("https://cdn.example/old%d.m3u8", nid))
	}
	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).Run(context.Background(), 1, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Enriched != 1 || st.episodes[2001].PlaybackStatus != "ok" {
		t.Fatalf("expected zero-ok video first: %+v ep=%+v", sum, st.episodes[2001])
	}
}
