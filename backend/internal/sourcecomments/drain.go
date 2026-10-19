package sourcecomments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

const (
	// DrainBatchSize is how many pending videos are selected between catalog queries.
	DrainBatchSize = 100
	// MaxDrainVideos is the largest --max-videos value one drain invocation may use.
	MaxDrainVideos = 20000
	// DrainDryRunPreview is how many titles a drain dry-run prints.
	DrainDryRunPreview = 10
)

// ErrInterrupted means the operator stopped the process. Completed checkpoints stay completed.
var ErrInterrupted = errors.New("interrupted")

// DrainCatalog selects backlog pages without loading the whole catalog.
type DrainCatalog interface {
	ListPendingBackfillVideos(sourceSystem, mode string, afterID uint64, limit int) ([]store.SourceVideoCandidate, error)
	ListFailedBackfillVideos(sourceSystem, mode string) ([]store.SourceVideoCandidate, error)
	FirstFailedBackfill(sourceSystem, mode string) (*store.BackfillCheckpoint, error)
	CountBackfillCheckpointStatus(sourceSystem, mode string) (completed, failed int, err error)
	CountRemainingBackfill(sourceSystem, mode string) (int, error)
	SaveBackfillCheckpoint(row store.BackfillCheckpoint, now time.Time) error
	CountSourceCommentRows(videoID uint64, sourceSystem string) (parents, replies int, err error)
}

// RunDrain walks the historical backlog in small id-ordered batches.
// It commits each video's checkpoint on its own. It stops on the first failure,
// on interrupt, when the backlog is empty, or when MaxVideos is reached.
func RunDrain(ctx context.Context, comments store.SourceCommentRepo, catalog DrainCatalog, fetcher PageFetcher, opts BackfillOptions) (BackfillSummary, error) {
	started := time.Now()
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	var summary BackfillSummary
	if err := validateDrainMax(opts.MaxVideos); err != nil {
		return summary, err
	}
	if comments == nil || catalog == nil {
		return summary, fmt.Errorf("comment backfill is not configured")
	}
	if !opts.DryRun && fetcher == nil {
		return summary, fmt.Errorf("comment backfill is not configured")
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = DrainBatchSize
	}

	blocked, err := catalog.FirstFailedBackfill(opts.SourceSystem, store.BackfillModeFull)
	if err != nil {
		return summary, err
	}
	completed, failed, err := catalog.CountBackfillCheckpointStatus(opts.SourceSystem, store.BackfillModeFull)
	if err != nil {
		return summary, err
	}
	remaining, err := catalog.CountRemainingBackfill(opts.SourceSystem, store.BackfillModeFull)
	if err != nil {
		return summary, err
	}
	if blocked != nil && !opts.RetryFailed {
		fmt.Fprintf(out, "failed checkpoint video_id=%d source_id=%s requires --retry-failed\n", blocked.VideoID, blocked.SourceVideoID)
		fmt.Fprintf(out, "catalog_completed=%d catalog_failed=%d remaining_pending=%d could_process=0\n", completed, failed, remaining)
		summary.Elapsed = time.Since(started)
		printDrainProgress(out, summary, remaining)
		return summary, ErrFailedCheckpoint
	}

	could := remaining
	if could > opts.MaxVideos {
		could = opts.MaxVideos
	}
	if opts.DryRun {
		fmt.Fprintf(out, "dry_run drain=1 catalog_completed=%d catalog_failed=%d remaining_pending=%d could_process=%d\n",
			completed, failed, remaining, could)
		preview, err := drainPreview(catalog, opts, could)
		if err != nil {
			return summary, err
		}
		show := preview
		truncated := false
		if len(show) > DrainDryRunPreview {
			show = show[:DrainDryRunPreview]
			truncated = true
		}
		for i, video := range show {
			fmt.Fprintf(out, "[%d] video_id=%d source_id=%s title=%q\n", i+1, video.ID, video.SourceID, video.Title)
		}
		if truncated {
			fmt.Fprintf(out, "preview_truncated=1\n")
		}
		summary.Selected = could
		summary.Elapsed = time.Since(started)
		printDrainProgress(out, summary, remaining)
		return summary, nil
	}

	if opts.RetryFailed && blocked != nil {
		failedVideos, err := catalog.ListFailedBackfillVideos(opts.SourceSystem, store.BackfillModeFull)
		if err != nil {
			return summary, err
		}
		stop, interrupted, err := processDrainVideos(ctx, comments, catalog, fetcher, opts, out, &summary, failedVideos)
		if err != nil || stop || interrupted {
			return finishDrain(out, &summary, catalog, opts, started, stop, interrupted, err)
		}
	}

	after := uint64(0)
	for summary.Completed+summary.Failed < opts.MaxVideos {
		if err := ctx.Err(); err != nil {
			return finishDrain(out, &summary, catalog, opts, started, false, true, nil)
		}
		room := opts.MaxVideos - (summary.Completed + summary.Failed)
		take := batch
		if take > room {
			take = room
		}
		page, err := catalog.ListPendingBackfillVideos(opts.SourceSystem, store.BackfillModeFull, after, take)
		if err != nil {
			return summary, err
		}
		if len(page) == 0 {
			summary.Elapsed = time.Since(started)
			remaining, err = catalog.CountRemainingBackfill(opts.SourceSystem, store.BackfillModeFull)
			if err != nil {
				return summary, err
			}
			printDrainProgress(out, summary, remaining)
			fmt.Fprintln(out, "BACKFILL DRAIN COMPLETE")
			return summary, nil
		}
		stop, interrupted, err := processDrainVideos(ctx, comments, catalog, fetcher, opts, out, &summary, page)
		if len(page) > 0 {
			after = page[len(page)-1].ID
		}
		if err != nil || stop || interrupted {
			return finishDrain(out, &summary, catalog, opts, started, stop, interrupted, err)
		}
		if err := printLiveProgress(out, &summary, catalog, opts, started); err != nil {
			return summary, err
		}
		if summary.Completed+summary.Failed >= opts.MaxVideos {
			break
		}
	}
	summary.Elapsed = time.Since(started)
	remaining, err = catalog.CountRemainingBackfill(opts.SourceSystem, store.BackfillModeFull)
	if err != nil {
		return summary, err
	}
	printDrainProgress(out, summary, remaining)
	fmt.Fprintln(out, "BACKFILL DRAIN PAUSED: safety ceiling reached")
	return summary, nil
}

func drainPreview(catalog DrainCatalog, opts BackfillOptions, could int) ([]store.SourceVideoCandidate, error) {
	if could <= 0 {
		return nil, nil
	}
	var out []store.SourceVideoCandidate
	if opts.RetryFailed {
		failed, err := catalog.ListFailedBackfillVideos(opts.SourceSystem, store.BackfillModeFull)
		if err != nil {
			return nil, err
		}
		out = append(out, failed...)
	}
	need := could
	if need > DrainDryRunPreview+1 {
		need = DrainDryRunPreview + 1
	}
	if len(out) < need {
		pending, err := catalog.ListPendingBackfillVideos(opts.SourceSystem, store.BackfillModeFull, 0, need-len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, pending...)
	}
	if len(out) > need {
		out = out[:need]
	}
	return out, nil
}

func processDrainVideos(ctx context.Context, comments store.SourceCommentRepo, catalog DrainCatalog, fetcher PageFetcher, opts BackfillOptions, out io.Writer, summary *BackfillSummary, videos []store.SourceVideoCandidate) (ceiling, interrupted bool, err error) {
	for _, video := range videos {
		if summary.Completed+summary.Failed >= opts.MaxVideos {
			return true, false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, true, nil
		}
		kind, syncErr := processDrainVideo(ctx, comments, catalog, fetcher, opts, out, summary, video)
		if kind == drainInterrupted {
			return false, true, nil
		}
		if kind == drainFailed {
			return false, false, syncErr
		}
	}
	return false, false, nil
}

type drainStop int

const (
	drainOK drainStop = iota
	drainFailed
	drainInterrupted
)

func processDrainVideo(ctx context.Context, comments store.SourceCommentRepo, catalog DrainCatalog, fetcher PageFetcher, opts BackfillOptions, out io.Writer, summary *BackfillSummary, video store.SourceVideoCandidate) (drainStop, error) {
	if err := ctx.Err(); err != nil {
		return drainInterrupted, nil
	}
	if err := catalog.SaveBackfillCheckpoint(store.BackfillCheckpoint{
		SourceSystem:  opts.SourceSystem,
		Mode:          store.BackfillModeFull,
		VideoID:       video.ID,
		SourceVideoID: video.SourceID,
		Status:        store.BackfillPending,
		StartedAt:     opts.Now,
	}, opts.Now); err != nil {
		return drainFailed, err
	}
	if err := ctx.Err(); err != nil {
		return drainInterrupted, nil
	}
	n := summary.Completed + summary.Failed + 1
	result, syncErr := Sync(ctx, comments, fetcher, video.ID, opts.SourceSystem, video.SourceID, ModeFull, opts.Now)
	if syncErr != nil {
		if ctx.Err() != nil || isInterrupt(syncErr) {
			return drainInterrupted, nil
		}
		message := syncErr.Error()
		if len(result.Errors) > 0 {
			message = result.Errors[0]
		}
		_ = catalog.SaveBackfillCheckpoint(store.BackfillCheckpoint{
			SourceSystem:  opts.SourceSystem,
			Mode:          store.BackfillModeFull,
			VideoID:       video.ID,
			SourceVideoID: video.SourceID,
			Status:        store.BackfillFailed,
			PagesFetched:  result.PagesFetched,
			Discovered:    result.Discovered,
			Parents:       result.Parents,
			Replies:       result.Replies,
			Inserted:      result.Inserted,
			Updated:       result.Updated,
			Skipped:       result.Skipped,
			ErrorMessage:  message,
			StartedAt:     opts.Now,
			CompletedAt:   opts.Now,
		}, opts.Now)
		summary.Failed++
		addResult(summary, result)
		fmt.Fprintf(out, "[%d] video_id=%d source_id=%s title=%q pages=%d discovered=%d inserted=%d updated=%d skipped=%d FAIL %s\n",
			n, video.ID, video.SourceID, video.Title, result.PagesFetched, result.Discovered, result.Inserted, result.Updated, result.Skipped, message)
		return drainFailed, syncErr
	}
	parents, replies, err := catalog.CountSourceCommentRows(video.ID, opts.SourceSystem)
	if err != nil {
		_ = catalog.SaveBackfillCheckpoint(failedCountCheckpoint(opts, video, result, err), opts.Now)
		summary.Failed++
		addResult(summary, result)
		return drainFailed, err
	}
	if err := acceptStoredTotals(result.PagerTotal, parents, replies); err != nil {
		_ = catalog.SaveBackfillCheckpoint(failedCountCheckpoint(opts, video, result, err), opts.Now)
		summary.Failed++
		addResult(summary, result)
		return drainFailed, err
	}
	if err := catalog.SaveBackfillCheckpoint(store.BackfillCheckpoint{
		SourceSystem:  opts.SourceSystem,
		Mode:          store.BackfillModeFull,
		VideoID:       video.ID,
		SourceVideoID: video.SourceID,
		Status:        store.BackfillCompleted,
		PagesFetched:  result.PagesFetched,
		Discovered:    result.Discovered,
		Parents:       parents,
		Replies:       replies,
		Inserted:      result.Inserted,
		Updated:       result.Updated,
		Skipped:       result.Skipped,
		StartedAt:     opts.Now,
		CompletedAt:   opts.Now,
	}, opts.Now); err != nil {
		return drainFailed, err
	}
	summary.Completed++
	addResult(summary, result)
	fmt.Fprintf(out, "[%d] video_id=%d source_id=%s title=%q pages=%d discovered=%d inserted=%d updated=%d skipped=%d parents=%d replies=%d OK\n",
		n, video.ID, video.SourceID, video.Title, result.PagesFetched, result.Discovered, result.Inserted, result.Updated, result.Skipped, parents, replies)
	return drainOK, nil
}

func addResult(summary *BackfillSummary, result Result) {
	summary.Pages += result.PagesFetched
	summary.Discovered += result.Discovered
	summary.Inserted += result.Inserted
	summary.Updated += result.Updated
	summary.Skipped += result.Skipped
}

func finishDrain(out io.Writer, summary *BackfillSummary, catalog DrainCatalog, opts BackfillOptions, started time.Time, ceiling, interrupted bool, cause error) (BackfillSummary, error) {
	summary.Elapsed = time.Since(started)
	remaining, err := catalog.CountRemainingBackfill(opts.SourceSystem, store.BackfillModeFull)
	if err != nil {
		return *summary, err
	}
	printDrainProgress(out, *summary, remaining)
	switch {
	case interrupted:
		fmt.Fprintln(out, "interrupted")
		return *summary, ErrInterrupted
	case cause != nil:
		return *summary, cause
	case ceiling:
		fmt.Fprintln(out, "BACKFILL DRAIN PAUSED: safety ceiling reached")
		return *summary, nil
	default:
		return *summary, nil
	}
}

func printLiveProgress(out io.Writer, summary *BackfillSummary, catalog DrainCatalog, opts BackfillOptions, started time.Time) error {
	summary.Elapsed = time.Since(started)
	remaining, err := catalog.CountRemainingBackfill(opts.SourceSystem, store.BackfillModeFull)
	if err != nil {
		return err
	}
	printDrainProgress(out, *summary, remaining)
	return nil
}

func printDrainProgress(out io.Writer, summary BackfillSummary, remaining int) {
	fmt.Fprintf(out, "drain_progress:\n  processed_this_run=%d\n  completed_this_run=%d\n  failed_this_run=%d\n  pages_fetched=%d\n  comments_discovered=%d\n  inserted=%d\n  updated=%d\n  skipped=%d\n  remaining_pending=%d\n  elapsed=%s\n",
		summary.Completed+summary.Failed, summary.Completed, summary.Failed, summary.Pages, summary.Discovered, summary.Inserted, summary.Updated, summary.Skipped, remaining, summary.Elapsed.Round(time.Millisecond))
}

func validateDrainMax(maxVideos int) error {
	if maxVideos <= 0 {
		return fmt.Errorf("missing --max-videos; no videos were selected")
	}
	if maxVideos > MaxDrainVideos {
		return fmt.Errorf("--max-videos %d exceeds %d", maxVideos, MaxDrainVideos)
	}
	return nil
}

func isInterrupt(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
