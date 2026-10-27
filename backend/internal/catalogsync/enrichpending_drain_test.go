package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

func TestValidateEnrichPendingDrainFlags(t *testing.T) {
	if err := ValidateEnrichPendingDrainFlags(false, true, false); err == nil {
		t.Fatal("drain without catch-up must fail")
	}
	if err := ValidateEnrichPendingDrainFlags(true, true, true); err == nil {
		t.Fatal("drain + dry-run must fail")
	}
	if err := ValidateEnrichPendingDrainFlags(true, true, false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnrichPendingDrainFlags(false, false, true); err != nil {
		t.Fatal(err)
	}
}

func TestValidateEnrichPendingDrainOptions(t *testing.T) {
	if err := ValidateEnrichPendingDrainOptions(90, 40, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnrichPendingDrainOptions(21, 40, time.Hour); err != nil {
		t.Fatal(err) // catch-up limit ok via ValidateEnrichPendingLimitMode(, true)
	}
	if err := ValidateEnrichPendingLimitMode(21, false); err == nil {
		t.Fatal("normal max remains 20")
	}
	if err := ValidateEnrichPendingLimitMode(90, true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnrichPendingLimitMode(91, true); err == nil {
		t.Fatal("catch-up max remains 90")
	}
	if err := ValidateEnrichPendingDrainOptions(90, 0, time.Hour); err == nil {
		t.Fatal("max-batches 0")
	}
	if err := ValidateEnrichPendingDrainOptions(90, 101, time.Hour); err == nil {
		t.Fatal("max-batches 101")
	}
	if err := ValidateEnrichPendingDrainOptions(90, 40, 0); err == nil {
		t.Fatal("max-runtime 0")
	}
	if err := ValidateEnrichPendingDrainOptions(90, 40, 4*time.Hour); err == nil {
		t.Fatal("max-runtime > 3h")
	}
}

func TestEvaluateDrainBatch(t *testing.T) {
	stop, reason, err := evaluateDrainBatch(context.Background(), EnrichPendingSummary{Selected: 0}, nil)
	if !stop || reason != DrainStopEmpty || err != nil {
		t.Fatalf("%v %s %v", stop, reason, err)
	}
	stop, reason, err = evaluateDrainBatch(context.Background(), EnrichPendingSummary{Selected: 5, Failures: 2, Error: "x"}, nil)
	if !stop || reason != DrainStopFailures || err == nil {
		t.Fatalf("%v %s %v", stop, reason, err)
	}
	stop, reason, err = evaluateDrainBatch(context.Background(), EnrichPendingSummary{Selected: 5, Attempted: 5}, nil)
	if !stop || reason != DrainStopNoProgress || err == nil {
		t.Fatalf("%v %s %v", stop, reason, err)
	}
	stop, reason, err = evaluateDrainBatch(context.Background(), EnrichPendingSummary{Selected: 5, Unavailable: 5}, nil)
	if stop || reason != "" || err != nil {
		t.Fatalf("unavailable is progress: %v %s %v", stop, reason, err)
	}
	stop, reason, err = evaluateDrainBatch(context.Background(), EnrichPendingSummary{Selected: 5, Enriched: 3, Unavailable: 2}, nil)
	if stop {
		t.Fatal("expected continue")
	}
	stop, reason, err = evaluateDrainBatch(context.Background(), EnrichPendingSummary{Selected: 2, SkippedExisting: 2}, nil)
	if stop {
		t.Fatal("skipped is progress when Apply returns !updated")
	}

	// Cancellation-shaped failures → interrupted, not failures.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stop, reason, err = evaluateDrainBatch(ctx, EnrichPendingSummary{
		Selected: 3, Failures: 1, Error: context.Canceled.Error(),
		Actions: []EnrichPendingAction{{Action: "FAILURE", Detail: context.Canceled.Error()}},
	}, nil)
	if !stop || reason != DrainStopInterrupted || err == nil {
		t.Fatalf("cancel classify: %v %s %v", stop, reason, err)
	}
	// Aggregate Error string must not override cancel-shaped per-item actions.
	stop, reason, err = evaluateDrainBatch(ctx, EnrichPendingSummary{
		Selected: 3, Failures: 3, Error: "3 enrichment failures",
		Actions: []EnrichPendingAction{
			{Action: "STILL_PENDING", Detail: "fetch: " + context.Canceled.Error()},
			{Action: "FAILURE", Detail: context.Canceled.Error()},
			{Action: "FAILURE", Detail: context.Canceled.Error()},
		},
	}, nil)
	if !stop || reason != DrainStopInterrupted {
		t.Fatalf("aggregate error under cancel: %v %s %v", stop, reason, err)
	}
	// Genuine fetch failure must stay failures even if ctx is also canceled.
	stop, reason, err = evaluateDrainBatch(ctx, EnrichPendingSummary{
		Selected: 2, Failures: 2, Error: "2 enrichment failures",
		Actions: []EnrichPendingAction{
			{Action: "STILL_PENDING", Detail: "fetch: boom"},
			{Action: "FAILURE", Detail: context.Canceled.Error()},
		},
	}, nil)
	if !stop || reason != DrainStopFailures {
		t.Fatalf("must not hide real failures: %v %s %v", stop, reason, err)
	}
}

func TestDrainExitCode(t *testing.T) {
	if DrainExitCode(EnrichPendingDrainSummary{StopReason: DrainStopEmpty}, nil) != 0 {
		t.Fatal()
	}
	if DrainExitCode(EnrichPendingDrainSummary{StopReason: DrainStopMaxBatches}, nil) != 0 {
		t.Fatal()
	}
	if DrainExitCode(EnrichPendingDrainSummary{StopReason: DrainStopMaxRuntime}, context.DeadlineExceeded) != 0 {
		t.Fatal()
	}
	if DrainExitCode(EnrichPendingDrainSummary{StopReason: DrainStopInterrupted}, context.Canceled) != 1 {
		t.Fatal()
	}
	if DrainExitCode(EnrichPendingDrainSummary{StopReason: DrainStopFailures}, errors.New("x")) != 1 {
		t.Fatal()
	}
	if DrainExitCode(EnrichPendingDrainSummary{StopReason: DrainStopNoProgress}, errors.New("x")) != 1 {
		t.Fatal()
	}
}

func TestRunDrain_stopsWhenEmpty(t *testing.T) {
	st := newEnrichMemStore()
	pages := map[string]string{}
	for i := 1; i <= 7; i++ {
		url := fmt.Sprintf("https://src/p%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		pages[url] = playHTML(fmt.Sprintf("https://cdn.example/%d.m3u8", i))
	}
	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 5, Concurrency: 1, MaxBatches: 10, MaxRuntime: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.StopReason != DrainStopEmpty {
		t.Fatalf("%+v", sum)
	}
	if sum.BatchesRun < 2 {
		t.Fatalf("expected multiple batches, got %d", sum.BatchesRun)
	}
	if sum.TotalEnriched != 7 || sum.TotalSelected != 7 {
		t.Fatalf("%+v", sum)
	}
	if len(st.opRuns) != sum.BatchesRun {
		// last empty batch also writes an operational_runs succeeded row
		t.Fatalf("op runs=%d batches=%d", len(st.opRuns), sum.BatchesRun)
	}
}

func TestRunDrain_stopsAtMaxBatches(t *testing.T) {
	st := newEnrichMemStore()
	pages := map[string]string{}
	for i := 1; i <= 50; i++ {
		url := fmt.Sprintf("https://src/p%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		pages[url] = playHTML(fmt.Sprintf("https://cdn.example/%d.m3u8", i))
	}
	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 5, Concurrency: 1, MaxBatches: 3, MaxRuntime: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.StopReason != DrainStopMaxBatches || sum.BatchesRun != 3 {
		t.Fatalf("%+v", sum)
	}
	if sum.TotalEnriched != 15 {
		t.Fatalf("enriched=%d", sum.TotalEnriched)
	}
}

func TestRunDrain_stopsOnFailures(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "v1",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	f := &mapFetcher{err: map[string]error{"https://src/p1": fmt.Errorf("boom")}}
	sum, err := NewEnrichPendingWorker(st, f).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 5, Concurrency: 1, MaxBatches: 5, MaxRuntime: time.Hour,
	})
	if err == nil || sum.StopReason != DrainStopFailures {
		t.Fatalf("%+v err=%v", sum, err)
	}
	if sum.BatchesRun != 1 || sum.TotalFailures < 1 {
		t.Fatalf("%+v", sum)
	}
}

func TestRunDrain_unavailableCountsAsProgress(t *testing.T) {
	st := newEnrichMemStore()
	// Body that classifies as source-restricted membership page.
	restricted := `<html><body>您没有权限访问此页面</body></html>`
	// Use a known restricted marker from maccms if needed — fall back to encrypt path.
	// Parse miss with empty stream isn't unavailable; use encrypt!=0 style via player_aaaa.
	pages := map[string]string{}
	for i := 1; i <= 3; i++ {
		url := fmt.Sprintf("https://src/p%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		// encrypt!=0 → unavailable terminal
		pages[url] = `var player_aaaa={"flag":"play","encrypt":1,"url":"https:\/\/x.m3u8","from":"plyr"}</script>`
	}
	_ = restricted
	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 5, Concurrency: 1, MaxBatches: 5, MaxRuntime: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.StopReason != DrainStopEmpty {
		t.Fatalf("%+v", sum)
	}
	if sum.TotalUnavailable != 3 || sum.TotalEnriched != 0 || sum.TotalFailures != 0 {
		t.Fatalf("%+v", sum)
	}
}

func TestRunDrain_cancellationStops(t *testing.T) {
	st := newEnrichMemStore()
	pages := map[string]string{}
	for i := 1; i <= 30; i++ {
		url := fmt.Sprintf("https://src/p%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		pages[url] = playHTML(fmt.Sprintf("https://cdn.example/%d.m3u8", i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	slow := &slowFetcher{inner: &mapFetcher{pages: pages}, delay: 30 * time.Millisecond}
	sum, err := NewEnrichPendingWorker(st, slow).RunDrain(ctx, EnrichPendingDrainOptions{
		Limit: 5, Concurrency: 1, MaxBatches: 40, MaxRuntime: time.Hour,
	})
	if sum.StopReason != DrainStopInterrupted {
		t.Fatalf("want interrupted, got stop=%s err=%v sum=%+v", sum.StopReason, err, sum)
	}
	if DrainExitCode(sum, err) == 0 {
		t.Fatal("interrupted must be non-zero exit")
	}
}

func TestRunDrain_skippedLeavesPendingSelection(t *testing.T) {
	// raceStore transitions the row to ok+URL before Apply, so Apply returns !updated
	// (SKIPPED_EXISTING). That row must not be selected again on the next drain batch.
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	st.episodes[2] = &store.PendingEnrichmentEpisode{
		ID: 2, VideoID: 2, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "b",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p2",
	}
	pages := map[string]string{
		"https://src/p1": playHTML("https://cdn.example/1.m3u8"),
		"https://src/p2": playHTML("https://cdn.example/2.m3u8"),
	}
	wrapped := &raceStore{enrichMemStore: st}
	sum, err := NewEnrichPendingWorker(wrapped, &mapFetcher{pages: pages}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 10, Concurrency: 1, MaxBatches: 5, MaxRuntime: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.StopReason != DrainStopEmpty {
		t.Fatalf("%+v", sum)
	}
	if sum.TotalSkipped < 1 {
		t.Fatalf("expected skips from raceStore: %+v", sum)
	}
	// Both rows must be out of pending_enrichment (skip path mutates to ok+URL).
	for _, id := range []uint64{1, 2} {
		e := st.episodes[id]
		if strings.EqualFold(e.PlaybackStatus, "pending_enrichment") && strings.TrimSpace(e.PlaybackURL) == "" {
			t.Fatalf("id=%d still selectable pending: %+v", id, e)
		}
	}
}

type slowFetcher struct {
	inner PageFetcher
	delay time.Duration
}

func (f *slowFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(f.delay):
	}
	return f.inner.Fetch(ctx, pageURL)
}

func TestRunDrain_lockPreventsOverlap(t *testing.T) {
	st := newEnrichMemStore()
	pages := map[string]string{}
	for i := 1; i <= 20; i++ {
		url := fmt.Sprintf("https://src/p%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		pages[url] = playHTML(fmt.Sprintf("https://cdn.example/%d.m3u8", i))
	}
	hold, err := st.AcquireAdvisoryLock(context.Background(), store.PendingEnrichmentLockName)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hold.Release() }()

	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 5, Concurrency: 1, MaxBatches: 5, MaxRuntime: time.Hour,
	})
	if !errors.Is(err, store.ErrOperationLocked) || sum.StopReason != DrainStopLockBusy {
		t.Fatalf("%+v err=%v", sum, err)
	}
}

func TestRun_singleBatchUnchangedWithCatchUpFalse(t *testing.T) {
	st := newEnrichMemStore()
	st.episodes[1] = &store.PendingEnrichmentEpisode{
		ID: 1, VideoID: 1, SourceSystem: olehdtv.SourceSystem, SourceVideoID: "a",
		SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: "https://src/p1",
	}
	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: map[string]string{
		"https://src/p1": playHTML("https://cdn.example/1.m3u8"),
	}}).Run(context.Background(), 15, false, 1, false)
	if err != nil || sum.Enriched != 1 || sum.CatchUp {
		t.Fatalf("%+v err=%v", sum, err)
	}
}

func TestRunDrain_aggregateCounters(t *testing.T) {
	st := newEnrichMemStore()
	pages := map[string]string{}
	for i := 1; i <= 4; i++ {
		url := fmt.Sprintf("https://src/ok%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		pages[url] = playHTML(fmt.Sprintf("https://cdn.example/%d.m3u8", i))
	}
	for i := 5; i <= 6; i++ {
		url := fmt.Sprintf("https://src/bad%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		pages[url] = `var player_aaaa={"flag":"play","encrypt":1,"url":"https:\/\/x.m3u8","from":"plyr"}</script>`
	}
	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 10, Concurrency: 1, MaxBatches: 5, MaxRuntime: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Drain || !sum.CatchUp {
		t.Fatalf("%+v", sum)
	}
	if sum.TotalEnriched != 4 || sum.TotalUnavailable != 2 || sum.TotalSelected != 6 {
		t.Fatalf("%+v", sum)
	}
	if sum.TotalFailures != 0 || sum.StopReason != DrainStopEmpty {
		t.Fatalf("%+v", sum)
	}
}

func TestRunDrain_sessionHoldsLockAcrossBatches(t *testing.T) {
	st := &drainLockProbeStore{enrichMemStore: newEnrichMemStore()}
	pages := map[string]string{}
	for i := 1; i <= 12; i++ {
		url := fmt.Sprintf("https://src/p%d", i)
		st.episodes[uint64(i)] = &store.PendingEnrichmentEpisode{
			ID: uint64(i), VideoID: uint64(i), SourceSystem: olehdtv.SourceSystem, SourceVideoID: fmt.Sprintf("v%d", i),
			SID: 1, NID: 1, PlaybackStatus: "pending_enrichment", PlayPageURL: url,
		}
		pages[url] = playHTML(fmt.Sprintf("https://cdn.example/%d.m3u8", i))
	}
	sum, err := NewEnrichPendingWorker(st, &mapFetcher{pages: pages}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 5, Concurrency: 1, MaxBatches: 10, MaxRuntime: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.StopReason != DrainStopEmpty {
		t.Fatalf("%+v", sum)
	}
	if st.acquireCount != 1 {
		t.Fatalf("expected 1 session lock acquire, got %d", st.acquireCount)
	}
	if st.releaseCount != 1 {
		t.Fatalf("expected 1 session lock release, got %d", st.releaseCount)
	}
	if !st.lockedDuringSecondList {
		t.Fatal("lock must remain held across batches")
	}
}

type drainLockProbeStore struct {
	*enrichMemStore
	acquireCount           int
	releaseCount           int
	listCalls              int
	lockedDuringSecondList bool
}

func (s *drainLockProbeStore) AcquireAdvisoryLock(ctx context.Context, name string) (*store.AdvisoryHold, error) {
	s.acquireCount++
	hold, err := s.enrichMemStore.AcquireAdvisoryLock(ctx, name)
	if err != nil {
		return nil, err
	}
	return store.NewTestAdvisoryHold(func() error {
		s.releaseCount++
		return hold.Release()
	}), nil
}

func (s *drainLockProbeStore) ListPendingEnrichmentEpisodes(system string, finalLimit int, includeMembershipRetry, includeLegacyUnavailable bool) ([]store.PendingEnrichmentEpisode, error) {
	s.listCalls++
	if s.listCalls == 2 {
		s.mu.Lock()
		s.lockedDuringSecondList = s.locked
		s.mu.Unlock()
	}
	return s.enrichMemStore.ListPendingEnrichmentEpisodes(system, finalLimit, includeMembershipRetry, includeLegacyUnavailable)
}

func TestRunDrain_rejectsViaValidationInRunDrain(t *testing.T) {
	st := newEnrichMemStore()
	_, err := NewEnrichPendingWorker(st, &mapFetcher{}).RunDrain(context.Background(), EnrichPendingDrainOptions{
		Limit: 91, Concurrency: 1, MaxBatches: 40, MaxRuntime: time.Hour,
	})
	if err == nil || !strings.Contains(err.Error(), "catch-up") {
		t.Fatalf("%v", err)
	}
}
