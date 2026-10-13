package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

type memStore struct {
	mu               sync.Mutex
	locked           bool
	videos           map[string]*model.Video
	episodes         map[string]*store.EpisodeSyncRow
	nextVID          uint64
	nextEID          uint64
	deactivateCalled bool
	syncRuns         int
	videoUpdatedAt   map[uint64]time.Time
	videoCreatedAt   map[uint64]time.Time
	opRuns           []store.OperationalRunRow
	nextOpID         uint64
	crawlBlocked     bool // set true when lock busy path should prove no work
}

func newMemStore() *memStore {
	return &memStore{
		videos:         map[string]*model.Video{},
		episodes:       map[string]*store.EpisodeSyncRow{},
		nextVID:        1,
		nextEID:        1,
		videoUpdatedAt: map[uint64]time.Time{},
		videoCreatedAt: map[uint64]time.Time{},
	}
}

func epKey(sys, vod string, sid, nid int) string {
	return fmt.Sprintf("%s|%s|%d|%d", sys, vod, sid, nid)
}

func (m *memStore) AcquireAdvisoryLock(ctx context.Context, name string) (*store.AdvisoryHold, error) {
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

func (m *memStore) StartOperationalRun(in store.StartOperationalRunInput) (uint64, error) {
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

func (m *memStore) FinishOperationalRun(in store.FinishOperationalRunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.opRuns {
		if m.opRuns[i].ID == in.ID {
			now := time.Now()
			m.opRuns[i].Status = in.Status
			m.opRuns[i].FinishedAt = &now
			m.opRuns[i].Scanned = in.Scanned
			m.opRuns[i].CreatedCount = in.CreatedCount
			m.opRuns[i].UpdatedCount = in.UpdatedCount
			m.opRuns[i].UnchangedCount = in.UnchangedCount
			m.opRuns[i].FailedCount = in.FailedCount
			m.opRuns[i].ErrorCode = in.ErrorCode
			m.opRuns[i].HasError = in.HasError
			m.opRuns[i].SyncRunID = in.SyncRunID
			return nil
		}
	}
	return fmt.Errorf("op run %d not found", in.ID)
}

func (m *memStore) RejectOperationalRun(in store.RejectOperationalRunInput) (uint64, error) {
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

func (m *memStore) UpsertCategoryResult(in store.CategoryUpsert) (uint64, bool, bool, error) {
	_ = in
	return 1, false, false, nil
}

func (m *memStore) GetCategoryBySource(system, sourceID string) (*model.Category, error) {
	_ = system
	_ = sourceID
	return &model.Category{ID: 1, SourceID: sourceID}, nil
}

func (m *memStore) GetVideoBySource(system, sourceID string) (*model.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.videos[system+"|"+sourceID]
	if v == nil {
		return nil, nil
	}
	cp := *v
	return &cp, nil
}

func (m *memStore) InsertVideoCatalog(in store.VideoUpsert) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := in.SourceSystem + "|" + in.SourceID
	if _, ok := m.videos[key]; ok {
		return 0, errors.New("duplicate video")
	}
	id := m.nextVID
	m.nextVID++
	now := time.Now()
	m.videos[key] = &model.Video{
		ID: id, SourceSystem: in.SourceSystem, SourceID: in.SourceID,
		CategoryID: in.CategoryID, SourceCategoryID: in.SourceCategoryID,
		Title: in.Title, Description: in.Description, Year: in.Year, Area: in.Area,
		Director: in.Director, Actors: in.Actors, Rating: in.Rating,
		PosterSourceURL: in.PosterSourceURL, CoverKey: in.CoverKey, CoverR2Key: in.CoverR2Key,
		DurationSec: in.DurationSec, ViewCount: in.ViewCount,
		PlaybackSource: in.PlaybackSource, PlaySID: in.PlaySID, PlayNID: in.PlayNID,
		PlaybackURL: in.PlaybackURL, IsActive: in.IsActive, Status: in.Status,
		CreatedAt: now,
	}
	m.videoCreatedAt[id] = now
	m.videoUpdatedAt[id] = now
	return id, nil
}

func (m *memStore) UpdateVideoCatalog(id uint64, in store.VideoUpsert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.videos {
		if v.ID == id {
			v.Title = in.Title
			v.Description = in.Description
			v.CategoryID = in.CategoryID
			v.SourceCategoryID = in.SourceCategoryID
			v.Year = in.Year
			v.Area = in.Area
			v.Director = in.Director
			v.Actors = in.Actors
			v.Rating = in.Rating
			v.PosterSourceURL = in.PosterSourceURL
			v.PlaybackURL = in.PlaybackURL
			v.PlaybackSource = in.PlaybackSource
			v.PlaySID = in.PlaySID
			v.PlayNID = in.PlayNID
			v.ViewCount = in.ViewCount
			v.DurationSec = in.DurationSec
			v.IsActive = in.IsActive
			v.Status = in.Status
			m.videoUpdatedAt[id] = time.Now()
			return nil
		}
	}
	return errors.New("video not found")
}

func (m *memStore) GetEpisodeForSync(system, sourceVideoID string, sid, nid int) (*store.EpisodeSyncRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.episodes[epKey(system, sourceVideoID, sid, nid)]
	if e == nil {
		return nil, nil
	}
	cp := *e
	return &cp, nil
}

func (m *memStore) InsertEpisodeCatalog(in store.EpisodeUpsert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := epKey(in.SourceSystem, in.SourceVideoID, in.SID, in.NID)
	if _, ok := m.episodes[key]; ok {
		return errors.New("duplicate episode")
	}
	id := m.nextEID
	m.nextEID++
	now := time.Now()
	m.episodes[key] = &store.EpisodeSyncRow{
		ID: id, VideoID: in.VideoID, SID: in.SID, NID: in.NID, Title: in.Title,
		PlaybackSource: in.PlaybackSource, PlaybackURL: in.PlaybackURL, PlayPageURL: in.PlayPageURL,
		PlaybackStatus: in.PlaybackStatus, IsActive: in.IsActive,
		CreatedAt: now, UpdatedAt: now,
	}
	return nil
}

func (m *memStore) UpdateEpisodeCatalog(id uint64, in store.EpisodeUpsert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.episodes {
		if e.ID == id {
			// preserve migration fields
			hls, prov, mig := e.HlsObjectKey, e.StorageProvider, e.MigrationStatus
			e.Title = in.Title
			e.PlaybackSource = in.PlaybackSource
			e.PlaybackURL = in.PlaybackURL
			e.PlayPageURL = in.PlayPageURL
			e.PlaybackStatus = in.PlaybackStatus
			e.IsActive = in.IsActive
			e.VideoID = in.VideoID
			e.UpdatedAt = time.Now()
			e.HlsObjectKey, e.StorageProvider, e.MigrationStatus = hls, prov, mig
			return nil
		}
	}
	return errors.New("episode not found")
}

func (m *memStore) InsertSyncRunDetailed(system, trigger string, scanned, created, updated, unchanged, failed, deactivated int, summary any, errText string) error {
	_ = system
	_ = trigger
	_ = scanned
	_ = created
	_ = updated
	_ = unchanged
	_ = failed
	_ = deactivated
	_ = summary
	_ = errText
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncRuns++
	return nil
}

func (m *memStore) DeactivateMissingVideos(system string, keep []string) (int, error) {
	_ = system
	_ = keep
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deactivateCalled = true
	return 0, nil
}

type memSource struct {
	videos []olehdtv.SourceVideo
	cats   []olehdtv.SourceCategory
}

func (s *memSource) Name() string { return "mem" }
func (s *memSource) ListCategories(ctx context.Context) ([]olehdtv.SourceCategory, error) {
	_ = ctx
	return s.cats, nil
}
func (s *memSource) ListRecentVideos(ctx context.Context, limit int) ([]olehdtv.SourceVideo, olehdtv.RecentMeta, error) {
	_ = ctx
	out := s.videos
	if limit < len(out) {
		out = out[:limit]
	}
	return out, olehdtv.RecentMeta{Note: "test", ReliableWatermark: false}, nil
}

func sampleVideo(id, title, url string) olehdtv.SourceVideo {
	return olehdtv.SourceVideo{
		SourceID: id, SourceCategoryID: "1", Title: title, Description: "d",
		Year: "2026", IsActive: true,
		Episodes: []olehdtv.SourceEpisode{{SID: 1, NID: 1, Title: "正片", PlaybackSource: "plyr", PlaybackURL: url}},
	}
}

func TestIncremental_newAndUnchangedAndUpdate(t *testing.T) {
	st := newMemStore()
	src := &memSource{
		cats:   []olehdtv.SourceCategory{{SourceID: "1", Name: "电影", Slug: "movie"}},
		videos: []olehdtv.SourceVideo{sampleVideo("100", "A", "https://example.com/a.m3u8")},
	}
	svc := NewIncrementalSync(st, src)

	sum, err := svc.Run(context.Background(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.NewVideos != 1 || sum.NewEpisodes != 1 {
		t.Fatalf("create: %+v", sum)
	}
	createdAt := st.videoCreatedAt[1]
	updatedAt := st.videoUpdatedAt[1]

	sum2, err := svc.Run(context.Background(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum2.UnchangedVideos != 1 || sum2.UnchangedEpisodes != 1 || sum2.UpdatedVideos != 0 {
		t.Fatalf("unchanged: %+v", sum2)
	}
	if !st.videoCreatedAt[1].Equal(createdAt) {
		t.Fatal("created_at changed")
	}
	if !st.videoUpdatedAt[1].Equal(updatedAt) {
		t.Fatal("updated_at bumped on unchanged video")
	}

	src.videos[0].Title = "A2"
	sum3, err := svc.Run(context.Background(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum3.UpdatedVideos != 1 || sum3.UnchangedVideos != 0 {
		t.Fatalf("update video: %+v", sum3)
	}
}

func TestIncremental_episodePlaybackChangePreservesMigration(t *testing.T) {
	st := newMemStore()
	if _, err := st.InsertVideoCatalog(store.VideoUpsert{
		SourceSystem: olehdtv.SourceSystem, SourceID: "53v", Title: "T", CategoryID: 1,
		IsActive: true, Status: "ready", PlaybackURL: "https://old.example/a.m3u8",
	}); err != nil {
		t.Fatal(err)
	}
	key := epKey(olehdtv.SourceSystem, "53v", 1, 1)
	st.episodes[key] = &store.EpisodeSyncRow{
		ID: 53, VideoID: 1, SID: 1, NID: 1, Title: "正片",
		PlaybackURL: "https://old.example/a.m3u8", PlaybackStatus: "ok", IsActive: true,
		HlsObjectKey: "videos/12/53/index.m3u8", StorageProvider: "r2", MigrationStatus: "done",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	src := &memSource{
		cats:   []olehdtv.SourceCategory{{SourceID: "1", Name: "电影", Slug: "movie"}},
		videos: []olehdtv.SourceVideo{sampleVideo("53v", "T", "https://new.example/a.m3u8")},
	}
	svc := NewIncrementalSync(st, src)
	sum, err := svc.Run(context.Background(), 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.UpdatedEpisodes != 1 || sum.MediaSourceChanged != 1 {
		t.Fatalf("sum=%+v", sum)
	}
	ep := st.episodes[key]
	if ep.HlsObjectKey != "videos/12/53/index.m3u8" || ep.StorageProvider != "r2" || ep.MigrationStatus != "done" {
		t.Fatalf("migration damaged: %+v", ep)
	}
	if ep.PlaybackURL != "https://new.example/a.m3u8" {
		t.Fatalf("playback not updated: %s", ep.PlaybackURL)
	}
	found := false
	for _, a := range sum.Actions {
		if a.Detail == "MEDIA_SOURCE_CHANGED" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected MEDIA_SOURCE_CHANGED action")
	}
}

func TestIncremental_dryRunNoWrites(t *testing.T) {
	st := newMemStore()
	src := &memSource{
		cats:   []olehdtv.SourceCategory{{SourceID: "1", Name: "电影", Slug: "movie"}},
		videos: []olehdtv.SourceVideo{sampleVideo("9", "X", "https://example.com/x.m3u8")},
	}
	svc := NewIncrementalSync(st, src)
	sum, err := svc.Run(context.Background(), 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if sum.NewVideos != 1 || len(st.videos) != 0 || st.syncRuns != 0 || len(st.opRuns) != 0 {
		t.Fatalf("dry-run wrote state: videos=%d runs=%d ops=%d sum=%+v", len(st.videos), st.syncRuns, len(st.opRuns), sum)
	}
}

func TestIncremental_noDuplicateInsertion(t *testing.T) {
	st := newMemStore()
	src := &memSource{
		cats:   []olehdtv.SourceCategory{{SourceID: "1", Name: "电影", Slug: "movie"}},
		videos: []olehdtv.SourceVideo{sampleVideo("7", "Y", "https://example.com/y.m3u8")},
	}
	svc := NewIncrementalSync(st, src)
	if _, err := svc.Run(context.Background(), 5, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Run(context.Background(), 5, false); err != nil {
		t.Fatal(err)
	}
	if len(st.videos) != 1 || len(st.episodes) != 1 {
		t.Fatalf("dupes videos=%d episodes=%d", len(st.videos), len(st.episodes))
	}
}

func TestIncremental_lockBusy(t *testing.T) {
	st := newMemStore()
	st.locked = true
	svc := NewIncrementalSync(st, &memSource{videos: []olehdtv.SourceVideo{sampleVideo("1", "a", "u")}})
	sum, err := svc.Run(context.Background(), 1, true)
	if err == nil || !errors.Is(err, store.ErrOperationLocked) || sum.LockAcquired {
		t.Fatalf("expected ErrOperationLocked, sum=%+v err=%v", sum, err)
	}
	if len(st.opRuns) != 0 {
		t.Fatal("dry-run must not write reject rows")
	}
}

func TestIncremental_lockBusyRecordsReject(t *testing.T) {
	st := newMemStore()
	st.locked = true
	svc := NewIncrementalSync(st, &memSource{videos: []olehdtv.SourceVideo{sampleVideo("1", "a", "u")}})
	svc.TriggerType = store.TriggerTimer
	sum, err := svc.Run(context.Background(), 1, false)
	if err == nil || !errors.Is(err, store.ErrOperationLocked) || sum.LockAcquired {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if len(st.videos) != 0 {
		t.Fatal("lock busy must not crawl/write catalog")
	}
	if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusRejected ||
		st.opRuns[0].ErrorCode != store.ErrCodeLockBusy || st.opRuns[0].TriggerType != store.TriggerTimer ||
		st.opRuns[0].JobType != store.JobIncrementalSync {
		t.Fatalf("opRuns=%+v", st.opRuns)
	}
	assertOpRunSafe(t, st.opRuns[0])
}

func testIncSource() *memSource {
	return &memSource{
		cats:   []olehdtv.SourceCategory{{SourceID: "1", Name: "电影", Slug: "movie"}},
		videos: []olehdtv.SourceVideo{sampleVideo("100", "A", "https://example.com/a.m3u8")},
	}
}

func TestIncremental_operationalSuccessAndPartial(t *testing.T) {
	st := newMemStore()
	svc := NewIncrementalSync(st, testIncSource())
	svc.TriggerType = store.TriggerCLI
	sum, err := svc.Run(context.Background(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.LockAcquired || len(st.opRuns) != 1 {
		t.Fatalf("sum=%+v ops=%+v", sum, st.opRuns)
	}
	op := st.opRuns[0]
	if op.Status != store.OpStatusSucceeded || op.HasError || op.ErrorCode != "" {
		t.Fatalf("%+v", op)
	}
	if op.Scanned == nil || *op.Scanned != sum.Discovered ||
		op.CreatedCount == nil || *op.CreatedCount != sum.NewVideos {
		t.Fatalf("counters %+v vs sum %+v", op, sum)
	}
	assertOpRunSafe(t, op)

	st2 := &failInsertStore{memStore: *newMemStore()}
	svc2 := NewIncrementalSync(st2, testIncSource())
	_, err = svc2.Run(context.Background(), 5, false)
	if err == nil {
		t.Fatal("expected partial failure")
	}
	if len(st2.opRuns) != 1 || st2.opRuns[0].Status != store.OpStatusFailed ||
		st2.opRuns[0].ErrorCode != store.ErrCodePartialFailure || !st2.opRuns[0].HasError {
		t.Fatalf("%+v", st2.opRuns)
	}
	assertOpRunSafe(t, st2.opRuns[0])
}

func TestIncremental_invalidTrigger(t *testing.T) {
	st := newMemStore()
	svc := NewIncrementalSync(st, testIncSource())
	svc.TriggerType = "evil"
	_, err := svc.Run(context.Background(), 1, false)
	if err == nil {
		t.Fatal("expected invalid trigger")
	}
	if st.locked || len(st.opRuns) != 0 {
		t.Fatal("must not lock or write on bad trigger")
	}
}

func TestIncremental_adminRequesterRulesAndAudit(t *testing.T) {
	st := newMemStore()
	svc := NewIncrementalSync(st, testIncSource())
	svc.TriggerType = store.TriggerAdmin
	svc.RequestedByUserID = 0
	_, err := svc.Run(context.Background(), 1, false)
	if err == nil || st.locked || len(st.opRuns) != 0 {
		t.Fatalf("admin without requester must fail early: err=%v locked=%v ops=%d", err, st.locked, len(st.opRuns))
	}

	st2 := newMemStore()
	svc2 := NewIncrementalSync(st2, testIncSource())
	svc2.TriggerType = store.TriggerCLI
	svc2.RequestedByUserID = 9
	_, err = svc2.Run(context.Background(), 1, false)
	if err == nil || st2.locked || len(st2.opRuns) != 0 {
		t.Fatal("cli with requester must fail early")
	}

	st3 := newMemStore()
	svc3 := NewIncrementalSync(st3, testIncSource())
	svc3.TriggerType = store.TriggerAdmin
	svc3.RequestedByUserID = 42
	sum, err := svc3.Run(context.Background(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.LockAcquired || len(st3.opRuns) != 1 {
		t.Fatalf("sum=%+v ops=%+v", sum, st3.opRuns)
	}
	if st3.opRuns[0].RequestedByUserID == nil || *st3.opRuns[0].RequestedByUserID != 42 {
		t.Fatalf("requester not audited: %+v", st3.opRuns[0])
	}

	st4 := newMemStore()
	st4.locked = true
	svc4 := NewIncrementalSync(st4, testIncSource())
	svc4.TriggerType = store.TriggerAdmin
	svc4.RequestedByUserID = 77
	_, err = svc4.Run(context.Background(), 1, false)
	if err == nil || !errors.Is(err, store.ErrOperationLocked) {
		t.Fatalf("expected lock busy: %v", err)
	}
	if len(st4.opRuns) != 1 || st4.opRuns[0].Status != store.OpStatusRejected ||
		st4.opRuns[0].ErrorCode != store.ErrCodeLockBusy ||
		st4.opRuns[0].RequestedByUserID == nil || *st4.opRuns[0].RequestedByUserID != 77 {
		t.Fatalf("admin reject audit: %+v", st4.opRuns)
	}
}

func assertOpRunSafe(t *testing.T, op store.OperationalRunRow) {
	t.Helper()
	blob := fmt.Sprintf("%#v", op)
	for _, bad := range []string{
		"http://", "https://", "ADMIN_API_TOKEN", "mysql://", "password=", "SENSITIVE",
	} {
		if strings.Contains(blob, bad) {
			t.Fatalf("unsafe content %q in %#v", bad, op)
		}
	}
}

type failInsertStore struct {
	memStore
}

func (m *failInsertStore) InsertVideoCatalog(in store.VideoUpsert) (uint64, error) {
	return 0, fmt.Errorf("insert failed")
}

func TestIncremental_neverCallsDeactivate(t *testing.T) {
	st := newMemStore()
	svc := NewIncrementalSync(st, &memSource{
		cats:   []olehdtv.SourceCategory{{SourceID: "1", Name: "电影", Slug: "movie"}},
		videos: []olehdtv.SourceVideo{sampleVideo("1", "a", "https://e/a.m3u8")},
	})
	if _, err := svc.Run(context.Background(), 1, false); err != nil {
		t.Fatal(err)
	}
	if st.deactivateCalled {
		t.Fatal("deactivate should not be called")
	}
}

func TestIncremental_neverWipesGoodPlaybackOrMigrated(t *testing.T) {
	st := newMemStore()
	if _, err := st.InsertVideoCatalog(store.VideoUpsert{
		SourceSystem: olehdtv.SourceSystem, SourceID: "keep1", Title: "K", CategoryID: 1,
		IsActive: true, Status: "ready", PlaybackURL: "https://keep.example/1.m3u8",
	}); err != nil {
		t.Fatal(err)
	}
	key := epKey(olehdtv.SourceSystem, "keep1", 1, 1)
	st.episodes[key] = &store.EpisodeSyncRow{
		ID: 1, VideoID: 1, SID: 1, NID: 1, Title: "正片",
		PlaybackURL: "https://keep.example/1.m3u8", PlaybackStatus: "ok", IsActive: true,
		HlsObjectKey: "videos/1/1/index.m3u8", StorageProvider: "r2", MigrationStatus: "done",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	src := &memSource{
		cats: []olehdtv.SourceCategory{{SourceID: "1", Name: "电影", Slug: "movie"}},
		videos: []olehdtv.SourceVideo{{
			SourceID: "keep1", SourceCategoryID: "1", Title: "K", IsActive: true,
			Episodes: []olehdtv.SourceEpisode{
				{SID: 1, NID: 1, Title: "正片", PlaybackStatus: "unavailable", PlaybackURL: ""},
			},
		}},
	}
	svc := NewIncrementalSync(st, src)
	if _, err := svc.Run(context.Background(), 5, false); err != nil {
		t.Fatal(err)
	}
	ep := st.episodes[key]
	if ep.PlaybackURL != "https://keep.example/1.m3u8" || ep.PlaybackStatus != "ok" || !ep.IsActive {
		t.Fatalf("good playback wiped: %+v", ep)
	}
	if ep.HlsObjectKey != "videos/1/1/index.m3u8" || ep.MigrationStatus != "done" {
		t.Fatalf("migration touched: %+v", ep)
	}
}

func TestIncremental_pendingEnrichmentDoesNotWipeURL(t *testing.T) {
	st := newMemStore()
	if _, err := st.InsertVideoCatalog(store.VideoUpsert{
		SourceSystem: olehdtv.SourceSystem, SourceID: "keep2", Title: "K2", CategoryID: 1,
		IsActive: true, Status: "ready",
	}); err != nil {
		t.Fatal(err)
	}
	key := epKey(olehdtv.SourceSystem, "keep2", 1, 2)
	st.episodes[key] = &store.EpisodeSyncRow{
		ID: 2, VideoID: 1, SID: 1, NID: 2, Title: "第2集",
		PlaybackURL: "https://keep.example/2.m3u8", PlaybackStatus: "ok", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	src := &memSource{
		cats: []olehdtv.SourceCategory{{SourceID: "1", Name: "连续剧", Slug: "tv"}},
		videos: []olehdtv.SourceVideo{{
			SourceID: "keep2", SourceCategoryID: "1", Title: "K2", IsActive: true,
			Episodes: []olehdtv.SourceEpisode{
				{SID: 1, NID: 2, Title: "第2集", PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p2"},
			},
		}},
	}
	if _, err := NewIncrementalSync(st, src).Run(context.Background(), 5, false); err != nil {
		t.Fatal(err)
	}
	ep := st.episodes[key]
	if ep.PlaybackURL != "https://keep.example/2.m3u8" || ep.PlaybackStatus != "ok" {
		t.Fatalf("%+v", ep)
	}
}

func TestIncremental_partialEpisodeObservationDoesNotDeactivate(t *testing.T) {
	st := newMemStore()
	if _, err := st.InsertVideoCatalog(store.VideoUpsert{
		SourceSystem: olehdtv.SourceSystem, SourceID: "series1", Title: "S", CategoryID: 1,
		IsActive: true, Status: "ready", PlaybackURL: "https://old.example/1.m3u8",
	}); err != nil {
		t.Fatal(err)
	}
	for nid, url := range map[int]string{1: "https://old.example/1.m3u8", 2: "https://old.example/2.m3u8", 50: "https://old.example/50.m3u8"} {
		key := epKey(olehdtv.SourceSystem, "series1", 1, nid)
		st.episodes[key] = &store.EpisodeSyncRow{
			ID: uint64(nid), VideoID: 1, SID: 1, NID: nid, Title: fmt.Sprintf("第%d集", nid),
			PlaybackURL: url, PlaybackStatus: "ok", IsActive: true,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
	}
	// Bounded recent only "sees" ep1 pending + ep2 with new URL; ep50 omitted entirely.
	src := &memSource{
		cats: []olehdtv.SourceCategory{{SourceID: "1", Name: "连续剧", Slug: "tv"}},
		videos: []olehdtv.SourceVideo{{
			SourceID: "series1", SourceCategoryID: "1", Title: "S", IsActive: true,
			Episodes: []olehdtv.SourceEpisode{
				{SID: 1, NID: 1, Title: "第1集", PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1"},
				{SID: 1, NID: 2, Title: "第2集", PlaybackURL: "https://new.example/2.m3u8", PlaybackStatus: "ok"},
			},
		}},
	}
	svc := NewIncrementalSync(st, src)
	sum, err := svc.Run(context.Background(), 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.deactivateCalled || sum.DeactivateMissingCalled {
		t.Fatal("must not deactivate missing")
	}
	ep1 := st.episodes[epKey(olehdtv.SourceSystem, "series1", 1, 1)]
	if ep1.PlaybackURL != "https://old.example/1.m3u8" || !ep1.IsActive || ep1.PlaybackStatus != "ok" {
		t.Fatalf("ep1 damaged by partial observation: %+v", ep1)
	}
	ep50 := st.episodes[epKey(olehdtv.SourceSystem, "series1", 1, 50)]
	if ep50 == nil || !ep50.IsActive || ep50.PlaybackURL != "https://old.example/50.m3u8" {
		t.Fatalf("omitted ep50 must remain: %+v", ep50)
	}
	ep2 := st.episodes[epKey(olehdtv.SourceSystem, "series1", 1, 2)]
	if ep2.PlaybackURL != "https://new.example/2.m3u8" {
		t.Fatalf("positively observed ep2 should update: %+v", ep2)
	}
}

func TestNewRecentSource_maccmsRecentRequiresBaseURL(t *testing.T) {
	_, err := olehdtv.NewRecentSourceOpts(olehdtv.RecentSourceOpts{Mode: "maccms_recent"})
	if err == nil {
		t.Fatal("expected base URL error")
	}
	src, err := olehdtv.NewRecentSourceOpts(olehdtv.RecentSourceOpts{
		Mode: "maccms_recent", BaseURL: "https://maccms.local", RequestDelayMs: 50, MaxConcurrency: 99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src.Name(), "maccms_recent") {
		t.Fatalf("name=%s", src.Name())
	}
}

func TestVideoCatalogChanged(t *testing.T) {
	v := &model.Video{Title: "A", Status: "ready", IsActive: true}
	in := store.VideoUpsert{Title: "A", Status: "ready", IsActive: true}
	if VideoCatalogChanged(v, in) {
		t.Fatal("expected unchanged")
	}
	in.Title = "B"
	if !VideoCatalogChanged(v, in) {
		t.Fatal("expected changed")
	}
}

func TestEpisodeCatalogChangedAndMedia(t *testing.T) {
	ex := &store.EpisodeSyncRow{
		Title: "正片", PlaybackURL: "https://a", PlaybackStatus: "ok", IsActive: true,
		HlsObjectKey: "k", MigrationStatus: "done",
	}
	ep := olehdtv.SourceEpisode{Title: "正片", PlaybackURL: "https://a", PlaybackStatus: "ok"}
	if EpisodeCatalogChanged(ex, ep) {
		t.Fatal("unchanged")
	}
	ep.PlaybackURL = "https://b"
	if !EpisodeCatalogChanged(ex, ep) || !EpisodeMediaSourceChanged(ex, ep) {
		t.Fatal("expected media source change")
	}
}

func TestNewRecentSourceRejectsHTMLDump(t *testing.T) {
	_, err := olehdtv.NewRecentSource("html_dump", "", "", "", "x", "https://maccms.local")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFixtureRecentRespectsLimit(t *testing.T) {
	src, err := olehdtv.NewRecentSource("fixture", "", "", "../../testdata/olehdtv_sample.json", "", "")
	if err != nil {
		t.Fatal(err)
	}
	vids, _, err := src.ListRecentVideos(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(vids) != 3 {
		t.Fatalf("got %d", len(vids))
	}
}
