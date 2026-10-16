package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

const (
	EnrichPendingDrainMaxBatchesDefault = 40
	EnrichPendingDrainMaxBatchesMin     = 1
	EnrichPendingDrainMaxBatchesMax     = 100
	EnrichPendingDrainMaxRuntimeDefault = 60 * time.Minute
	EnrichPendingDrainMaxRuntimeHardMax = 3 * time.Hour
)

// Drain stop reasons (aggregate summary).
const (
	DrainStopEmpty       = "empty"
	DrainStopMaxBatches  = "max_batches"
	DrainStopMaxRuntime  = "max_runtime"
	DrainStopInterrupted = "interrupted"
	DrainStopFailures    = "failures"
	DrainStopNoProgress  = "no_progress"
	DrainStopLockBusy    = "lock_busy"
	DrainStopError       = "error"
	DrainStopValidation  = "validation"
)

// EnrichPendingDrainOptions configures a manual multi-batch catch-up drain.
type EnrichPendingDrainOptions struct {
	Limit       int
	Concurrency int
	MaxBatches  int
	MaxRuntime  time.Duration // informational / validated; caller supplies ctx budget
}

// EnrichPendingDrainSummary is the concise aggregate report for --drain (no per-episode actions).
type EnrichPendingDrainSummary struct {
	Drain             bool   `json:"drain"`
	CatchUp           bool   `json:"catch_up"`
	Limit             int    `json:"limit"`
	Concurrency       int    `json:"concurrency"`
	MaxBatches        int    `json:"max_batches"`
	BatchesRun        int    `json:"batches_run"`
	TotalSelected     int    `json:"total_selected"`
	TotalAttempted    int    `json:"total_attempted"`
	TotalEnriched     int    `json:"total_enriched"`
	TotalUnavailable  int    `json:"total_unavailable"`
	TotalFailures     int    `json:"total_failures"`
	TotalStillPending int    `json:"total_still_pending"`
	TotalSkipped      int    `json:"total_skipped_existing"`
	StopReason        string `json:"stop_reason"`
	Elapsed           string `json:"elapsed"`
	Error             string `json:"error,omitempty"`
}

// ValidateEnrichPendingDrainFlags enforces --drain requires --catch-up and rejects --dry-run.
func ValidateEnrichPendingDrainFlags(catchUp, drain, dryRun bool) error {
	if !drain {
		return nil
	}
	if !catchUp {
		return fmt.Errorf("enrichpending: --drain requires --catch-up")
	}
	if dryRun {
		return fmt.Errorf("enrichpending: --drain cannot be combined with --dry-run")
	}
	return nil
}

// ValidateEnrichPendingDrainOptions validates catch-up limit plus drain bounds.
func ValidateEnrichPendingDrainOptions(limit, maxBatches int, maxRuntime time.Duration) error {
	if err := ValidateEnrichPendingLimitMode(limit, true); err != nil {
		return err
	}
	if maxBatches < EnrichPendingDrainMaxBatchesMin || maxBatches > EnrichPendingDrainMaxBatchesMax {
		return fmt.Errorf("enrichpending: --max-batches must be %d..%d", EnrichPendingDrainMaxBatchesMin, EnrichPendingDrainMaxBatchesMax)
	}
	if maxRuntime <= 0 {
		return fmt.Errorf("enrichpending: --max-runtime must be positive")
	}
	if maxRuntime > EnrichPendingDrainMaxRuntimeHardMax {
		return fmt.Errorf("enrichpending: --max-runtime exceeds hard max %s", EnrichPendingDrainMaxRuntimeHardMax)
	}
	return nil
}

// batchResolvedProgress counts rows that left the pending_enrichment selection set.
//
// SkippedExisting is included because processOne only sets skip when
// ApplyPlaybackEnrichment returns updated=false, and that UPDATE is gated on
// pending_enrichment + empty playback_url — so the row can no longer be selected.
func batchResolvedProgress(sum EnrichPendingSummary) int {
	return sum.Enriched + sum.Unavailable + sum.SkippedExisting
}

// evaluateDrainBatch decides whether to stop after one batch (pure; testable).
// When Failures>0 but ctx is canceled/deadline and all observed failures are
// cancellation-shaped, prefer interrupted/max_runtime over failures.
func evaluateDrainBatch(ctx context.Context, sum EnrichPendingSummary, batchErr error) (stop bool, reason string, err error) {
	if sum.Selected == 0 {
		return true, DrainStopEmpty, nil
	}
	if sum.Failures > 0 {
		if ctx != nil && ctx.Err() != nil && !sumHasNonCancellationFailure(sum, ctx) {
			cerr := ctx.Err()
			return true, drainStopReasonFromContext(cerr), cerr
		}
		msg := sum.Error
		if msg == "" {
			msg = fmt.Sprintf("%d enrichment failures", sum.Failures)
		}
		return true, DrainStopFailures, fmt.Errorf("%s", msg)
	}
	if batchResolvedProgress(sum) == 0 {
		return true, DrainStopNoProgress, fmt.Errorf("selected %d but resolved 0 (enriched+unavailable+skipped)", sum.Selected)
	}
	if batchErr != nil {
		return true, DrainStopError, batchErr
	}
	return false, "", nil
}

// sumHasNonCancellationFailure reports genuine processing failures (fetch/parse/DB)
// that are not solely attributable to ctx cancellation/deadline.
func sumHasNonCancellationFailure(sum EnrichPendingSummary, ctx context.Context) bool {
	if ctx == nil || ctx.Err() == nil {
		return sum.Failures > 0
	}
	cerr := ctx.Err().Error()
	sawFailureAction := false
	for _, a := range sum.Actions {
		switch a.Action {
		case "FAILURE", "STILL_PENDING":
			sawFailureAction = true
			if !detailLooksLikeCancellation(a.Detail, cerr) {
				return true
			}
		}
	}
	if sawFailureAction {
		// Per-item outcomes decide; ignore aggregate "N enrichment failures" strings.
		return false
	}
	// Dispatch-cancel path sets sum.Error to ctx.Err().Error() with no per-item actions.
	if sum.Error != "" && !detailLooksLikeCancellation(sum.Error, cerr) {
		return true
	}
	return false
}

func detailLooksLikeCancellation(detail, ctxErr string) bool {
	d := strings.ToLower(strings.TrimSpace(detail))
	if d == "" {
		return false
	}
	ce := strings.ToLower(strings.TrimSpace(ctxErr))
	if ce != "" && (d == ce || strings.Contains(d, ce)) {
		return true
	}
	return strings.Contains(d, "context canceled") ||
		strings.Contains(d, "context cancelled") ||
		strings.Contains(d, "context deadline exceeded")
}

// RunDrain repeatedly runs bounded catch-up batches under one advisory lock session.
// Each batch writes its own operational_runs row via runBatchLocked.
// ctx should already incorporate --max-runtime and signal cancellation.
func (w *EnrichPendingWorker) RunDrain(ctx context.Context, opt EnrichPendingDrainOptions) (EnrichPendingDrainSummary, error) {
	start := time.Now()
	out := EnrichPendingDrainSummary{
		Drain:       true,
		CatchUp:     true,
		Limit:       opt.Limit,
		Concurrency: opt.Concurrency,
		MaxBatches:  opt.MaxBatches,
	}
	if out.Concurrency <= 0 {
		out.Concurrency = 1
	}
	if out.Concurrency > EnrichPendingMaxConc {
		out.Concurrency = EnrichPendingMaxConc
	}

	if err := ValidateEnrichPendingDrainOptions(opt.Limit, opt.MaxBatches, opt.MaxRuntime); err != nil {
		out.StopReason = DrainStopValidation
		out.Error = err.Error()
		out.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return out, err
	}
	if w.Store == nil || w.Fetcher == nil {
		out.StopReason = DrainStopError
		out.Error = "store and fetcher are required"
		out.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return out, fmt.Errorf("%s", out.Error)
	}

	trigger := w.TriggerType
	if trigger == "" {
		trigger = store.TriggerCLI
	}
	trigger, err := store.ValidateTriggerType(trigger)
	if err != nil {
		out.StopReason = DrainStopValidation
		out.Error = err.Error()
		out.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return out, err
	}
	if err := store.ValidateTriggerRequester(trigger, w.RequestedByUserID); err != nil {
		out.StopReason = DrainStopValidation
		out.Error = err.Error()
		out.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return out, err
	}
	reqBy := store.RequestedByPtr(w.RequestedByUserID)

	hold, err := w.Store.AcquireAdvisoryLock(ctx, w.LockName)
	if err != nil {
		if errors.Is(err, store.ErrOperationLocked) {
			out.StopReason = DrainStopLockBusy
			out.Error = "another enrichpending worker is already running"
			if _, rerr := w.Store.RejectOperationalRun(store.RejectOperationalRunInput{
				JobType:           store.JobEnrichment,
				TriggerType:       trigger,
				RequestedByUserID: reqBy,
				ErrorCode:         store.ErrCodeLockBusy,
			}); rerr != nil {
				w.Log.Warn("reject operational run failed", "err", rerr)
			}
			out.Elapsed = time.Since(start).Round(time.Millisecond).String()
			return out, fmt.Errorf("%w: %s", store.ErrOperationLocked, out.Error)
		}
		out.StopReason = DrainStopError
		out.Error = "advisory lock error"
		out.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return out, err
	}
	defer func() { _ = hold.Release() }()

	var runErr error
	for batchIdx := 0; batchIdx < opt.MaxBatches; batchIdx++ {
		if err := ctx.Err(); err != nil {
			out.StopReason = drainStopReasonFromContext(err)
			out.Error = err.Error()
			runErr = err
			break
		}

		sum, err := w.runBatchLocked(ctx, opt.Limit, false /*dryRun*/, out.Concurrency, true /*catchUp*/)
		out.BatchesRun++
		out.TotalSelected += sum.Selected
		out.TotalAttempted += sum.Attempted
		out.TotalEnriched += sum.Enriched
		out.TotalUnavailable += sum.Unavailable
		out.TotalFailures += sum.Failures
		out.TotalStillPending += sum.StillPending
		out.TotalSkipped += sum.SkippedExisting

		stop, reason, stopErr := evaluateDrainBatch(ctx, sum, err)
		if stop {
			out.StopReason = reason
			if stopErr != nil {
				out.Error = stopErr.Error()
			}
			runErr = stopErr
			break
		}
		if out.BatchesRun >= opt.MaxBatches {
			out.StopReason = DrainStopMaxBatches
			runErr = nil
			break
		}
	}

	if out.StopReason == "" {
		// Loop exited because max batches exhausted without empty/failure.
		out.StopReason = DrainStopMaxBatches
	}
	out.Elapsed = time.Since(start).Round(time.Millisecond).String()
	return out, runErr
}

func drainStopReasonFromContext(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return DrainStopMaxRuntime
	}
	if errors.Is(err, context.Canceled) {
		return DrainStopInterrupted
	}
	return DrainStopInterrupted
}

// DrainExitCode maps drain stop reasons to process exit codes.
// Expected budgets (empty / max_batches / max_runtime) → 0.
// Interrupted / failures / no-progress / lock / validation → non-zero.
func DrainExitCode(sum EnrichPendingDrainSummary, err error) int {
	switch sum.StopReason {
	case DrainStopEmpty, DrainStopMaxBatches, DrainStopMaxRuntime:
		return 0
	case DrainStopInterrupted, DrainStopFailures, DrainStopNoProgress, DrainStopLockBusy, DrainStopError, DrainStopValidation:
		return 1
	default:
		if err != nil {
			return 1
		}
		return 0
	}
}
