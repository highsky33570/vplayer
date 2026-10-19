package sourcecomments

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

const MaintenanceBatchSize = 100

// MaintenanceOptions selects one incremental or full maintenance invocation.
// A completed historical backfill checkpoint does not satisfy this run.
type MaintenanceOptions struct {
	SourceSystem string
	Mode         string
	RetryFailed  bool
	StatusOnly   bool
	BatchSize    int
	Now          time.Time
	Out          io.Writer
	Locker       Locker
}

// MaintenanceSummary counts the current invocation, not the historical backfill.
type MaintenanceSummary struct {
	CycleID    uint64
	Mode       string
	Completed  int
	Failed     int
	Pages      int
	Discovered int
	Inserted   int
	Updated    int
	Skipped    int
	Elapsed    time.Duration
}

// MaintenanceCatalog stores cycle and per-video resume state.
// It must not read or write source_comment_backfill, video_comments, or catalog status.
type MaintenanceCatalog interface {
	LatestMaintenanceCycle(sourceSystem, mode string) (*store.MaintenanceCycle, error)
	CreateMaintenanceCycle(row store.MaintenanceCycle, now time.Time) (store.MaintenanceCycle, error)
	SaveMaintenanceCycleStatus(id uint64, status, errorMessage string, completedAt, now time.Time) error
	MaxEligibleVideoID(sourceSystem string) (uint64, error)
	ListPendingMaintenanceVideos(cycleID, maxVideoID, afterID uint64, sourceSystem string, limit int) ([]store.SourceVideoCandidate, error)
	ListFailedMaintenanceVideos(cycleID, maxVideoID uint64, sourceSystem string) ([]store.SourceVideoCandidate, error)
	FirstFailedMaintenance(cycleID uint64) (*store.MaintenanceCheckpoint, error)
	SaveMaintenanceCheckpoint(row store.MaintenanceCheckpoint, now time.Time) error
	CountMaintenanceCheckpointStatus(cycleID uint64) (completed, failed int, err error)
	CountRemainingMaintenance(cycleID, maxVideoID uint64, sourceSystem string) (int, error)
	CountVideosAboveBoundary(sourceSystem string, maxVideoID uint64) (int, error)
}

// ReportMaintenanceStatus prints the latest cycle without taking the lock or contacting OLEHDTV.
func ReportMaintenanceStatus(catalog MaintenanceCatalog, opts MaintenanceOptions) error {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	if _, err := maintenanceSyncMode(opts.Mode); err != nil {
		return err
	}
	if catalog == nil {
		return fmt.Errorf("comment maintenance is not configured")
	}
	cycle, err := catalog.LatestMaintenanceCycle(opts.SourceSystem, opts.Mode)
	if err != nil {
		return err
	}
	catalogMax, err := catalog.MaxEligibleVideoID(opts.SourceSystem)
	if err != nil {
		return err
	}
	if cycle == nil {
		above, err := catalog.CountVideosAboveBoundary(opts.SourceSystem, 0)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "cycle_id=0\nmode=%s\nstatus=none\nmax_video_id=0\ncatalog_max_video_id=%d\nvideos_completed=0\nvideos_failed=0\nfailed_video_id=0\nfailed_source_id=\nremaining_in_cycle=0\nvideos_above_boundary=%d\nstarted_at=\ncompleted_at=\n",
			opts.Mode, catalogMax, above)
		return nil
	}
	completed, failed, err := catalog.CountMaintenanceCheckpointStatus(cycle.ID)
	if err != nil {
		return err
	}
	remaining, err := catalog.CountRemainingMaintenance(cycle.ID, cycle.MaxVideoID, opts.SourceSystem)
	if err != nil {
		return err
	}
	above, err := catalog.CountVideosAboveBoundary(opts.SourceSystem, cycle.MaxVideoID)
	if err != nil {
		return err
	}
	blocked, err := catalog.FirstFailedMaintenance(cycle.ID)
	if err != nil {
		return err
	}
	var failedID uint64
	var failedSource string
	if blocked != nil {
		failedID = blocked.VideoID
		failedSource = blocked.SourceVideoID
	}
	fmt.Fprintf(out, "cycle_id=%d\nmode=%s\nstatus=%s\nmax_video_id=%d\ncatalog_max_video_id=%d\nvideos_completed=%d\nvideos_failed=%d\nfailed_video_id=%d\nfailed_source_id=%s\nremaining_in_cycle=%d\nvideos_above_boundary=%d\nstarted_at=%s\ncompleted_at=%s\n",
		cycle.ID, cycle.Mode, cycle.Status, cycle.MaxVideoID, catalogMax, completed, failed, failedID, failedSource, remaining, above, formatStamp(cycle.StartedAt), formatStamp(cycle.CompletedAt))
	return nil
}

// RunMaintenance walks one maintenance cycle, one video and one comment page at a time.
// An incomplete cycle is resumed. A completed cycle causes the next call to open a new cycle.
// Historical source_comment_backfill rows are not consulted.
func RunMaintenance(ctx context.Context, comments store.SourceCommentRepo, catalog MaintenanceCatalog, fetcher PageFetcher, opts MaintenanceOptions) (MaintenanceSummary, error) {
	started := time.Now()
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	var summary MaintenanceSummary
	mode, err := maintenanceSyncMode(opts.Mode)
	if err != nil {
		return summary, err
	}
	summary.Mode = opts.Mode
	if opts.StatusOnly {
		return summary, ReportMaintenanceStatus(catalog, opts)
	}
	if comments == nil || catalog == nil || fetcher == nil {
		return summary, fmt.Errorf("comment maintenance is not configured")
	}
	release, err := acquireCommentLock(ctx, opts.Locker)
	if err != nil {
		return summary, err
	}
	defer release()

	batch := opts.BatchSize
	if batch <= 0 {
		batch = MaintenanceBatchSize
	}
	cycle, blocked, err := openMaintenanceCycle(catalog, opts)
	if err != nil {
		return summary, err
	}
	summary.CycleID = cycle.ID
	if blocked != nil && !opts.RetryFailed {
		fmt.Fprintf(out, "failed checkpoint cycle_id=%d video_id=%d source_id=%s requires --retry-failed\n", cycle.ID, blocked.VideoID, blocked.SourceVideoID)
		return summary, ErrFailedCheckpoint
	}
	if opts.RetryFailed && blocked != nil {
		failedVideos, err := catalog.ListFailedMaintenanceVideos(cycle.ID, cycle.MaxVideoID, opts.SourceSystem)
		if err != nil {
			return summary, err
		}
		if len(failedVideos) == 0 {
			return summary, fmt.Errorf("failed checkpoint video_id=%d is not an active source video", blocked.VideoID)
		}
		_, interrupted, err := processMaintenanceVideos(ctx, comments, catalog, fetcher, opts, mode, cycle, out, &summary, failedVideos)
		if err != nil || interrupted {
			return summary, finishMaintenance(out, &summary, catalog, cycle, opts, started, interrupted, err)
		}
		if err := catalog.SaveMaintenanceCycleStatus(cycle.ID, store.MaintenanceRunning, "", time.Time{}, opts.Now); err != nil {
			return summary, err
		}
	}

	after := uint64(0)
	for {
		if err := ctx.Err(); err != nil {
			return summary, finishMaintenance(out, &summary, catalog, cycle, opts, started, true, nil)
		}
		page, err := catalog.ListPendingMaintenanceVideos(cycle.ID, cycle.MaxVideoID, after, opts.SourceSystem, batch)
		if err != nil {
			return summary, err
		}
		if len(page) == 0 {
			if err := catalog.SaveMaintenanceCycleStatus(cycle.ID, store.MaintenanceCycleCompleted, "", opts.Now, opts.Now); err != nil {
				return summary, err
			}
			if err := finishMaintenance(out, &summary, catalog, cycle, opts, started, false, nil); err != nil {
				return summary, err
			}
			fmt.Fprintln(out, "MAINTENANCE CYCLE COMPLETE")
			return summary, nil
		}
		_, interrupted, err := processMaintenanceVideos(ctx, comments, catalog, fetcher, opts, mode, cycle, out, &summary, page)
		after = page[len(page)-1].ID
		if err != nil || interrupted {
			return summary, finishMaintenance(out, &summary, catalog, cycle, opts, started, interrupted, err)
		}
	}
}

func openMaintenanceCycle(catalog MaintenanceCatalog, opts MaintenanceOptions) (store.MaintenanceCycle, *store.MaintenanceCheckpoint, error) {
	latest, err := catalog.LatestMaintenanceCycle(opts.SourceSystem, opts.Mode)
	if err != nil {
		return store.MaintenanceCycle{}, nil, err
	}
	if latest == nil || latest.Status == store.MaintenanceCycleCompleted {
		maxID, err := catalog.MaxEligibleVideoID(opts.SourceSystem)
		if err != nil {
			return store.MaintenanceCycle{}, nil, err
		}
		created, err := catalog.CreateMaintenanceCycle(store.MaintenanceCycle{
			SourceSystem: opts.SourceSystem,
			Mode:         opts.Mode,
			Status:       store.MaintenanceRunning,
			MaxVideoID:   maxID,
			StartedAt:    opts.Now,
		}, opts.Now)
		return created, nil, err
	}
	blocked, err := catalog.FirstFailedMaintenance(latest.ID)
	if err != nil {
		return store.MaintenanceCycle{}, nil, err
	}
	if latest.Status == store.MaintenanceCycleFailed && blocked == nil {
		if err := catalog.SaveMaintenanceCycleStatus(latest.ID, store.MaintenanceRunning, "", time.Time{}, opts.Now); err != nil {
			return store.MaintenanceCycle{}, nil, err
		}
		latest.Status = store.MaintenanceRunning
	}
	return *latest, blocked, nil
}

func processMaintenanceVideos(ctx context.Context, comments store.SourceCommentRepo, catalog MaintenanceCatalog, fetcher PageFetcher, opts MaintenanceOptions, mode Mode, cycle store.MaintenanceCycle, out io.Writer, summary *MaintenanceSummary, videos []store.SourceVideoCandidate) (stop, interrupted bool, err error) {
	for _, video := range videos {
		if err := ctx.Err(); err != nil {
			return false, true, nil
		}
		kind, syncErr := processMaintenanceVideo(ctx, comments, catalog, fetcher, opts, mode, cycle, out, summary, video)
		if kind == drainInterrupted {
			return false, true, nil
		}
		if kind == drainFailed {
			_ = catalog.SaveMaintenanceCycleStatus(cycle.ID, store.MaintenanceCycleFailed, syncErr.Error(), time.Time{}, opts.Now)
			return false, false, syncErr
		}
	}
	return false, false, nil
}

func processMaintenanceVideo(ctx context.Context, comments store.SourceCommentRepo, catalog MaintenanceCatalog, fetcher PageFetcher, opts MaintenanceOptions, mode Mode, cycle store.MaintenanceCycle, out io.Writer, summary *MaintenanceSummary, video store.SourceVideoCandidate) (drainStop, error) {
	if err := ctx.Err(); err != nil {
		return drainInterrupted, nil
	}
	if err := catalog.SaveMaintenanceCheckpoint(store.MaintenanceCheckpoint{
		CycleID:       cycle.ID,
		SourceSystem:  opts.SourceSystem,
		Mode:          opts.Mode,
		VideoID:       video.ID,
		SourceVideoID: video.SourceID,
		Status:        store.MaintenancePending,
		StartedAt:     opts.Now,
	}, opts.Now); err != nil {
		return drainFailed, err
	}
	if err := ctx.Err(); err != nil {
		return drainInterrupted, nil
	}
	n := summary.Completed + summary.Failed + 1
	result, syncErr := Sync(ctx, comments, fetcher, video.ID, opts.SourceSystem, video.SourceID, mode, opts.Now)
	if syncErr != nil {
		if ctx.Err() != nil || isInterrupt(syncErr) {
			return drainInterrupted, nil
		}
		message := syncErr.Error()
		if len(result.Errors) > 0 {
			message = result.Errors[0]
		}
		_ = catalog.SaveMaintenanceCheckpoint(store.MaintenanceCheckpoint{
			CycleID:       cycle.ID,
			SourceSystem:  opts.SourceSystem,
			Mode:          opts.Mode,
			VideoID:       video.ID,
			SourceVideoID: video.SourceID,
			Status:        store.MaintenanceVideoFailed,
			PagesFetched:  result.PagesFetched,
			Discovered:    result.Discovered,
			Inserted:      result.Inserted,
			Updated:       result.Updated,
			Skipped:       result.Skipped,
			ErrorMessage:  message,
			StartedAt:     opts.Now,
			CompletedAt:   opts.Now,
		}, opts.Now)
		summary.Failed++
		addMaintenanceResult(summary, result)
		fmt.Fprintf(out, "[%d] video_id=%d source_id=%s title=%q pages=%d discovered=%d inserted=%d updated=%d skipped=%d FAIL %s\n",
			n, video.ID, video.SourceID, video.Title, result.PagesFetched, result.Discovered, result.Inserted, result.Updated, result.Skipped, message)
		return drainFailed, syncErr
	}
	if err := catalog.SaveMaintenanceCheckpoint(store.MaintenanceCheckpoint{
		CycleID:       cycle.ID,
		SourceSystem:  opts.SourceSystem,
		Mode:          opts.Mode,
		VideoID:       video.ID,
		SourceVideoID: video.SourceID,
		Status:        store.MaintenanceVideoCompleted,
		PagesFetched:  result.PagesFetched,
		Discovered:    result.Discovered,
		Inserted:      result.Inserted,
		Updated:       result.Updated,
		Skipped:       result.Skipped,
		StartedAt:     opts.Now,
		CompletedAt:   opts.Now,
	}, opts.Now); err != nil {
		return drainFailed, err
	}
	summary.Completed++
	addMaintenanceResult(summary, result)
	fmt.Fprintf(out, "[%d] video_id=%d source_id=%s title=%q pages=%d discovered=%d inserted=%d updated=%d skipped=%d OK\n",
		n, video.ID, video.SourceID, video.Title, result.PagesFetched, result.Discovered, result.Inserted, result.Updated, result.Skipped)
	return drainOK, nil
}

func addMaintenanceResult(summary *MaintenanceSummary, result Result) {
	summary.Pages += result.PagesFetched
	summary.Discovered += result.Discovered
	summary.Inserted += result.Inserted
	summary.Updated += result.Updated
	summary.Skipped += result.Skipped
}

func finishMaintenance(out io.Writer, summary *MaintenanceSummary, catalog MaintenanceCatalog, cycle store.MaintenanceCycle, opts MaintenanceOptions, started time.Time, interrupted bool, cause error) error {
	summary.Elapsed = time.Since(started)
	remaining, err := catalog.CountRemainingMaintenance(cycle.ID, cycle.MaxVideoID, opts.SourceSystem)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "maintenance_summary:\n  cycle_id=%d\n  mode=%s\n  videos_processed=%d\n  videos_completed=%d\n  failed=%d\n  pages_fetched=%d\n  comments_discovered=%d\n  inserted=%d\n  updated=%d\n  skipped=%d\n  elapsed=%s\n  remaining_in_cycle=%d\n",
		summary.CycleID, summary.Mode, summary.Completed+summary.Failed, summary.Completed, summary.Failed, summary.Pages, summary.Discovered, summary.Inserted, summary.Updated, summary.Skipped, summary.Elapsed.Round(time.Millisecond), remaining)
	if interrupted {
		fmt.Fprintln(out, "interrupted")
		return ErrInterrupted
	}
	return cause
}

func maintenanceSyncMode(mode string) (Mode, error) {
	switch mode {
	case store.MaintenanceModeIncremental:
		return ModeIncremental, nil
	case store.MaintenanceModeFull:
		return ModeFull, nil
	case "":
		return 0, fmt.Errorf("missing --mode")
	default:
		return 0, fmt.Errorf("invalid --mode %q", mode)
	}
}

func formatStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}
