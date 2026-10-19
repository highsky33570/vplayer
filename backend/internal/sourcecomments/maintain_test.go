package sourcecomments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

type memMaint struct {
	videos     []store.SourceVideoCandidate
	cycles     []store.MaintenanceCycle
	checks     []store.MaintenanceCheckpoint
	nextID     uint64
	listLimits []int
}

func (m *memMaint) LatestMaintenanceCycle(sourceSystem, mode string) (*store.MaintenanceCycle, error) {
	var found *store.MaintenanceCycle
	for i := range m.cycles {
		row := &m.cycles[i]
		if row.SourceSystem == sourceSystem && row.Mode == mode && (found == nil || row.ID > found.ID) {
			found = row
		}
	}
	if found == nil {
		return nil, nil
	}
	copy := *found
	return &copy, nil
}

func (m *memMaint) CreateMaintenanceCycle(row store.MaintenanceCycle, now time.Time) (store.MaintenanceCycle, error) {
	m.nextID++
	row.ID = m.nextID
	if row.StartedAt.IsZero() {
		row.StartedAt = now
	}
	m.cycles = append(m.cycles, row)
	return row, nil
}

func (m *memMaint) SaveMaintenanceCycleStatus(id uint64, status, errorMessage string, completedAt, now time.Time) error {
	for i := range m.cycles {
		if m.cycles[i].ID == id {
			m.cycles[i].Status = status
			m.cycles[i].ErrorMessage = errorMessage
			m.cycles[i].CompletedAt = completedAt
			_ = now
			return nil
		}
	}
	return fmt.Errorf("missing cycle %d", id)
}

func (m *memMaint) MaxEligibleVideoID(sourceSystem string) (uint64, error) {
	var max uint64
	for _, video := range m.videos {
		if maintenanceEligible(video, sourceSystem) && video.ID > max {
			max = video.ID
		}
	}
	return max, nil
}

func (m *memMaint) ListPendingMaintenanceVideos(cycleID, maxVideoID, afterID uint64, sourceSystem string, limit int) ([]store.SourceVideoCandidate, error) {
	m.listLimits = append(m.listLimits, limit)
	var out []store.SourceVideoCandidate
	for _, video := range m.videos {
		if !maintenanceEligible(video, sourceSystem) || video.ID <= afterID || video.ID > maxVideoID {
			continue
		}
		status := m.videoStatus(cycleID, video.ID)
		if status == store.MaintenanceVideoCompleted || status == store.MaintenanceVideoFailed {
			continue
		}
		out = append(out, video)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memMaint) ListFailedMaintenanceVideos(cycleID, maxVideoID uint64, sourceSystem string) ([]store.SourceVideoCandidate, error) {
	var out []store.SourceVideoCandidate
	for _, video := range m.videos {
		if !maintenanceEligible(video, sourceSystem) || video.ID > maxVideoID {
			continue
		}
		if m.videoStatus(cycleID, video.ID) == store.MaintenanceVideoFailed {
			out = append(out, video)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memMaint) FirstFailedMaintenance(cycleID uint64) (*store.MaintenanceCheckpoint, error) {
	var found *store.MaintenanceCheckpoint
	for i := range m.checks {
		row := &m.checks[i]
		if row.CycleID == cycleID && row.Status == store.MaintenanceVideoFailed && (found == nil || row.VideoID < found.VideoID) {
			found = row
		}
	}
	if found == nil {
		return nil, nil
	}
	copy := *found
	return &copy, nil
}

func (m *memMaint) SaveMaintenanceCheckpoint(row store.MaintenanceCheckpoint, now time.Time) error {
	_ = now
	for i := range m.checks {
		if m.checks[i].CycleID == row.CycleID && m.checks[i].VideoID == row.VideoID {
			m.checks[i] = row
			return nil
		}
	}
	m.checks = append(m.checks, row)
	return nil
}

func (m *memMaint) CountMaintenanceCheckpointStatus(cycleID uint64) (int, int, error) {
	completed, failed := 0, 0
	for _, row := range m.checks {
		if row.CycleID != cycleID {
			continue
		}
		switch row.Status {
		case store.MaintenanceVideoCompleted:
			completed++
		case store.MaintenanceVideoFailed:
			failed++
		}
	}
	return completed, failed, nil
}

func (m *memMaint) CountRemainingMaintenance(cycleID, maxVideoID uint64, sourceSystem string) (int, error) {
	n := 0
	for _, video := range m.videos {
		if !maintenanceEligible(video, sourceSystem) || video.ID > maxVideoID {
			continue
		}
		if m.videoStatus(cycleID, video.ID) != store.MaintenanceVideoCompleted {
			n++
		}
	}
	return n, nil
}

func (m *memMaint) CountVideosAboveBoundary(sourceSystem string, maxVideoID uint64) (int, error) {
	n := 0
	for _, video := range m.videos {
		if maintenanceEligible(video, sourceSystem) && video.ID > maxVideoID {
			n++
		}
	}
	return n, nil
}

func (m *memMaint) videoStatus(cycleID, videoID uint64) string {
	for _, row := range m.checks {
		if row.CycleID == cycleID && row.VideoID == videoID {
			return row.Status
		}
	}
	return ""
}

func (m *memMaint) cycleByID(id uint64) store.MaintenanceCycle {
	for _, row := range m.cycles {
		if row.ID == id {
			return row
		}
	}
	return store.MaintenanceCycle{}
}

func maintenanceEligible(video store.SourceVideoCandidate, sourceSystem string) bool {
	return video.SourceSystem == sourceSystem && video.Active && strings.TrimSpace(video.SourceID) != ""
}

type serialPages struct {
	body     map[string]map[int]string
	calls    []string
	errAt    string
	cancel   context.CancelFunc
	cancelOn string
	mu       sync.Mutex
	inside   int
}

func (s *serialPages) FetchCommentPage(_ context.Context, sourceVideoID string, page int) (string, error) {
	s.mu.Lock()
	if s.inside != 0 {
		s.mu.Unlock()
		return "", errors.New("concurrent comment fetch")
	}
	s.inside++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inside--
		s.mu.Unlock()
	}()
	key := fmt.Sprintf("%s:%d", sourceVideoID, page)
	s.calls = append(s.calls, key)
	if s.cancelOn == key && s.cancel != nil {
		s.cancel()
		s.cancelOn = ""
		return "", context.Canceled
	}
	if s.errAt == key {
		return "", errors.New("source unavailable")
	}
	body, ok := s.body[sourceVideoID][page]
	if !ok {
		return "", errors.New("missing page")
	}
	return body, nil
}

func activeVideos(ids ...uint64) []store.SourceVideoCandidate {
	out := make([]store.SourceVideoCandidate, 0, len(ids))
	for _, id := range ids {
		out = append(out, store.SourceVideoCandidate{
			ID: id, SourceSystem: "olehdtv", SourceID: fmt.Sprintf("%d", id), Title: fmt.Sprintf("v%d", id), Active: true,
		})
	}
	return out
}

func TestMaintenanceCyclesResumeAndSeparateModes(t *testing.T) {
	comments := store.NewMemorySourceComments()
	native := store.NewMemoryComments()
	native.Seed(store.Comment{VideoID: 1, UserID: 1, Body: "native", Status: "visible"})
	catalog := &memMaint{videos: activeVideos(1, 2)}
	pages := map[string]map[int]string{
		"1": {1: parentPage("11", "one", 2, 2), 2: parentPage("12", "two", 2, 2)},
		"2": {1: parentPage("21", "three", 1, 1)},
	}
	fetcher := &serialPages{body: pages}
	var out bytes.Buffer
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	summary, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Now: now, Out: &out, BatchSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.CycleID != 1 || summary.Completed != 2 || !strings.Contains(out.String(), "MAINTENANCE CYCLE COMPLETE") {
		t.Fatalf("summary=%+v\n%s", summary, out.String())
	}
	if strings.Join(fetcher.calls, ",") != "1:1,1:2,2:1" {
		t.Fatalf("incremental calls=%v", fetcher.calls)
	}
	if catalog.cycleByID(1).MaxVideoID != 2 || catalog.cycleByID(1).Status != store.MaintenanceCycleCompleted {
		t.Fatalf("cycle=%+v", catalog.cycleByID(1))
	}

	ctx, cancel := context.WithCancel(context.Background())
	out.Reset()
	fetcher.calls = nil
	fetcher.cancel = cancel
	fetcher.cancelOn = "1:1"
	interrupted, err := RunMaintenance(ctx, comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Now: now, Out: &out, BatchSize: 1,
	})
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("interrupt err=%v", err)
	}
	if interrupted.CycleID != 2 || catalog.cycleByID(2).Status != store.MaintenanceRunning {
		t.Fatalf("interrupted cycle=%+v summary=%+v", catalog.cycleByID(2), interrupted)
	}
	if catalog.videoStatus(2, 1) == store.MaintenanceVideoFailed || catalog.videoStatus(2, 2) == store.MaintenanceVideoFailed {
		t.Fatal("interrupt marked a video failed")
	}
	out.Reset()
	fetcher.cancelOn = ""
	fetcher.calls = nil
	resumed, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Now: now, Out: &out, BatchSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.CycleID != 2 || !strings.Contains(strings.Join(fetcher.calls, ","), "1:1") || !strings.Contains(strings.Join(fetcher.calls, ","), "2:1") {
		t.Fatalf("resume cycle=%d calls=%v", resumed.CycleID, fetcher.calls)
	}

	out.Reset()
	fetcher.calls = nil
	catalog.videos = append(catalog.videos, activeVideos(3)...)
	pages["3"] = map[int]string{1: parentPage("31", "new", 1, 1)}
	third, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Now: now, Out: &out,
	})
	if err != nil || third.CycleID != 3 || catalog.cycleByID(3).MaxVideoID != 3 {
		t.Fatalf("third=%+v max=%d err=%v", third, catalog.cycleByID(3).MaxVideoID, err)
	}
	if catalog.videoStatus(2, 3) != "" {
		t.Fatal("video added after the previous cycle started was pulled into that cycle")
	}

	out.Reset()
	fetcher.calls = nil
	full, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeFull, Now: now, Out: &out,
	})
	if err != nil || full.CycleID == third.CycleID {
		t.Fatalf("full=%+v incremental=%d err=%v", full, third.CycleID, err)
	}
	joined := strings.Join(fetcher.calls, ",")
	if !strings.Contains(joined, "1:1") || !strings.Contains(joined, "1:2") {
		t.Fatalf("full did not walk every page: %s", joined)
	}
	if native.Snapshot()[0].Body != "native" {
		t.Fatal("maintenance changed native comments")
	}
	if len(comments.Snapshot()) != 4 {
		t.Fatalf("duplicate source comments: %+v", comments.Snapshot())
	}
}

func TestMaintenanceBoundaryFailureRetryAndEmptyVideo(t *testing.T) {
	comments := store.NewMemorySourceComments()
	if _, _, _, err := comments.UpsertSourceComments(1, "olehdtv", []store.SourceCommentInput{{
		SourceCommentID: "11", SourceVideoID: "1", AuthorName: "游客", Body: "one", SourceCreatedRaw: "2026-10-19 01:56:45",
	}}, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	catalog := &memMaint{videos: activeVideos(1, 2, 4)}
	fetcher := &serialPages{body: map[string]map[int]string{
		"1": {1: parentPage("11", "one", 2, 2), 2: parentPage("12", "two", 2, 2)},
		"2": {1: parentPage("21", "two", 1, 1)},
		"4": {1: "   "},
	}, errAt: "2:1"}
	var out bytes.Buffer
	now := time.Unix(50, 0).UTC()
	_, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Now: now, Out: &out, BatchSize: 2,
	})
	if err == nil || !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("err=%v\n%s", err, out.String())
	}
	if strings.Contains(strings.Join(fetcher.calls, ","), "4:1") || strings.Contains(strings.Join(fetcher.calls, ","), "1:2") {
		t.Fatal("incremental continued past the known parent or the failed video")
	}
	if len(catalog.listLimits) == 0 || catalog.listLimits[0] != 2 {
		t.Fatalf("batch limits=%v", catalog.listLimits)
	}
	if catalog.cycleByID(1).MaxVideoID != 4 || catalog.cycleByID(1).Status != store.MaintenanceCycleFailed {
		t.Fatalf("cycle=%+v", catalog.cycleByID(1))
	}
	fetcher.calls = nil
	if _, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Now: now,
	}); !errors.Is(err, ErrFailedCheckpoint) || fetcher.calls != nil {
		t.Fatalf("blocked err=%v calls=%v", err, fetcher.calls)
	}
	fetcher.errAt = ""
	out.Reset()
	summary, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, RetryFailed: true, Now: now, Out: &out,
	})
	if err != nil || summary.CycleID != 1 || !strings.Contains(out.String(), "MAINTENANCE CYCLE COMPLETE") {
		t.Fatalf("retry summary=%+v err=%v\n%s", summary, err, out.String())
	}
	if catalog.videoStatus(1, 4) != store.MaintenanceVideoCompleted {
		t.Fatal("zero-comment video was not completed")
	}
	if strings.Join(fetcher.calls, ",") != "2:1,4:1" {
		t.Fatalf("retry calls=%v", fetcher.calls)
	}
	before := len(comments.Snapshot())
	again, err := RunMaintenance(context.Background(), comments, catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Now: now,
	})
	if err != nil || again.CycleID == 1 || again.Inserted != 0 || again.Skipped == 0 {
		t.Fatalf("new cycle summary=%+v before=%d err=%v", again, before, err)
	}
	if len(comments.Snapshot()) != before {
		t.Fatal("new cycle duplicated source comments")
	}
}

func TestMaintenanceNewVideoWaitsForNextCycle(t *testing.T) {
	comments := store.NewMemorySourceComments()
	catalog := &memMaint{videos: activeVideos(1)}
	fetcher := &serialPages{body: map[string]map[int]string{
		"1": {1: "   "},
		"9": {1: "   "},
	}}
	fetcherOnFirst := &addAfterFirstFetch{serialPages: fetcher, catalog: catalog}
	summary, err := RunMaintenance(context.Background(), comments, catalog, fetcherOnFirst, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeFull, Now: time.Unix(3, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.cycleByID(summary.CycleID).MaxVideoID != 1 || catalog.videoStatus(summary.CycleID, 9) != "" {
		t.Fatalf("boundary did not stay at cycle start: %+v", catalog.cycleByID(summary.CycleID))
	}
	var status bytes.Buffer
	if err := ReportMaintenanceStatus(catalog, MaintenanceOptions{SourceSystem: "olehdtv", Mode: store.MaintenanceModeFull, Out: &status}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.String(), "videos_above_boundary=1") || !strings.Contains(status.String(), "status=completed") {
		t.Fatalf("status:\n%s", status.String())
	}
}

type addAfterFirstFetch struct {
	*serialPages
	catalog *memMaint
	added   bool
}

func (a *addAfterFirstFetch) FetchCommentPage(ctx context.Context, sourceVideoID string, page int) (string, error) {
	body, err := a.serialPages.FetchCommentPage(ctx, sourceVideoID, page)
	if !a.added {
		a.catalog.videos = append(a.catalog.videos, activeVideos(9)...)
		a.added = true
	}
	return body, err
}

func TestSourceCommentLockBlocksMaintenanceAndBackfill(t *testing.T) {
	lock := &holdLock{}
	release, err := lock.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	fetcher := &serialPages{body: map[string]map[int]string{"1": {1: "   "}}}
	catalog := &memMaint{videos: activeVideos(1)}
	if _, err := RunMaintenance(context.Background(), store.NewMemorySourceComments(), catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeFull, Locker: lock,
	}); !errors.Is(err, store.ErrOperationLocked) || fetcher.calls != nil || len(catalog.cycles) != 0 {
		t.Fatalf("maintenance lock err=%v calls=%v cycles=%d", err, fetcher.calls, len(catalog.cycles))
	}
	backfill := &fakeCatalog{videos: activeVideos(1)}
	if _, err := RunBackfill(context.Background(), store.NewMemorySourceComments(), backfill, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", Limit: 1, Locker: lock,
	}); !errors.Is(err, store.ErrOperationLocked) || backfill.saves != 0 {
		t.Fatalf("backfill lock err=%v saves=%d", err, backfill.saves)
	}
	if _, err := RunDrain(context.Background(), store.NewMemorySourceComments(), backfill, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 1, Locker: lock,
	}); !errors.Is(err, store.ErrOperationLocked) {
		t.Fatalf("drain lock err=%v", err)
	}
	if _, err := RunMaintenance(context.Background(), store.NewMemorySourceComments(), catalog, fetcher, MaintenanceOptions{
		SourceSystem: "olehdtv", Mode: store.MaintenanceModeIncremental, Locker: lock,
	}); !errors.Is(err, store.ErrOperationLocked) {
		t.Fatalf("incremental overlapped the held lock err=%v", err)
	}
}

type holdLock struct {
	held bool
}

func (h *holdLock) Acquire(context.Context) (func(), error) {
	if h.held {
		return nil, store.ErrOperationLocked
	}
	h.held = true
	return func() { h.held = false }, nil
}
