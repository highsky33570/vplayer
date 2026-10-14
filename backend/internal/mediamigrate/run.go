package mediamigrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/tycdn/vplayer/internal/store"
)

// BatchStore is the DB surface for locked media-migration runs (mockable).
type BatchStore interface {
	AcquireAdvisoryLock(ctx context.Context, name string) (*store.AdvisoryHold, error)
	ListMigratableEpisodes(limit int, episodeID uint64) ([]store.MigratableEpisode, error)
	GetMigratableEpisodeByID(id uint64) (*store.MigratableEpisode, error)
	StartOperationalRun(in store.StartOperationalRunInput) (uint64, error)
	FinishOperationalRun(in store.FinishOperationalRunInput) error
	RejectOperationalRun(in store.RejectOperationalRunInput) (uint64, error)
}

// RunOptions controls one migratemedia worker pass.
type RunOptions struct {
	DryRun    bool
	Execute   bool
	Limit     int
	EpisodeID uint64
}

// RunSummary is the machine-readable worker report (also drives CLI exit codes).
type RunSummary struct {
	DryRun          bool   `json:"dry_run"`
	Execute         bool   `json:"execute"`
	Limit           int    `json:"limit"`
	EpisodeID       uint64 `json:"episode_id,omitempty"`
	Selected        int    `json:"selected"`
	Planned         int    `json:"planned,omitempty"`
	Migrated        int    `json:"migrated"`
	Skipped         int    `json:"skipped"`
	AlreadyMigrated int    `json:"already_migrated"`
	Failed          int    `json:"failed"`
	LockAcquired    bool   `json:"lock_acquired"`
	Error           string `json:"error,omitempty"`
}

// BatchRunner acquires the media-migration advisory lock and audits execute runs.
type BatchRunner struct {
	Store             BatchStore
	Migrator          *Migrator
	Log               *slog.Logger
	LockName          string
	TriggerType       string // cli|timer|admin; empty defaults to cli
	RequestedByUserID uint64
}

// NewBatchRunner wires defaults (LockName = store.MediaMigrateLockName).
func NewBatchRunner(st BatchStore, mig *Migrator) *BatchRunner {
	return &BatchRunner{
		Store:       st,
		Migrator:    mig,
		Log:         slog.Default().With("component", "media-migrate"),
		LockName:    store.MediaMigrateLockName,
		TriggerType: store.TriggerCLI,
	}
}

// Run acquires the advisory lock, optionally audits operational_runs, then
// inspects (dry-run) or migrates (execute) episodes sequentially.
func (r *BatchRunner) Run(ctx context.Context, opts RunOptions) (RunSummary, error) {
	sum := RunSummary{
		DryRun:    opts.DryRun,
		Execute:   opts.Execute,
		Limit:     opts.Limit,
		EpisodeID: opts.EpisodeID,
	}
	if r.Store == nil || r.Migrator == nil {
		sum.Error = "store and migrator are required"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if opts.DryRun == opts.Execute {
		sum.Error = "exactly one of dry-run or execute required"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if opts.EpisodeID == 0 && opts.Limit <= 0 {
		sum.Error = "--limit must be > 0"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if opts.Limit <= 0 {
		opts.Limit = 1
	}

	trigger := r.TriggerType
	if trigger == "" {
		trigger = store.TriggerCLI
	}
	trigger, err := store.ValidateTriggerType(trigger)
	if err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	if err := store.ValidateTriggerRequester(trigger, r.RequestedByUserID); err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	reqBy := store.RequestedByPtr(r.RequestedByUserID)
	lockName := r.LockName
	if lockName == "" {
		lockName = store.MediaMigrateLockName
	}
	log := r.Log
	if log == nil {
		log = slog.Default()
	}

	hold, err := r.Store.AcquireAdvisoryLock(ctx, lockName)
	if err != nil {
		if errors.Is(err, store.ErrOperationLocked) {
			sum.Error = "another media migration worker is already running"
			if opts.Execute {
				if _, rerr := r.Store.RejectOperationalRun(store.RejectOperationalRunInput{
					JobType:           store.JobMediaMigration,
					TriggerType:       trigger,
					RequestedByUserID: reqBy,
					ErrorCode:         store.ErrCodeLockBusy,
				}); rerr != nil {
					log.Warn("reject operational run failed", "err", rerr)
				}
			}
			return sum, fmt.Errorf("%w: %s", store.ErrOperationLocked, sum.Error)
		}
		sum.Error = "advisory lock error"
		log.Error("advisory lock acquire failed", "err", err)
		return sum, err
	}
	sum.LockAcquired = true
	defer func() { _ = hold.Release() }()

	var opID uint64
	if opts.Execute {
		opID, err = r.Store.StartOperationalRun(store.StartOperationalRunInput{
			JobType:           store.JobMediaMigration,
			TriggerType:       trigger,
			RequestedByUserID: reqBy,
		})
		if err != nil {
			log.Warn("start operational run failed", "err", err)
			opID = 0
		}
	}

	finishOp := func(status, errCode string, hasErr bool) {
		if !opts.Execute || opID == 0 {
			return
		}
		skipped := sum.Skipped + sum.AlreadyMigrated
		in := store.FinishOperationalRunInput{
			ID:             opID,
			Status:         status,
			SelectedCount:  store.IntPtr(sum.Selected),
			SucceededCount: store.IntPtr(sum.Migrated),
			SkippedCount:   store.IntPtr(skipped),
			FailedCount:    store.IntPtr(sum.Failed),
			ErrorCode:      errCode,
			HasError:       hasErr,
		}
		if ferr := r.Store.FinishOperationalRun(in); ferr != nil {
			log.Warn("finish operational run failed", "err", ferr)
		}
	}

	eps, err := r.loadEpisodes(opts)
	if err != nil {
		sum.Failed++
		sum.Error = "list episodes: " + err.Error()
		code := store.ErrCodeDatabaseError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = store.ErrCodeTimeout
		}
		finishOp(store.OpStatusFailed, code, true)
		return sum, err
	}
	sum.Selected = len(eps)
	if len(eps) == 0 {
		finishOp(store.OpStatusSucceeded, "", false)
		return sum, nil
	}

	for _, ep := range eps {
		if err := ctx.Err(); err != nil {
			sum.Failed++
			sum.Error = err.Error()
			code := store.ErrCodeInternalError
			if errors.Is(err, context.DeadlineExceeded) {
				code = store.ErrCodeTimeout
			}
			finishOp(store.OpStatusFailed, code, true)
			return sum, err
		}
		if opts.DryRun {
			plan, ierr := r.Migrator.InspectSource(ctx, ep)
			if ierr != nil {
				sum.Failed++
				continue
			}
			switch {
			case plan.AlreadyMigrated:
				sum.AlreadyMigrated++
			case plan.SkipReason != "":
				sum.Skipped++
			default:
				sum.Planned++
			}
			continue
		}

		res := r.Migrator.MigrateEpisode(ctx, ep)
		switch {
		case res.AlreadyMigrated:
			sum.AlreadyMigrated++
		case res.Skipped && res.Error == "":
			sum.Skipped++
		case res.Error != "":
			sum.Failed++
		default:
			sum.Migrated++
		}
	}

	if opts.Execute {
		switch {
		case sum.Failed == 0:
			finishOp(store.OpStatusSucceeded, "", false)
		default:
			finishOp(store.OpStatusFailed, store.ErrCodePartialFailure, true)
			if sum.Error == "" {
				sum.Error = fmt.Sprintf("%d migration failures", sum.Failed)
			}
			return sum, fmt.Errorf("%s", sum.Error)
		}
	}
	return sum, nil
}

func (r *BatchRunner) loadEpisodes(opts RunOptions) ([]store.MigratableEpisode, error) {
	if opts.EpisodeID > 0 {
		ep, err := r.Store.GetMigratableEpisodeByID(opts.EpisodeID)
		if err != nil {
			return nil, err
		}
		if ep == nil {
			return nil, fmt.Errorf("episode %d not found", opts.EpisodeID)
		}
		return []store.MigratableEpisode{*ep}, nil
	}
	return r.Store.ListMigratableEpisodes(opts.Limit, 0)
}

// Ensure *store.MySQL satisfies BatchStore.
var _ BatchStore = (*store.MySQL)(nil)
