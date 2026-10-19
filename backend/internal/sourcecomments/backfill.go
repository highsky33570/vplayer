package sourcecomments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

// MaxBackfillLimit is the most videos one invocation may import.
// There is no unlimited default.
const MaxBackfillLimit = 100

// ErrFailedCheckpoint means a previous video failed and the operator did not pass --retry-failed.
var ErrFailedCheckpoint = errors.New("failed checkpoint requires --retry-failed")

// BackfillOptions selects one bounded historical batch.
// Mode is always the full walk. Confirmation is enforced by the operator command
// before RunBackfill is called.
type BackfillOptions struct {
	SourceSystem string
	Limit        int
	MaxVideos    int
	BatchSize    int
	RetryFailed  bool
	DryRun       bool
	Now          time.Time
	Out          io.Writer
	Locker       Locker
}

// BackfillSummary is the counters printed at the end of one invocation.
type BackfillSummary struct {
	Selected   int
	Completed  int
	Failed     int
	Pages      int
	Discovered int
	Inserted   int
	Updated    int
	Skipped    int
	Elapsed    time.Duration
}

// BackfillCatalog is the read/write surface for resume state.
// Implementations must not update video catalog status or native comments.
type BackfillCatalog interface {
	ListActiveSourceVideos(sourceSystem string) ([]store.SourceVideoCandidate, error)
	ListBackfillCheckpoints(sourceSystem, mode string) ([]store.BackfillCheckpoint, error)
	SaveBackfillCheckpoint(row store.BackfillCheckpoint, now time.Time) error
	CountSourceCommentRows(videoID uint64, sourceSystem string) (parents, replies int, err error)
}

// RunBackfill imports public comments for a bounded, ordered set of videos.
// It processes one video at a time and stops the invocation on the first failure.
// Completed videos stay checkpointed and are not read again.
//
// Historical backfill uses ModeFull. Completion of those checkpoints does not skip
// later maintenance. Ongoing incremental and full repair live in maintainsourcecomments.
func RunBackfill(ctx context.Context, comments store.SourceCommentRepo, catalog BackfillCatalog, fetcher PageFetcher, opts BackfillOptions) (BackfillSummary, error) {
	started := time.Now()
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	var summary BackfillSummary
	if err := validateBackfillLimit(opts.Limit); err != nil {
		return summary, err
	}
	if comments == nil || catalog == nil {
		return summary, fmt.Errorf("comment backfill is not configured")
	}
	if !opts.DryRun && fetcher == nil {
		return summary, fmt.Errorf("comment backfill is not configured")
	}
	release, err := acquireCommentLock(ctx, opts.Locker)
	if err != nil {
		return summary, err
	}
	defer release()

	videos, err := catalog.ListActiveSourceVideos(opts.SourceSystem)
	if err != nil {
		return summary, err
	}
	checks, err := catalog.ListBackfillCheckpoints(opts.SourceSystem, store.BackfillModeFull)
	if err != nil {
		return summary, err
	}
	selected, blocked, counts := selectBackfillVideos(videos, checks, opts.SourceSystem, opts.Limit, opts.RetryFailed)
	if blocked != nil {
		fmt.Fprintf(out, "failed checkpoint video_id=%d source_id=%s requires --retry-failed\n", blocked.VideoID, blocked.SourceVideoID)
		fmt.Fprintf(out, "catalog_completed=%d catalog_failed=%d catalog_pending=%d\n", counts.Completed, counts.Failed, counts.Pending)
		summary.Elapsed = time.Since(started)
		printBackfillSummary(out, summary)
		return summary, ErrFailedCheckpoint
	}
	summary.Selected = len(selected)
	if opts.DryRun {
		fmt.Fprintf(out, "dry_run selected=%d catalog_completed=%d catalog_failed=%d catalog_pending=%d\n",
			len(selected), counts.Completed, counts.Failed, counts.Pending)
		for i, video := range selected {
			fmt.Fprintf(out, "[%d/%d] video_id=%d source_id=%s title=%q\n", i+1, len(selected), video.ID, video.SourceID, video.Title)
		}
		summary.Elapsed = time.Since(started)
		printBackfillSummary(out, summary)
		return summary, nil
	}

	for i, video := range selected {
		if err := ctx.Err(); err != nil {
			summary.Elapsed = time.Since(started)
			printBackfillSummary(out, summary)
			return summary, err
		}
		if err := catalog.SaveBackfillCheckpoint(store.BackfillCheckpoint{
			SourceSystem:  opts.SourceSystem,
			Mode:          store.BackfillModeFull,
			VideoID:       video.ID,
			SourceVideoID: video.SourceID,
			Status:        store.BackfillPending,
			StartedAt:     opts.Now,
		}, opts.Now); err != nil {
			summary.Elapsed = time.Since(started)
			printBackfillSummary(out, summary)
			return summary, err
		}
		result, syncErr := Sync(ctx, comments, fetcher, video.ID, opts.SourceSystem, video.SourceID, ModeFull, opts.Now)
		if syncErr != nil {
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
			summary.Pages += result.PagesFetched
			summary.Discovered += result.Discovered
			summary.Inserted += result.Inserted
			summary.Updated += result.Updated
			summary.Skipped += result.Skipped
			fmt.Fprintf(out, "[%d/%d] video_id=%d source_id=%s title=%q pages=%d discovered=%d inserted=%d updated=%d skipped=%d FAIL %s\n",
				i+1, len(selected), video.ID, video.SourceID, video.Title, result.PagesFetched, result.Discovered, result.Inserted, result.Updated, result.Skipped, message)
			summary.Elapsed = time.Since(started)
			printBackfillSummary(out, summary)
			return summary, syncErr
		}
		parents, replies, err := catalog.CountSourceCommentRows(video.ID, opts.SourceSystem)
		if err != nil {
			_ = catalog.SaveBackfillCheckpoint(failedCountCheckpoint(opts, video, result, err), opts.Now)
			summary.Failed++
			summary.Elapsed = time.Since(started)
			printBackfillSummary(out, summary)
			return summary, err
		}
		// PagerTotal counts top-level comments only. parents+replies may be larger.
		if err := acceptStoredTotals(result.PagerTotal, parents, replies); err != nil {
			_ = catalog.SaveBackfillCheckpoint(failedCountCheckpoint(opts, video, result, err), opts.Now)
			summary.Failed++
			summary.Elapsed = time.Since(started)
			printBackfillSummary(out, summary)
			return summary, err
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
			summary.Elapsed = time.Since(started)
			printBackfillSummary(out, summary)
			return summary, err
		}
		summary.Completed++
		summary.Pages += result.PagesFetched
		summary.Discovered += result.Discovered
		summary.Inserted += result.Inserted
		summary.Updated += result.Updated
		summary.Skipped += result.Skipped
		fmt.Fprintf(out, "[%d/%d] video_id=%d source_id=%s title=%q pages=%d discovered=%d inserted=%d updated=%d skipped=%d parents=%d replies=%d OK\n",
			i+1, len(selected), video.ID, video.SourceID, video.Title, result.PagesFetched, result.Discovered, result.Inserted, result.Updated, result.Skipped, parents, replies)
	}
	summary.Elapsed = time.Since(started)
	printBackfillSummary(out, summary)
	return summary, nil
}

func failedCountCheckpoint(opts BackfillOptions, video store.SourceVideoCandidate, result Result, err error) store.BackfillCheckpoint {
	return store.BackfillCheckpoint{
		SourceSystem:  opts.SourceSystem,
		Mode:          store.BackfillModeFull,
		VideoID:       video.ID,
		SourceVideoID: video.SourceID,
		Status:        store.BackfillFailed,
		PagesFetched:  result.PagesFetched,
		Discovered:    result.Discovered,
		Inserted:      result.Inserted,
		Updated:       result.Updated,
		Skipped:       result.Skipped,
		ErrorMessage:  err.Error(),
		StartedAt:     opts.Now,
		CompletedAt:   opts.Now,
	}
}

// acceptStoredTotals allows stored rows to exceed the pager total.
// OLEHDTV's pager counts parents. Replies are stored as their own rows,
// so 311 parents and 50 replies (361 rows) is a successful import.
func acceptStoredTotals(pagerTotal, parents, replies int) error {
	if pagerTotal < 0 || parents < 0 || replies < 0 {
		return fmt.Errorf("unexpected comment totals")
	}
	return nil
}

func validateBackfillLimit(limit int) error {
	if limit <= 0 {
		return fmt.Errorf("missing --limit; no videos were selected")
	}
	if limit > MaxBackfillLimit {
		return fmt.Errorf("--limit %d exceeds %d", limit, MaxBackfillLimit)
	}
	return nil
}

type catalogCounts struct {
	Completed int
	Failed    int
	Pending   int
}

func selectBackfillVideos(videos []store.SourceVideoCandidate, checks []store.BackfillCheckpoint, sourceSystem string, limit int, retryFailed bool) ([]store.SourceVideoCandidate, *store.BackfillCheckpoint, catalogCounts) {
	byVideo := map[uint64]store.BackfillCheckpoint{}
	for _, row := range checks {
		if row.Mode != "" && row.Mode != store.BackfillModeFull {
			continue
		}
		byVideo[row.VideoID] = row
	}
	var counts catalogCounts
	var eligible []store.SourceVideoCandidate
	var blocked *store.BackfillCheckpoint
	for _, video := range videos {
		if video.SourceSystem != sourceSystem || strings.TrimSpace(video.SourceID) == "" || !video.Active || video.ID == 0 {
			continue
		}
		row, ok := byVideo[video.ID]
		switch {
		case ok && row.Status == store.BackfillCompleted:
			counts.Completed++
		case ok && row.Status == store.BackfillFailed:
			counts.Failed++
			if blocked == nil || video.ID < blocked.VideoID {
				copyRow := row
				if copyRow.SourceVideoID == "" {
					copyRow.SourceVideoID = video.SourceID
				}
				blocked = &copyRow
			}
			if retryFailed {
				eligible = append(eligible, video)
			}
		default:
			counts.Pending++
			eligible = append(eligible, video)
		}
	}
	for _, row := range checks {
		if row.Status != store.BackfillFailed {
			continue
		}
		if _, active := activeByID(videos, row.VideoID, sourceSystem); active {
			continue
		}
		if blocked == nil || row.VideoID < blocked.VideoID {
			copyRow := row
			blocked = &copyRow
		}
		counts.Failed++
	}
	if blocked != nil && !retryFailed {
		return nil, blocked, counts
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	if len(eligible) > limit {
		eligible = eligible[:limit]
	}
	return eligible, nil, counts
}

func activeByID(videos []store.SourceVideoCandidate, id uint64, sourceSystem string) (store.SourceVideoCandidate, bool) {
	for _, video := range videos {
		if video.ID == id && video.SourceSystem == sourceSystem && video.Active && strings.TrimSpace(video.SourceID) != "" {
			return video, true
		}
	}
	return store.SourceVideoCandidate{}, false
}

func printBackfillSummary(out io.Writer, summary BackfillSummary) {
	fmt.Fprintf(out, "selected=%d\ncompleted=%d\nfailed=%d\npages_fetched=%d\ncomments_discovered=%d\ninserted=%d\nupdated=%d\nskipped=%d\nelapsed=%s\n",
		summary.Selected, summary.Completed, summary.Failed, summary.Pages, summary.Discovered, summary.Inserted, summary.Updated, summary.Skipped, summary.Elapsed.Round(time.Millisecond))
}
