package sourcecomments

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv/commenthtml"
	"github.com/tycdn/vplayer/internal/store"
)

func TestDrainRejectsMissingOrHugeCeilingBeforeRequests(t *testing.T) {
	fetcher := &scriptPages{fail: errors.New("network")}
	catalog := &fakeCatalog{}
	for _, max := range []int{0, -1, MaxDrainVideos + 1} {
		if _, err := RunDrain(context.Background(), store.NewMemorySourceComments(), catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", MaxVideos: max}); err == nil {
			t.Fatalf("max %d accepted", max)
		}
	}
	if fetcher.calls != nil || catalog.saves != 0 || catalog.pendingLimits != nil {
		t.Fatal("rejected drain touched the source or the catalog")
	}
}

func TestDrainDryRunMakesNoWritesAndTruncatesPreview(t *testing.T) {
	videos := make([]store.SourceVideoCandidate, 12)
	for i := range videos {
		videos[i] = store.SourceVideoCandidate{ID: uint64(i + 1), SourceSystem: "olehdtv", SourceID: "s", Title: "t", Active: true}
	}
	catalog := &fakeCatalog{videos: videos, checks: []store.BackfillCheckpoint{{
		SourceSystem: "olehdtv", Mode: store.BackfillModeFull, VideoID: 1, SourceVideoID: "s", Status: store.BackfillCompleted,
	}}}
	fetcher := &scriptPages{fail: errors.New("network")}
	var out bytes.Buffer
	summary, err := RunDrain(context.Background(), store.NewMemorySourceComments(), catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10000, DryRun: true, Out: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != nil || catalog.saves != 0 {
		t.Fatal("dry-run performed work")
	}
	text := out.String()
	if !strings.Contains(text, "catalog_completed=1") || !strings.Contains(text, "could_process=11") || !strings.Contains(text, "preview_truncated=1") {
		t.Fatalf("output:\n%s", text)
	}
	if strings.Count(text, "video_id=") != DrainDryRunPreview {
		t.Fatalf("preview lines=%d\n%s", strings.Count(text, "video_id="), text)
	}
	if summary.Selected != 11 {
		t.Fatalf("summary %+v", summary)
	}
	if len(catalog.pendingLimits) != 1 || catalog.pendingLimits[0] != DrainDryRunPreview+1 {
		t.Fatalf("dry-run page sizes %v", catalog.pendingLimits)
	}
}

func TestDrainSkipsCompletedAndRollsBatches(t *testing.T) {
	comments := store.NewMemorySourceComments()
	native := store.NewMemoryComments()
	native.Seed(store.Comment{VideoID: 1, UserID: 1, Body: "native", Status: "visible"})
	videos := make([]store.SourceVideoCandidate, 5)
	body := map[string]map[int]string{}
	statuses := map[uint64]string{}
	for i := range videos {
		id := uint64(i + 1)
		videos[i] = store.SourceVideoCandidate{ID: id, SourceSystem: "olehdtv", SourceID: string(rune('a' + i)), Title: "v", Active: true}
		body[videos[i].SourceID] = map[int]string{1: parentPage("1", "body", 1, 1)}
		statuses[id] = "ready"
	}
	videos[0].SourceID = "a"
	catalog := &fakeCatalog{
		comments: comments,
		videos:   videos,
		statuses: statuses,
		checks: []store.BackfillCheckpoint{{
			SourceSystem: "olehdtv", Mode: store.BackfillModeFull, VideoID: 1, SourceVideoID: "a", Status: store.BackfillCompleted,
		}},
	}
	fetcher := &scriptPages{body: body}
	var out bytes.Buffer
	summary, err := RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10, BatchSize: 2, Now: time.Unix(10, 0).UTC(), Out: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fetcher.calls, ",") != "b:1,c:1,d:1,e:1" {
		t.Fatalf("calls=%v", fetcher.calls)
	}
	if summary.Completed != 4 || catalog.statusOf(1) != store.BackfillCompleted {
		t.Fatalf("summary %+v first=%s", summary, catalog.statusOf(1))
	}
	if len(catalog.pendingLimits) < 2 || catalog.pendingLimits[0] != 2 || catalog.pendingLimits[1] != 2 {
		t.Fatalf("batches %v after %v", catalog.pendingLimits, catalog.pendingAfter)
	}
	if !strings.Contains(out.String(), "BACKFILL DRAIN COMPLETE") || !strings.Contains(out.String(), "completed_this_run=4") {
		t.Fatalf("output:\n%s", out.String())
	}
	if native.Snapshot()[0].Body != "native" {
		t.Fatal("native comments changed")
	}
	for id, status := range statuses {
		if status != "ready" {
			t.Fatalf("catalog status %d=%s", id, status)
		}
	}
}

func TestDrainCeilingStopsWithoutMarkingTheRest(t *testing.T) {
	comments := store.NewMemorySourceComments()
	videos, body := drainVideos(4)
	catalog := &fakeCatalog{comments: comments, videos: videos}
	fetcher := &scriptPages{body: body}
	var out bytes.Buffer
	summary, err := RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 3, BatchSize: 2, Now: time.Unix(1, 0).UTC(), Out: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Completed != 3 || strings.Join(fetcher.calls, ",") != "1:1,2:1,3:1" {
		t.Fatalf("summary %+v calls=%v", summary, fetcher.calls)
	}
	if _, ok := catalog.checkpoint(4); ok {
		t.Fatal("video past the ceiling was checkpointed")
	}
	if !strings.Contains(out.String(), "BACKFILL DRAIN PAUSED: safety ceiling reached") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestDrainStopsOnFirstFailureAndBlocksUntilRetry(t *testing.T) {
	comments := store.NewMemorySourceComments()
	videos, body := drainVideos(3)
	body["1"] = map[int]string{1: parentPage("11", "one", 2, 2), 2: parentPage("12", "two", 2, 2)}
	catalog := &fakeCatalog{comments: comments, videos: videos, statuses: map[uint64]string{1: "ready", 2: "ready", 3: "ready"}}
	fetcher := &scriptPages{body: body, errAt: "2:1"}
	summary, err := RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10, BatchSize: 10, Now: time.Unix(2, 0).UTC(),
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	if strings.Join(fetcher.calls, ",") != "1:1,1:2,2:1" {
		t.Fatalf("calls=%v", fetcher.calls)
	}
	if summary.Completed != 1 || summary.Failed != 1 || catalog.statusOf(2) != store.BackfillFailed {
		t.Fatalf("summary %+v status=%s", summary, catalog.statusOf(2))
	}
	if _, ok := catalog.checkpoint(3); ok {
		t.Fatal("untouched video was checkpointed")
	}
	fetcher.calls = nil
	if _, err := RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", MaxVideos: 10}); !errors.Is(err, ErrFailedCheckpoint) {
		t.Fatalf("second drain err=%v", err)
	}
	if fetcher.calls != nil || strings.Contains(catalog.statusOf(3), store.BackfillFailed) {
		t.Fatal("blocked drain continued")
	}

	fetcher.errAt = "2:1"
	if _, err := RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10, RetryFailed: true, Now: time.Unix(3, 0).UTC(),
	}); err == nil {
		t.Fatal("retry should fail again")
	}
	if strings.Join(fetcher.calls, ",") != "2:1" {
		t.Fatalf("failed retry calls=%v", fetcher.calls)
	}
	if _, ok := catalog.checkpoint(3); ok {
		t.Fatal("pending video ran after a failed retry")
	}

	fetcher.calls = nil
	fetcher.errAt = ""
	summary, err = RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10, RetryFailed: true, Now: time.Unix(4, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fetcher.calls, ",") != "2:1,3:1" {
		t.Fatalf("successful retry calls=%v", fetcher.calls)
	}
	if summary.Completed != 2 || catalog.statusOf(1) != store.BackfillCompleted || catalog.statusOf(2) != store.BackfillCompleted {
		t.Fatalf("summary %+v statuses %s %s", summary, catalog.statusOf(1), catalog.statusOf(2))
	}
}

func TestDrainInterruptKeepsCompletedAndDoesNotFailUntouched(t *testing.T) {
	comments := store.NewMemorySourceComments()
	videos, body := drainVideos(3)
	catalog := &fakeCatalog{comments: comments, videos: videos}
	ctx, cancel := context.WithCancel(context.Background())
	fetcher := &scriptPages{body: body, cancel: cancel, cancelOn: "2:1"}
	_, err := RunDrain(ctx, comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10, BatchSize: 10, Now: time.Unix(8, 0).UTC(),
	})
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err=%v", err)
	}
	if catalog.statusOf(1) != store.BackfillCompleted {
		t.Fatalf("completed video became %s", catalog.statusOf(1))
	}
	if catalog.statusOf(2) == store.BackfillFailed {
		t.Fatal("interrupted video was marked failed")
	}
	if _, ok := catalog.checkpoint(3); ok {
		t.Fatal("untouched video was marked")
	}
	if strings.Join(fetcher.calls, ",") != "1:1,2:1" {
		t.Fatalf("calls=%v", fetcher.calls)
	}

	fetcher.calls = nil
	fetcher.cancel = nil
	summary, err := RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10, BatchSize: 10, Now: time.Unix(9, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fetcher.calls, ",") != "2:1,3:1" {
		t.Fatalf("resume calls=%v", fetcher.calls)
	}
	if summary.Completed != 2 || catalog.statusOf(1) != store.BackfillCompleted {
		t.Fatalf("summary %+v", summary)
	}
}

func TestDrainZeroCommentAndExistingImport(t *testing.T) {
	comments := store.NewMemorySourceComments()
	now := time.Unix(50, 0).UTC()
	if _, _, _, err := comments.UpsertSourceComments(9863, "olehdtv", []store.SourceCommentInput{{
		SourceCommentID: "413210", SourceVideoID: "82861", AuthorName: "游客", Body: "already", SourceCreatedRaw: "2026-10-19 01:56:45",
	}}, now); err != nil {
		t.Fatal(err)
	}
	catalog := &fakeCatalog{comments: comments, videos: []store.SourceVideoCandidate{
		{ID: 4, SourceSystem: "olehdtv", SourceID: "40", Title: "empty", Active: true},
		{ID: 9863, SourceSystem: "olehdtv", SourceID: "82861", Title: "功夫女足", Active: true},
	}}
	fetcher := &scriptPages{body: map[string]map[int]string{
		"40":    {1: ""},
		"82861": {1: parentPage("413210", "already", 1, 1)},
	}}
	summary, err := RunDrain(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", MaxVideos: 10, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	empty, _ := catalog.checkpoint(4)
	if empty.Status != store.BackfillCompleted || empty.Discovered != 0 || empty.Parents != 0 {
		t.Fatalf("zero checkpoint %+v", empty)
	}
	if len(comments.Snapshot()) != 1 || summary.Inserted != 0 || summary.Skipped != 1 {
		t.Fatalf("rows=%d summary %+v", len(comments.Snapshot()), summary)
	}
	raw, err := commenthtml.ListURL("https://www.olehdtv.com", "82861", 1)
	if err != nil || strings.Contains(strings.ToLower(raw), "savedata") || strings.Contains(strings.ToLower(raw), "gbook") {
		t.Fatalf("url %s err %v", raw, err)
	}
}

func drainVideos(n int) ([]store.SourceVideoCandidate, map[string]map[int]string) {
	videos := make([]store.SourceVideoCandidate, n)
	body := map[string]map[int]string{}
	for i := 0; i < n; i++ {
		id := uint64(i + 1)
		source := string(rune('0' + i + 1))
		videos[i] = store.SourceVideoCandidate{ID: id, SourceSystem: "olehdtv", SourceID: source, Title: "v", Active: true}
		body[source] = map[int]string{1: parentPage("9", "x", 1, 1)}
	}
	return videos, body
}
