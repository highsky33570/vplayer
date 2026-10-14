package mediamigrate

import (
	"context"
	"errors"
	"sync"
)

// maxSegmentWorkers caps concurrent source GETs and R2 segment ops per episode.
const maxSegmentWorkers = 4

type segmentJob struct {
	index int
	seg   SegmentRef
	key   string
}

type segmentWorkResult struct {
	index         int
	key           string
	downloadedURL string
	ok            bool
}

// segmentFailGate records the first hard failure and cancels sibling workers.
type segmentFailGate struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	code   string
}

func (g *segmentFailGate) fail(code string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.code != "" {
		return
	}
	g.code = code
	if g.cancel != nil {
		g.cancel()
	}
}

func (g *segmentFailGate) codeOrEmpty() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.code
}

func (m *Migrator) segmentWorkerCount(nSeg int) int {
	w := maxSegmentWorkers
	if m.SegmentWorkers > 0 {
		w = m.SegmentWorkers
	}
	if nSeg < w {
		w = nSeg
	}
	if w < 1 {
		return 1
	}
	return w
}

// transferSegmentsBounded downloads, uploads, and verifies each segment with a
// fixed worker pool. Segment bodies are not retained after each worker finishes.
// On the first hard failure, workCtx is cancelled so remaining jobs stop promptly.
// DB success/failure writes are the caller's responsibility (coordinator only).
func (m *Migrator) transferSegmentsBounded(
	ctx context.Context,
	episodeID uint64,
	prefix string,
	segments []SegmentRef,
) (uploadedKeys []string, downloaded []string, errCode string) {
	n := len(segments)
	if n == 0 {
		return nil, nil, ""
	}

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	gate := &segmentFailGate{cancel: cancel}

	jobs := make(chan segmentJob)
	results := make(chan segmentWorkResult, n)
	workers := m.segmentWorkerCount(n)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- m.processOneSegment(workCtx, episodeID, job, gate)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for i, seg := range segments {
			job := segmentJob{
				index: i,
				seg:   seg,
				key:   prefix + seg.LocalName,
			}
			select {
			case <-workCtx.Done():
				return
			case jobs <- job:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	downloadedByIdx := make([]string, n)
	keysByIdx := make([]string, n)
	gotOK := make([]bool, n)

	for r := range results {
		if r.index >= 0 && r.index < n {
			downloadedByIdx[r.index] = r.downloadedURL
			if r.ok {
				gotOK[r.index] = true
				keysByIdx[r.index] = r.key
			}
		}
	}

	if code := gate.codeOrEmpty(); code != "" {
		return nil, collectNonEmpty(downloadedByIdx), code
	}
	if err := ctx.Err(); err != nil {
		// Parent deadline/cancel with no worker-recorded root cause.
		return nil, collectNonEmpty(downloadedByIdx), "segment_download_failed"
	}
	for i := 0; i < n; i++ {
		if !gotOK[i] || keysByIdx[i] == "" {
			return nil, collectNonEmpty(downloadedByIdx), "segment_upload_failed"
		}
	}

	uploadedKeys = make([]string, n)
	copy(uploadedKeys, keysByIdx)
	return uploadedKeys, collectNonEmpty(downloadedByIdx), ""
}

func (m *Migrator) processOneSegment(ctx context.Context, episodeID uint64, job segmentJob, gate *segmentFailGate) segmentWorkResult {
	out := segmentWorkResult{index: job.index, key: job.key}
	if ctx.Err() != nil {
		return out // cancelled follower; root cause already on gate or parent ctx
	}

	body, err := m.fetchBytesLabeled(ctx, job.seg.SourceURL, "segment", job.index, job.seg.LocalName)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return out
		}
		m.logger().Printf("episode=%d segment_download_failed index=%d name=%s src=%s",
			episodeID, job.index, job.seg.LocalName, RedactURL(job.seg.SourceURL))
		gate.fail("segment_download_failed")
		return out
	}
	out.downloadedURL = RedactURL(job.seg.SourceURL)

	if ctx.Err() != nil {
		return out
	}

	if _, err := m.ensureObject(ctx, episodeID, job.key, body, contentTypeFor(job.seg.LocalName)); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return out
		}
		m.logger().Printf("episode=%d segment_upload_failed key=%s", episodeID, job.key)
		gate.fail("segment_upload_failed")
		return out
	}

	expected := int64(len(body))
	body = nil // release segment body for GC before verify / return

	if err := m.verifyObjectSize(ctx, episodeID, job.key, expected); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return out
		}
		m.logger().Printf("episode=%d verification_failed key=%s phase=segments", episodeID, job.key)
		gate.fail("verification_failed")
		return out
	}
	out.ok = true
	return out
}

func collectNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
