package catalogsync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

const (
	EnrichPendingMinLimit   = 1
	EnrichPendingMaxLimit   = 20
	EnrichPendingDefaultLim = 15
	EnrichPendingMaxPerVideo = 3
	EnrichPendingMinDelayMs = 1000
	EnrichPendingMaxConc    = 2
)

// EnrichPendingStore is the DB surface for deferred enrichment (mockable).
type EnrichPendingStore interface {
	TryAdvisoryLock(name string, timeoutSec int) (bool, error)
	ReleaseAdvisoryLock(name string) error
	ListPendingEnrichmentEpisodes(system string, fetchLimit int) ([]store.PendingEnrichmentEpisode, error)
	ApplyPlaybackEnrichment(id uint64, playbackURL, playbackSource, status string) (updated bool, err error)
}

// PageFetcher fetches a single play page HTML body.
type PageFetcher interface {
	Fetch(ctx context.Context, pageURL string) (body string, err error)
}

// EnrichPendingAction is a short per-item report line.
type EnrichPendingAction struct {
	Action        string `json:"action"`
	EpisodeID     uint64 `json:"episode_id,omitempty"`
	SourceVideoID string `json:"source_video_id,omitempty"`
	SID           int    `json:"sid,omitempty"`
	NID           int    `json:"nid,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

// EnrichPendingSummary is the machine-readable worker report.
type EnrichPendingSummary struct {
	DryRun         bool                  `json:"dry_run"`
	Limit          int                   `json:"limit"`
	Selected       int                   `json:"selected"`
	Attempted      int                   `json:"attempted"`
	Enriched       int                   `json:"enriched"`
	StillPending   int                   `json:"still_pending"`
	Unavailable    int                   `json:"unavailable"`
	Failures       int                   `json:"failures"`
	SkippedExisting int                  `json:"skipped_existing"`
	LockAcquired   bool                  `json:"lock_acquired"`
	RequestsMade   int                   `json:"requests_made"`
	Actions        []EnrichPendingAction `json:"actions,omitempty"`
	Error          string                `json:"error,omitempty"`
}

// EnrichPendingWorker gradually resolves pending_enrichment play pages.
type EnrichPendingWorker struct {
	Store    EnrichPendingStore
	Fetcher  PageFetcher
	Log      *slog.Logger
	LockName string
}

func NewEnrichPendingWorker(st EnrichPendingStore, fetcher PageFetcher) *EnrichPendingWorker {
	return &EnrichPendingWorker{
		Store:    st,
		Fetcher:  fetcher,
		Log:      slog.Default().With("component", "olehdtv-enrichpending"),
		LockName: store.PendingEnrichmentLockName,
	}
}

// ValidateEnrichPendingLimit enforces 1..20.
func ValidateEnrichPendingLimit(limit int) error {
	if limit < EnrichPendingMinLimit {
		return fmt.Errorf("enrichpending: --limit must be >= %d", EnrichPendingMinLimit)
	}
	if limit > EnrichPendingMaxLimit {
		return fmt.Errorf("enrichpending: --limit %d exceeds hard max %d", limit, EnrichPendingMaxLimit)
	}
	return nil
}

// ClampEnrichPendingHTTPSettings enforces delay>=1000ms and concurrency in 1..2.
func ClampEnrichPendingHTTPSettings(delayMs, concurrency, retries int) (delayOut, concOut, retriesOut int) {
	delayOut = delayMs
	if delayOut < EnrichPendingMinDelayMs {
		delayOut = EnrichPendingMinDelayMs
	}
	concOut = concurrency
	if concOut <= 0 {
		concOut = 1
	}
	if concOut > EnrichPendingMaxConc {
		concOut = EnrichPendingMaxConc
	}
	retriesOut = retries
	if retriesOut <= 0 {
		retriesOut = 3
	}
	return delayOut, concOut, retriesOut
}

// SelectFairPendingBatch picks up to limit rows, prioritizing videos with zero
// playable episodes, then applying a per-video cap for backlog fairness.
func SelectFairPendingBatch(cands []store.PendingEnrichmentEpisode, limit int) []store.PendingEnrichmentEpisode {
	if limit <= 0 {
		return nil
	}
	if limit > EnrichPendingMaxLimit {
		limit = EnrichPendingMaxLimit
	}
	out := make([]store.PendingEnrichmentEpisode, 0, limit)
	perVideo := map[uint64]int{}
	seen := map[uint64]bool{}

	take := func(allowZeroOnly bool) {
		for _, c := range cands {
			if len(out) >= limit {
				return
			}
			if seen[c.ID] {
				continue
			}
			if allowZeroOnly && c.OkEpisodeCount > 0 {
				continue
			}
			if !allowZeroOnly && c.OkEpisodeCount == 0 {
				continue
			}
			if perVideo[c.VideoID] >= EnrichPendingMaxPerVideo {
				continue
			}
			seen[c.ID] = true
			perVideo[c.VideoID]++
			out = append(out, c)
		}
	}
	take(true)
	take(false)
	return out
}

type enrichPendingResult struct {
	ep     store.PendingEnrichmentEpisode
	action EnrichPendingAction
	req    bool
	enr    bool
	pend   bool
	unav   bool
	fail   bool
	skip   bool
}

// Run executes one bounded enrichment pass.
func (w *EnrichPendingWorker) Run(ctx context.Context, limit int, dryRun bool, concurrency int) (EnrichPendingSummary, error) {
	sum := EnrichPendingSummary{
		DryRun:  dryRun,
		Limit:   limit,
		Actions: make([]EnrichPendingAction, 0, limit),
	}
	if err := ValidateEnrichPendingLimit(limit); err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	if w.Store == nil || w.Fetcher == nil {
		sum.Error = "store and fetcher are required"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > EnrichPendingMaxConc {
		concurrency = EnrichPendingMaxConc
	}

	got, err := w.Store.TryAdvisoryLock(w.LockName, 0)
	if err != nil {
		sum.Error = "advisory lock error: " + err.Error()
		return sum, err
	}
	if !got {
		sum.Error = "another enrichpending worker is already running"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	sum.LockAcquired = true
	defer func() { _ = w.Store.ReleaseAdvisoryLock(w.LockName) }()

	fetchLimit := limit * 4
	if fetchLimit < limit {
		fetchLimit = limit
	}
	cands, err := w.Store.ListPendingEnrichmentEpisodes(olehdtv.SourceSystem, fetchLimit)
	if err != nil {
		sum.Failures++
		sum.Error = "list pending: " + err.Error()
		return sum, err
	}
	batch := SelectFairPendingBatch(cands, limit)
	sum.Selected = len(batch)
	if len(batch) == 0 {
		return sum, nil
	}

	jobs := make(chan store.PendingEnrichmentEpisode)
	results := make(chan enrichPendingResult, len(batch))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ep := range jobs {
				r := w.processOne(ctx, ep, dryRun)
				results <- r
			}
		}()
	}
	for _, ep := range batch {
		select {
		case <-ctx.Done():
			sum.Error = ctx.Err().Error()
			sum.Failures++
		case jobs <- ep:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	for r := range results {
		sum.Attempted++
		if r.req {
			sum.RequestsMade++
		}
		if r.enr {
			sum.Enriched++
		}
		if r.pend {
			sum.StillPending++
		}
		if r.unav {
			sum.Unavailable++
		}
		if r.fail {
			sum.Failures++
		}
		if r.skip {
			sum.SkippedExisting++
		}
		if r.action.Action != "" {
			sum.Actions = append(sum.Actions, r.action)
		}
	}
	if sum.Failures > 0 && sum.Enriched == 0 && sum.Error == "" {
		sum.Error = fmt.Sprintf("%d enrichment failures", sum.Failures)
		return sum, fmt.Errorf("%s", sum.Error)
	}
	return sum, nil
}

func (w *EnrichPendingWorker) processOne(ctx context.Context, ep store.PendingEnrichmentEpisode, dryRun bool) enrichPendingResult {
	r := enrichPendingResult{ep: ep}
	act := EnrichPendingAction{
		EpisodeID:     ep.ID,
		SourceVideoID: ep.SourceVideoID,
		SID:           ep.SID,
		NID:           ep.NID,
	}
	if strings.TrimSpace(ep.PlayPageURL) == "" {
		r.fail = true
		act.Action = "FAILURE"
		act.Detail = "empty play_page_url"
		r.action = act
		return r
	}
	if ctx.Err() != nil {
		r.fail = true
		r.pend = true
		act.Action = "FAILURE"
		act.Detail = ctx.Err().Error()
		r.action = act
		return r
	}

	body, err := w.Fetcher.Fetch(ctx, ep.PlayPageURL)
	r.req = true
	if err != nil {
		r.fail = true
		r.pend = true
		act.Action = "STILL_PENDING"
		act.Detail = "fetch: " + err.Error()
		r.action = act
		return r
	}

	p, ok := maccms.ParsePlayerAAAA(body)
	if !ok {
		r.fail = true
		r.pend = true
		act.Action = "STILL_PENDING"
		act.Detail = "player_aaaa parse miss"
		r.action = act
		return r
	}

	tmp := maccms.EpisodeRef{PlayURL: ep.PlayPageURL, Available: true}
	maccms.ApplyPlayerToEpisode(&tmp, p)
	url := strings.TrimSpace(tmp.StreamURL)
	from := strings.TrimSpace(tmp.From)
	if from == "" {
		from = ep.PlaybackSource
	}

	if url == "" || !tmp.Available {
		// Explicit unusable stream after successful parse (e.g. encrypt!=0).
		if dryRun {
			r.unav = true
			act.Action = "WOULD_UNAVAILABLE"
			act.Detail = "parsed but no usable stream"
			r.action = act
			return r
		}
		updated, uerr := w.Store.ApplyPlaybackEnrichment(ep.ID, "", from, "unavailable")
		if uerr != nil {
			r.fail = true
			r.pend = true
			act.Action = "FAILURE"
			act.Detail = uerr.Error()
			r.action = act
			return r
		}
		if !updated {
			r.skip = true
			act.Action = "SKIPPED_EXISTING"
			act.Detail = "row no longer pending"
			r.action = act
			return r
		}
		r.unav = true
		act.Action = "UNAVAILABLE"
		act.Detail = "parsed but no usable stream"
		r.action = act
		return r
	}

	if dryRun {
		r.enr = true
		act.Action = "WOULD_ENRICH"
		act.Detail = "playback_url set"
		r.action = act
		return r
	}

	updated, uerr := w.Store.ApplyPlaybackEnrichment(ep.ID, url, from, "ok")
	if uerr != nil {
		r.fail = true
		r.pend = true
		act.Action = "FAILURE"
		act.Detail = uerr.Error()
		r.action = act
		return r
	}
	if !updated {
		r.skip = true
		act.Action = "SKIPPED_EXISTING"
		act.Detail = "row no longer pending or already has playback_url"
		r.action = act
		return r
	}
	r.enr = true
	act.Action = "ENRICHED"
	act.Detail = "playback_status=ok"
	r.action = act
	return r
}

// Ensure *store.MySQL satisfies EnrichPendingStore.
var _ EnrichPendingStore = (*store.MySQL)(nil)
