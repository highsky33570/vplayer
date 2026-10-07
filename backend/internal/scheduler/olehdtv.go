package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/catalogsync"
	"github.com/tycdn/vplayer/internal/model"
)

// OleHdTvRunner periodically invokes OleHdTvSyncService.
type OleHdTvRunner struct {
	Service  *catalogsync.OleHdTvSyncService
	Interval time.Duration
	Log      *slog.Logger

	mu      sync.Mutex
	running bool
	stop    chan struct{}
}

func NewOleHdTvRunner(svc *catalogsync.OleHdTvSyncService, intervalMinutes int) *OleHdTvRunner {
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	return &OleHdTvRunner{
		Service:  svc,
		Interval: time.Duration(intervalMinutes) * time.Minute,
		Log:      slog.Default().With("component", "olehdtv-scheduler"),
		stop:     make(chan struct{}),
	}
}

func (r *OleHdTvRunner) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stop = make(chan struct{})
	r.mu.Unlock()

	go func() {
		r.Log.Info("scheduler started", "interval", r.Interval.String())
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-r.stop:
				r.Log.Info("scheduler stopped")
				return
			case <-timer.C:
				r.tick()
				timer.Reset(r.Interval)
			}
		}
	}()
}

func (r *OleHdTvRunner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return
	}
	close(r.stop)
	r.running = false
}

func (r *OleHdTvRunner) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	sum, err := r.Service.Run(ctx, "schedule")
	if err != nil {
		r.Log.Error("scheduled sync failed", "err", err)
		return
	}
	r.Log.Info("scheduled sync ok",
		"scanned", sum.Scanned,
		"created", sum.Created,
		"updated", sum.Updated,
		"unchanged", sum.Unchanged,
		"failed", sum.Failed,
	)
}

func (r *OleHdTvRunner) Trigger(ctx context.Context) (model.SyncSummary, error) {
	return r.Service.Run(ctx, "manual")
}
