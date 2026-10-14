package mediamigrate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

type migrateMemStore struct {
	mu           sync.Mutex
	locked       bool
	busy         bool
	released     int
	episodes     []store.MigratableEpisode
	byID         map[uint64]store.MigratableEpisode
	listCalls    int
	getByIDCalls int
	lastListLim  int
	opRuns       []store.OperationalRunRow
	nextOpID     uint64
}

func (m *migrateMemStore) AcquireAdvisoryLock(ctx context.Context, name string) (*store.AdvisoryHold, error) {
	_ = ctx
	if name != store.MediaMigrateLockName && name != "test-lock" {
		return nil, errors.New("unexpected lock name: " + name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy || m.locked {
		return nil, store.ErrOperationLocked
	}
	m.locked = true
	return store.NewTestAdvisoryHold(func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.locked = false
		m.released++
		return nil
	}), nil
}

func (m *migrateMemStore) ListMigratableEpisodes(limit int, episodeID uint64) ([]store.MigratableEpisode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listCalls++
	m.lastListLim = limit
	_ = episodeID
	out := make([]store.MigratableEpisode, 0, limit)
	for _, ep := range m.episodes {
		if len(out) >= limit {
			break
		}
		out = append(out, ep)
	}
	return out, nil
}

func (m *migrateMemStore) GetMigratableEpisodeByID(id uint64) (*store.MigratableEpisode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getByIDCalls++
	if m.byID == nil {
		return nil, nil
	}
	ep, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	cp := ep
	return &cp, nil
}

func (m *migrateMemStore) StartOperationalRun(in store.StartOperationalRunInput) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextOpID++
	id := m.nextOpID
	now := time.Now()
	m.opRuns = append(m.opRuns, store.OperationalRunRow{
		ID:                id,
		JobType:           in.JobType,
		TriggerType:       in.TriggerType,
		Status:            store.OpStatusRunning,
		RequestedByUserID: in.RequestedByUserID,
		RequestedAt:       now,
		StartedAt:         &now,
	})
	return id, nil
}

func (m *migrateMemStore) FinishOperationalRun(in store.FinishOperationalRunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for i := range m.opRuns {
		if m.opRuns[i].ID == in.ID {
			m.opRuns[i].Status = in.Status
			m.opRuns[i].FinishedAt = &now
			m.opRuns[i].SelectedCount = in.SelectedCount
			m.opRuns[i].SucceededCount = in.SucceededCount
			m.opRuns[i].SkippedCount = in.SkippedCount
			m.opRuns[i].FailedCount = in.FailedCount
			m.opRuns[i].ErrorCode = in.ErrorCode
			m.opRuns[i].HasError = in.HasError
			return nil
		}
	}
	return errors.New("op run not found")
}

func (m *migrateMemStore) RejectOperationalRun(in store.RejectOperationalRunInput) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextOpID++
	id := m.nextOpID
	now := time.Now()
	m.opRuns = append(m.opRuns, store.OperationalRunRow{
		ID:                id,
		JobType:           in.JobType,
		TriggerType:       in.TriggerType,
		Status:            store.OpStatusRejected,
		RequestedByUserID: in.RequestedByUserID,
		RequestedAt:       now,
		StartedAt:         &now,
		FinishedAt:        &now,
		ErrorCode:         in.ErrorCode,
		HasError:          true,
	})
	return id, nil
}

func failHTTPClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
}

func testEpisode(id uint64) store.MigratableEpisode {
	return store.MigratableEpisode{
		ID: id, VideoID: 10 + id, PlaybackURL: "https://src.example/v/index.m3u8",
		PlaybackStatus: "ok", MigrationStatus: "",
	}
}

func TestBatchRunner_lockPreventsConcurrentOwnership(t *testing.T) {
	st := &migrateMemStore{busy: true}
	mig := &Migrator{Store: NewMemoryObjectStore(), Episodes: &fakeEpisodes{}, HTTP: migrateHTTPClient(),
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r := NewBatchRunner(st, mig)
	sum, err := r.Run(context.Background(), RunOptions{Execute: true, Limit: 1})
	if err == nil || !errors.Is(err, store.ErrOperationLocked) || sum.LockAcquired {
		t.Fatalf("expected lock busy: sum=%+v err=%v", sum, err)
	}
	if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusRejected ||
		st.opRuns[0].ErrorCode != store.ErrCodeLockBusy || st.opRuns[0].JobType != store.JobMediaMigration {
		t.Fatalf("reject audit: %+v", st.opRuns)
	}
}

func TestBatchRunner_lockReleasedAfterSuccessAndError(t *testing.T) {
	st := &migrateMemStore{episodes: []store.MigratableEpisode{testEpisode(1)}}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	mig := &Migrator{Store: NewMemoryObjectStore(), Episodes: eps, HTTP: migrateHTTPClient(),
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r := NewBatchRunner(st, mig)
	sum, err := r.Run(context.Background(), RunOptions{Execute: true, Limit: 1})
	if err != nil || !sum.LockAcquired || sum.Migrated != 1 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if st.locked || st.released != 1 {
		t.Fatalf("lock not released: locked=%v released=%d", st.locked, st.released)
	}

	st2 := &migrateMemStore{episodes: []store.MigratableEpisode{testEpisode(2)}}
	mig2 := &Migrator{Store: NewMemoryObjectStore(), Episodes: &fakeEpisodes{playback: "https://src.example/v/index.m3u8"},
		HTTP: failHTTPClient(), SourceRetryBase: time.Millisecond, SourceMaxAttempts: 1,
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r2 := NewBatchRunner(st2, mig2)
	sum2, err2 := r2.Run(context.Background(), RunOptions{Execute: true, Limit: 1})
	if err2 == nil || sum2.Failed != 1 {
		t.Fatalf("expected failure: sum=%+v err=%v", sum2, err2)
	}
	if st2.locked || st2.released != 1 {
		t.Fatalf("lock not released after error: locked=%v released=%d", st2.locked, st2.released)
	}
}

func TestBatchRunner_dryRunNoOperationalRun(t *testing.T) {
	st := &migrateMemStore{busy: true}
	mig := &Migrator{Store: NewMemoryObjectStore(), Episodes: &fakeEpisodes{}, HTTP: migrateHTTPClient(),
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r := NewBatchRunner(st, mig)
	_, err := r.Run(context.Background(), RunOptions{DryRun: true, Limit: 1})
	if err == nil || !errors.Is(err, store.ErrOperationLocked) {
		t.Fatalf("err=%v", err)
	}
	if len(st.opRuns) != 0 {
		t.Fatalf("dry-run must not audit lock-busy: %+v", st.opRuns)
	}
}

func TestBatchRunner_successAuditCounters(t *testing.T) {
	st := &migrateMemStore{episodes: []store.MigratableEpisode{testEpisode(1)}}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	mig := &Migrator{Store: NewMemoryObjectStore(), Episodes: eps, HTTP: migrateHTTPClient(),
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r := NewBatchRunner(st, mig)
	r.TriggerType = store.TriggerTimer
	sum, err := r.Run(context.Background(), RunOptions{Execute: true, Limit: 1})
	if err != nil || sum.Migrated != 1 || sum.Selected != 1 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if len(st.opRuns) != 1 {
		t.Fatalf("ops=%+v", st.opRuns)
	}
	op := st.opRuns[0]
	if op.JobType != store.JobMediaMigration || op.TriggerType != store.TriggerTimer ||
		op.Status != store.OpStatusSucceeded || op.HasError ||
		op.SelectedCount == nil || *op.SelectedCount != 1 ||
		op.SucceededCount == nil || *op.SucceededCount != 1 ||
		op.FailedCount == nil || *op.FailedCount != 0 {
		t.Fatalf("audit=%+v", op)
	}
}

func TestBatchRunner_failedAuditCounters(t *testing.T) {
	st := &migrateMemStore{episodes: []store.MigratableEpisode{testEpisode(1), testEpisode(2)}}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	// First succeeds via real HTTP; second fails — use limit 2 with one-shot fail after first.
	// Simpler: both fail.
	mig := &Migrator{Store: NewMemoryObjectStore(), Episodes: eps, HTTP: failHTTPClient(),
		SourceRetryBase: time.Millisecond, SourceMaxAttempts: 1,
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r := NewBatchRunner(st, mig)
	sum, err := r.Run(context.Background(), RunOptions{Execute: true, Limit: 2})
	if err == nil || sum.Failed != 2 || sum.Migrated != 0 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if len(st.opRuns) != 1 || st.opRuns[0].Status != store.OpStatusFailed ||
		st.opRuns[0].ErrorCode != store.ErrCodePartialFailure || !st.opRuns[0].HasError ||
		st.opRuns[0].FailedCount == nil || *st.opRuns[0].FailedCount != 2 ||
		st.opRuns[0].SelectedCount == nil || *st.opRuns[0].SelectedCount != 2 {
		t.Fatalf("audit=%+v", st.opRuns)
	}
}

func TestBatchRunner_explicitEpisodeIDBypassesList(t *testing.T) {
	ep := testEpisode(77)
	ep.MigrationStatus = "failed" // would be cooldown-filtered in batch list
	st := &migrateMemStore{
		episodes: nil, // batch list empty
		byID:     map[uint64]store.MigratableEpisode{77: ep},
	}
	eps := &fakeEpisodes{playback: ep.PlaybackURL}
	mig := &Migrator{Store: NewMemoryObjectStore(), Episodes: eps, HTTP: migrateHTTPClient(),
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r := NewBatchRunner(st, mig)
	sum, err := r.Run(context.Background(), RunOptions{Execute: true, Limit: 1, EpisodeID: 77})
	if err != nil || sum.Migrated != 1 || sum.Selected != 1 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if st.listCalls != 0 || st.getByIDCalls != 1 {
		t.Fatalf("expected GetByID only: list=%d get=%d", st.listCalls, st.getByIDCalls)
	}
}

func TestBatchRunner_defaultLimitUsesCallerLimit(t *testing.T) {
	st := &migrateMemStore{episodes: []store.MigratableEpisode{testEpisode(1), testEpisode(2)}}
	mig := &Migrator{Store: NewMemoryObjectStore(), Episodes: &fakeEpisodes{playback: "https://src.example/v/index.m3u8"},
		HTTP: migrateHTTPClient(), CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	r := NewBatchRunner(st, mig)
	sum, err := r.Run(context.Background(), RunOptions{DryRun: true, Limit: 1})
	if err != nil || sum.Selected != 1 || st.lastListLim != 1 {
		t.Fatalf("default path limit=1: sum=%+v listLim=%d err=%v", sum, st.lastListLim, err)
	}
}

func TestMediaMigrateLockNameStable(t *testing.T) {
	if store.MediaMigrateLockName != "vplayer_media_migrate" {
		t.Fatalf("lock name=%q", store.MediaMigrateLockName)
	}
	if NewBatchRunner(&migrateMemStore{}, &Migrator{}).LockName != store.MediaMigrateLockName {
		t.Fatal("runner must default to MediaMigrateLockName")
	}
}
