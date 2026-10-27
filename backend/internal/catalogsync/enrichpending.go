package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

const (
	EnrichPendingMinLimit    = 1
	EnrichPendingMaxLimit    = 20 // normal / timer / admin hard max
	EnrichPendingDefaultLim  = 15
	EnrichPendingMaxPerVideo = 3
	EnrichPendingMinDelayMs  = 1000
	EnrichPendingMaxConc     = 2
	// EnrichPendingCatchUpMaxLimit is the hard max when --catch-up is set (manual only).
	// Must match store.PendingEnrichmentMaxFinalLimit.
	EnrichPendingCatchUpMaxLimit = 90
)

// EnrichPendingStore is the DB surface for deferred enrichment (mockable).
type EnrichPendingStore interface {
	AcquireAdvisoryLock(ctx context.Context, name string) (*store.AdvisoryHold, error)
	ListPendingEnrichmentEpisodes(system string, finalLimit int, includeMembershipRetry, includeLegacyUnavailable bool) ([]store.PendingEnrichmentEpisode, error)
	GetEnrichmentCandidateByID(system string, id uint64) (store.PendingEnrichmentEpisode, bool, error)
	GetEnrichmentCandidateBySourceRef(system, sourceVideoID string, sid, nid int) (store.PendingEnrichmentEpisode, bool, error)
	ApplyPlaybackEnrichment(id uint64, playbackURL, playbackSource, status, restrictionReason string) (updated bool, err error)
	StartOperationalRun(in store.StartOperationalRunInput) (uint64, error)
	FinishOperationalRun(in store.FinishOperationalRunInput) error
	RejectOperationalRun(in store.RejectOperationalRunInput) (uint64, error)
}

// EnrichPendingTarget selects exactly one DB episode for manual CLI verification.
// Either EpisodeID or the full SourceVideoID+SID+NID triple must be set — never both.
type EnrichPendingTarget struct {
	EpisodeID     uint64
	SourceVideoID string
	SID           int
	NID           int
}

// Active reports whether any single-episode targeting flag was provided.
func (t EnrichPendingTarget) Active() bool {
	return t.EpisodeID > 0 || strings.TrimSpace(t.SourceVideoID) != "" || t.SID != 0 || t.NID != 0
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
	DryRun          bool                  `json:"dry_run"`
	CatchUp         bool                  `json:"catch_up"`
	Limit           int                   `json:"limit"`
	Selected        int                   `json:"selected"`
	Attempted       int                   `json:"attempted"`
	Enriched        int                   `json:"enriched"`
	StillPending    int                   `json:"still_pending"`
	Unavailable     int                   `json:"unavailable"`
	Failures        int                   `json:"failures"`
	SkippedExisting int                   `json:"skipped_existing"`
	LockAcquired    bool                  `json:"lock_acquired"`
	RequestsMade    int                   `json:"requests_made"`
	Actions         []EnrichPendingAction `json:"actions,omitempty"`
	Error           string                `json:"error,omitempty"`
}

// EnrichPendingWorker gradually resolves pending_enrichment play pages.
type EnrichPendingWorker struct {
	Store             EnrichPendingStore
	Fetcher           PageFetcher
	Log               *slog.Logger
	LockName          string
	TriggerType       string // cli|timer|admin; empty defaults to cli
	RequestedByUserID uint64 // required >0 when TriggerType=admin; must be 0 for cli|timer
	// IncludeMembershipRetry selects unavailable + source_restricted.
	// CLI: auth configured AND explicit manual catch-up/drain/target/legacy flag.
	IncludeMembershipRetry bool
	// IncludeLegacyUnavailable selects unavailable + empty restriction_reason
	// (historical pre-classification shape, e.g. episode 43393). Same CLI gate as
	// IncludeMembershipRetry — never timers / never auth-alone.
	IncludeLegacyUnavailable bool
}

func NewEnrichPendingWorker(st EnrichPendingStore, fetcher PageFetcher) *EnrichPendingWorker {
	return &EnrichPendingWorker{
		Store:       st,
		Fetcher:     fetcher,
		Log:         slog.Default().With("component", "olehdtv-enrichpending"),
		LockName:    store.PendingEnrichmentLockName,
		TriggerType: store.TriggerCLI,
	}
}

// ValidateEnrichPendingLimit enforces normal-mode 1..20 (timer/admin/default CLI).
func ValidateEnrichPendingLimit(limit int) error {
	return ValidateEnrichPendingLimitMode(limit, false)
}

// ValidateEnrichPendingLimitMode enforces 1..20 normally, or 1..90 with catchUp.
func ValidateEnrichPendingLimitMode(limit int, catchUp bool) error {
	if limit < EnrichPendingMinLimit {
		return fmt.Errorf("enrichpending: --limit must be >= %d", EnrichPendingMinLimit)
	}
	max := EnrichPendingMaxLimit
	if catchUp {
		max = EnrichPendingCatchUpMaxLimit
	}
	if limit > max {
		if catchUp {
			return fmt.Errorf("enrichpending: --limit %d exceeds catch-up hard max %d", limit, EnrichPendingCatchUpMaxLimit)
		}
		return fmt.Errorf("enrichpending: --limit %d exceeds hard max %d (use --catch-up for up to %d)", limit, EnrichPendingMaxLimit, EnrichPendingCatchUpMaxLimit)
	}
	return nil
}

// EffectiveEnrichPendingMaxLimit returns the hard ceiling for the mode.
func EffectiveEnrichPendingMaxLimit(catchUp bool) int {
	if catchUp {
		return EnrichPendingCatchUpMaxLimit
	}
	return EnrichPendingMaxLimit
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

// SelectFairPendingBatch picks up to limit rows:
//  1. videos with zero playable episodes first
//  2. then any remaining eligible rows (other videos / already-ok videos)
//  3. never more than EnrichPendingMaxPerVideo from one video
//  4. never more than EnrichPendingCatchUpMaxLimit globally (absolute ceiling)
//
// Candidates should already be diversified by ListPendingEnrichmentEpisodes
// (≤3 pending rows/video); this pass re-applies the same fairness rules.
// Callers must validate --limit for normal vs --catch-up before invoking Run.
func SelectFairPendingBatch(cands []store.PendingEnrichmentEpisode, limit int) []store.PendingEnrichmentEpisode {
	if limit <= 0 {
		return nil
	}
	if limit > EnrichPendingCatchUpMaxLimit {
		limit = EnrichPendingCatchUpMaxLimit
	}
	out := make([]store.PendingEnrichmentEpisode, 0, limit)
	perVideo := map[uint64]int{}
	seen := map[uint64]bool{}

	take := func(zeroOkOnly bool) {
		for _, c := range cands {
			if len(out) >= limit {
				return
			}
			if seen[c.ID] {
				continue
			}
			if zeroOkOnly && c.OkEpisodeCount > 0 {
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
	// Pass 1: prefer videos with no playable episodes yet.
	take(true)
	// Pass 2: fill remaining slots from any video still under the per-video cap
	// (including other zero-ok videos if the candidate window still has them).
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
// dryRun skips catalog writes and operational_runs (lock still acquired).
// catchUp enables the higher --limit ceiling (manual catch-up only).
// Lock is acquired for this single batch and released before return.
func (w *EnrichPendingWorker) Run(ctx context.Context, limit int, dryRun bool, concurrency int, catchUp bool) (EnrichPendingSummary, error) {
	sum := EnrichPendingSummary{
		DryRun:  dryRun,
		CatchUp: catchUp,
		Limit:   limit,
		Actions: make([]EnrichPendingAction, 0, limit),
	}
	if err := ValidateEnrichPendingLimitMode(limit, catchUp); err != nil {
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

	trigger := w.TriggerType
	if trigger == "" {
		trigger = store.TriggerCLI
	}
	trigger, err := store.ValidateTriggerType(trigger)
	if err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	if err := store.ValidateTriggerRequester(trigger, w.RequestedByUserID); err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	reqBy := store.RequestedByPtr(w.RequestedByUserID)

	hold, err := w.Store.AcquireAdvisoryLock(ctx, w.LockName)
	if err != nil {
		if errors.Is(err, store.ErrOperationLocked) {
			sum.Error = "another enrichpending worker is already running"
			if !dryRun {
				if _, rerr := w.Store.RejectOperationalRun(store.RejectOperationalRunInput{
					JobType:           store.JobEnrichment,
					TriggerType:       trigger,
					RequestedByUserID: reqBy,
					ErrorCode:         store.ErrCodeLockBusy,
				}); rerr != nil {
					w.Log.Warn("reject operational run failed", "err", rerr)
				}
			}
			return sum, fmt.Errorf("%w: %s", store.ErrOperationLocked, sum.Error)
		}
		sum.Error = "advisory lock error"
		w.Log.Error("advisory lock acquire failed", "err", err)
		return sum, err
	}
	sum.LockAcquired = true
	defer func() { _ = hold.Release() }()

	return w.runBatchLocked(ctx, limit, dryRun, concurrency, catchUp)
}

// runBatchLocked performs one enrichment batch. Caller must already hold the advisory lock
// for the desired session lifetime (single Run or multi-batch RunDrain).
func (w *EnrichPendingWorker) runBatchLocked(ctx context.Context, limit int, dryRun bool, concurrency int, catchUp bool) (EnrichPendingSummary, error) {
	sum := EnrichPendingSummary{
		DryRun:       dryRun,
		CatchUp:      catchUp,
		Limit:        limit,
		LockAcquired: true,
		Actions:      make([]EnrichPendingAction, 0, limit),
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > EnrichPendingMaxConc {
		concurrency = EnrichPendingMaxConc
	}

	trigger := w.TriggerType
	if trigger == "" {
		trigger = store.TriggerCLI
	}
	trigger, err := store.ValidateTriggerType(trigger)
	if err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	if err := store.ValidateTriggerRequester(trigger, w.RequestedByUserID); err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	reqBy := store.RequestedByPtr(w.RequestedByUserID)

	var opID uint64
	if !dryRun {
		opID, err = w.Store.StartOperationalRun(store.StartOperationalRunInput{
			JobType:           store.JobEnrichment,
			TriggerType:       trigger,
			RequestedByUserID: reqBy,
		})
		if err != nil {
			w.Log.Warn("start operational run failed", "err", err)
			opID = 0
		}
	}

	finishOp := func(status, errCode string, hasErr bool) {
		if dryRun || opID == 0 {
			return
		}
		in := store.FinishOperationalRunInput{
			ID:               opID,
			Status:           status,
			SelectedCount:    store.IntPtr(sum.Selected),
			SucceededCount:   store.IntPtr(sum.Enriched),
			SkippedCount:     store.IntPtr(sum.SkippedExisting),
			UnavailableCount: store.IntPtr(sum.Unavailable),
			FailedCount:      store.IntPtr(sum.Failures),
			ErrorCode:        errCode,
			HasError:         hasErr,
		}
		if ferr := w.Store.FinishOperationalRun(in); ferr != nil {
			w.Log.Warn("finish operational run failed", "err", ferr)
		}
	}

	// Store diversifies (≤3/video) then LIMITs; pass the worker --limit directly.
	cands, err := w.Store.ListPendingEnrichmentEpisodes(olehdtv.SourceSystem, limit, w.IncludeMembershipRetry, w.IncludeLegacyUnavailable)
	if err != nil {
		sum.Failures++
		sum.Error = "list pending: " + err.Error()
		code := store.ErrCodeDatabaseError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = store.ErrCodeTimeout
		}
		finishOp(store.OpStatusFailed, code, true)
		return sum, err
	}
	batch := SelectFairPendingBatch(cands, limit)
	sum.Selected = len(batch)
	if len(batch) == 0 {
		finishOp(store.OpStatusSucceeded, "", false)
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
	// dispatchCanceled is audit-only; pre-4A still returned nil when Error was set
	// during ctx.Done dispatch (even with Enriched==0), so we must not invent a
	// new timeout return from ctx.Err() after work completes.
	dispatchCanceled := false
	for _, ep := range batch {
		select {
		case <-ctx.Done():
			sum.Error = ctx.Err().Error()
			sum.Failures++
			dispatchCanceled = true
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

	// Exact pre-4A return semantics (d781153):
	// only Failures>0 && Enriched==0 && Error=="" returns a non-nil error.
	if sum.Failures > 0 && sum.Enriched == 0 && sum.Error == "" {
		sum.Error = fmt.Sprintf("%d enrichment failures", sum.Failures)
		finishOp(store.OpStatusFailed, store.ErrCodePartialFailure, true)
		return sum, fmt.Errorf("%s", sum.Error)
	}

	// Operational audit classification must not change the Run return value.
	switch {
	case sum.Failures == 0:
		finishOp(store.OpStatusSucceeded, "", false)
	case sum.Enriched > 0:
		finishOp(store.OpStatusFailed, store.ErrCodePartialFailure, true)
	case dispatchCanceled && errors.Is(ctx.Err(), context.DeadlineExceeded):
		// Deadline interrupted dispatch before any enrichment; Run still returns nil
		// (Error already set), but audit can record timeout.
		finishOp(store.OpStatusFailed, store.ErrCodeTimeout, true)
	case sum.Failures > 0:
		finishOp(store.OpStatusFailed, store.ErrCodePartialFailure, true)
	default:
		finishOp(store.OpStatusSucceeded, "", false)
	}
	return sum, nil
}

// ValidateEnrichPendingLegacyRetryFlags rejects unsafe --retry-legacy-unavailable use.
// Auth presence is checked by the CLI (must not log cookie contents).
func ValidateEnrichPendingLegacyRetryFlags(retryLegacy bool, drain bool, trigger string) error {
	if !retryLegacy {
		return nil
	}
	trig := trigger
	if trig == "" {
		trig = store.TriggerCLI
	}
	trig, err := store.ValidateTriggerType(trig)
	if err != nil {
		return err
	}
	if trig != store.TriggerCLI {
		return fmt.Errorf("enrichpending: --retry-legacy-unavailable requires --trigger=cli (manual only; not for timers)")
	}
	_ = drain // drain is allowed only with catch-up + cli; still never scheduled
	return nil
}

// ValidateEnrichPendingTargetFlags rejects unsafe single-episode CLI combinations.
func ValidateEnrichPendingTargetFlags(target EnrichPendingTarget, drain bool, trigger string) error {
	if !target.Active() {
		return nil
	}
	hasID := target.EpisodeID > 0
	src := strings.TrimSpace(target.SourceVideoID)
	hasSrcParts := src != "" || target.SID != 0 || target.NID != 0
	if hasID && hasSrcParts {
		return fmt.Errorf("enrichpending: --episode-id cannot be combined with --source-video-id/--sid/--nid")
	}
	if hasSrcParts && (src == "" || target.SID < 1 || target.NID < 1) {
		return fmt.Errorf("enrichpending: --source-video-id, --sid, and --nid must all be set together (sid/nid >= 1)")
	}
	if drain {
		return fmt.Errorf("enrichpending: single-episode targeting cannot be combined with --drain")
	}
	trig := trigger
	if trig == "" {
		trig = store.TriggerCLI
	}
	trig, err := store.ValidateTriggerType(trig)
	if err != nil {
		return err
	}
	if trig != store.TriggerCLI {
		return fmt.Errorf("enrichpending: single-episode targeting requires --trigger=cli (manual only)")
	}
	return nil
}

// ValidateEnrichPendingEpisodeIDFlags rejects unsafe combinations for --episode-id.
func ValidateEnrichPendingEpisodeIDFlags(episodeID uint64, drain bool, trigger string) error {
	return ValidateEnrichPendingTargetFlags(EnrichPendingTarget{EpisodeID: episodeID}, drain, trigger)
}

// RunOneEpisode processes a single episode by id (manual --episode-id).
func (w *EnrichPendingWorker) RunOneEpisode(ctx context.Context, episodeID uint64, dryRun bool) (EnrichPendingSummary, error) {
	return w.RunOneTarget(ctx, EnrichPendingTarget{EpisodeID: episodeID}, dryRun)
}

// RunOneTarget processes exactly one eligible episode by id or source_video_id/sid/nid.
// Unavailable historical rows require IncludeMembershipRetry (auth configured).
// Does not overwrite non-empty playback_url; uses the same ClassifyPlayPage path.
func (w *EnrichPendingWorker) RunOneTarget(ctx context.Context, target EnrichPendingTarget, dryRun bool) (EnrichPendingSummary, error) {
	sum := EnrichPendingSummary{
		DryRun:  dryRun,
		Limit:   1,
		Actions: make([]EnrichPendingAction, 0, 1),
	}
	if !target.Active() {
		sum.Error = "single-episode target required"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if w.Store == nil || w.Fetcher == nil {
		sum.Error = "store and fetcher are required"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if err := ValidateEnrichPendingTargetFlags(target, false, w.TriggerType); err != nil {
		sum.Error = err.Error()
		return sum, err
	}

	hold, err := w.Store.AcquireAdvisoryLock(ctx, w.LockName)
	if err != nil {
		if errors.Is(err, store.ErrOperationLocked) {
			sum.Error = "another enrichpending worker is already running"
			return sum, fmt.Errorf("%w: %s", store.ErrOperationLocked, sum.Error)
		}
		sum.Error = "advisory lock error"
		return sum, err
	}
	sum.LockAcquired = true
	defer func() { _ = hold.Release() }()

	var ep store.PendingEnrichmentEpisode
	var ok bool
	if target.EpisodeID > 0 {
		ep, ok, err = w.Store.GetEnrichmentCandidateByID(olehdtv.SourceSystem, target.EpisodeID)
		if !ok && err == nil {
			sum.Error = fmt.Sprintf("episode %d not found or not eligible (need empty playback_url, non-empty play_page_url, pending_enrichment|unavailable)", target.EpisodeID)
		}
	} else {
		src := strings.TrimSpace(target.SourceVideoID)
		ep, ok, err = w.Store.GetEnrichmentCandidateBySourceRef(olehdtv.SourceSystem, src, target.SID, target.NID)
		if !ok && err == nil {
			sum.Error = fmt.Sprintf("episode source_video_id=%s sid=%d nid=%d not found or not eligible (need empty playback_url, non-empty play_page_url, pending_enrichment|unavailable)", src, target.SID, target.NID)
		}
	}
	if err != nil {
		sum.Failures++
		sum.Error = "load episode: " + err.Error()
		return sum, err
	}
	if !ok {
		return sum, fmt.Errorf("%s", sum.Error)
	}
	st := strings.ToLower(strings.TrimSpace(ep.PlaybackStatus))
	if st == "unavailable" {
		reason := strings.TrimSpace(ep.RestrictionReason)
		switch {
		case reason == store.RestrictionReasonMembership:
			if !w.IncludeMembershipRetry {
				sum.Error = "source_restricted retry requires OLEHDTV authentication and catch-up/manual targeting"
				return sum, fmt.Errorf("%s", sum.Error)
			}
		case reason == "":
			if !w.IncludeLegacyUnavailable {
				sum.Error = "legacy unavailable (empty restriction_reason) requires authenticated manual catch-up"
				return sum, fmt.Errorf("%s", sum.Error)
			}
		default:
			sum.Error = "episode restriction_reason is not eligible for authenticated retry"
			return sum, fmt.Errorf("%s", sum.Error)
		}
	}

	sum.Selected = 1
	sum.Attempted = 1
	r := w.processOne(ctx, ep, dryRun)
	if r.req {
		sum.RequestsMade++
	}
	sum.Actions = append(sum.Actions, r.action)
	if r.enr {
		sum.Enriched++
	}
	if r.unav {
		sum.Unavailable++
	}
	if r.pend {
		sum.StillPending++
	}
	if r.fail {
		sum.Failures++
	}
	if r.skip {
		sum.SkippedExisting++
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

	// Confirmed OLEHDTV membership/permission page → terminal unavailable (not retryable).
	if maccms.ClassifyPlayPage(body) == maccms.PlayPageSourceRestricted {
		if dryRun {
			r.unav = true
			act.Action = "WOULD_UNAVAILABLE"
			act.Detail = "source_restricted"
			r.action = act
			return r
		}
		updated, uerr := w.Store.ApplyPlaybackEnrichment(ep.ID, "", ep.PlaybackSource, "unavailable", store.RestrictionReasonMembership)
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
		act.Detail = "source_restricted"
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
		updated, uerr := w.Store.ApplyPlaybackEnrichment(ep.ID, "", from, "unavailable", "")
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

	updated, uerr := w.Store.ApplyPlaybackEnrichment(ep.ID, url, from, "ok", "")
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
