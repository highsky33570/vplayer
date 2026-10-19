package sourcecomments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv/commenthtml"
	"github.com/tycdn/vplayer/internal/store"
)

func TestAcceptStoredTotalsAllowsRepliesBeyondPager(t *testing.T) {
	if err := acceptStoredTotals(311, 311, 50); err != nil {
		t.Fatalf("311 parents + 50 replies must be valid: %v", err)
	}
	if 311+50 != 361 {
		t.Fatal("stored rows are parents plus replies")
	}
	if err := acceptStoredTotals(-1, 0, 0); err == nil {
		t.Fatal("negative totals accepted")
	}
}

func TestSelectBackfillVideosIsDeterministicAndBounded(t *testing.T) {
	videos := []store.SourceVideoCandidate{
		{ID: 30, SourceSystem: "olehdtv", SourceID: "300", Title: "c", Active: true},
		{ID: 10, SourceSystem: "olehdtv", SourceID: "100", Title: "a", Active: true},
		{ID: 20, SourceSystem: "other", SourceID: "200", Title: "b", Active: true},
		{ID: 15, SourceSystem: "olehdtv", SourceID: "150", Title: "off", Active: false},
		{ID: 16, SourceSystem: "olehdtv", SourceID: "", Title: "blank", Active: true},
		{ID: 12, SourceSystem: "olehdtv", SourceID: "120", Title: "done", Active: true},
	}
	checks := []store.BackfillCheckpoint{{
		VideoID: 12, SourceVideoID: "120", Status: store.BackfillCompleted, Mode: store.BackfillModeFull,
	}}
	selected, blocked, counts := selectBackfillVideos(videos, checks, "olehdtv", 2, false)
	if blocked != nil {
		t.Fatalf("blocked %+v", blocked)
	}
	if counts.Completed != 1 || counts.Pending != 2 {
		t.Fatalf("counts %+v", counts)
	}
	if len(selected) != 2 || selected[0].ID != 10 || selected[1].ID != 30 {
		t.Fatalf("selected %+v", selected)
	}
}

func TestBackfillConfirmationLimitAndDryRunMakeNoRequests(t *testing.T) {
	catalog := &fakeCatalog{}
	fetcher := &scriptPages{fail: errors.New("network")}
	if _, err := RunBackfill(context.Background(), store.NewMemorySourceComments(), catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 0}); err == nil {
		t.Fatal("missing limit accepted")
	}
	if fetcher.calls != nil || catalog.saves != 0 {
		t.Fatalf("limit failure called source or wrote checkpoints calls=%v saves=%d", fetcher.calls, catalog.saves)
	}
	var out bytes.Buffer
	summary, err := RunBackfill(context.Background(), store.NewMemorySourceComments(), &fakeCatalog{videos: []store.SourceVideoCandidate{
		{ID: 2, SourceSystem: "olehdtv", SourceID: "20", Title: "two", Active: true},
	}}, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 10, DryRun: true, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != nil || catalog.saves != 0 {
		t.Fatal("dry-run performed work")
	}
	if summary.Selected != 1 || !strings.Contains(out.String(), "dry_run selected=1") || !strings.Contains(out.String(), "video_id=2") {
		t.Fatalf("summary %+v\n%s", summary, out.String())
	}
}

func TestBackfillRunsOneVideoAtATimeAndStopsOnFirstFailure(t *testing.T) {
	comments := store.NewMemorySourceComments()
	native := store.NewMemoryComments()
	native.Seed(store.Comment{VideoID: 10, UserID: 1, Body: "native", Status: "visible"})
	catalog := &fakeCatalog{
		comments: comments,
		statuses: map[uint64]string{10: "ready", 20: "ready", 30: "ready"},
		videos: []store.SourceVideoCandidate{
			{ID: 20, SourceSystem: "olehdtv", SourceID: "200", Title: "second", Active: true},
			{ID: 10, SourceSystem: "olehdtv", SourceID: "100", Title: "first", Active: true},
			{ID: 30, SourceSystem: "olehdtv", SourceID: "300", Title: "third", Active: true},
		},
	}
	fetcher := &scriptPages{body: map[string]map[int]string{
		"100": {1: parentPage("1", "one", 2, 2), 2: parentPage("2", "two", 2, 2)},
		"200": {1: parentPage("3", "three", 1, 1)},
		"300": {1: parentPage("4", "four", 1, 1)},
	}, errAt: "200:1"}
	var out bytes.Buffer
	summary, err := RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{
		SourceSystem: "olehdtv", Limit: 10, Now: time.Unix(100, 0).UTC(), Out: &out,
	})
	if err == nil {
		t.Fatal("expected stop")
	}
	if strings.Join(fetcher.calls, ",") != "100:1,100:2,200:1" {
		t.Fatalf("calls=%v", fetcher.calls)
	}
	if summary.Completed != 1 || summary.Failed != 1 || summary.Selected != 3 {
		t.Fatalf("summary %+v", summary)
	}
	if status := catalog.statusOf(10); status != store.BackfillCompleted {
		t.Fatalf("first checkpoint %s", status)
	}
	if status := catalog.statusOf(20); status != store.BackfillFailed {
		t.Fatalf("failed checkpoint %s", status)
	}
	if _, ok := catalog.checkpoint(30); ok {
		t.Fatal("third video was checkpointed after the stop")
	}
	if native.Snapshot()[0].Body != "native" {
		t.Fatal("native comments changed")
	}
	for id, status := range catalog.statuses {
		if status != "ready" {
			t.Fatalf("catalog status for %d changed to %s", id, status)
		}
	}
	if !strings.Contains(out.String(), "selected=3") || !strings.Contains(out.String(), "failed=1") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestBackfillCheckpointsZeroCommentsAndSkipsCompleted(t *testing.T) {
	comments := store.NewMemorySourceComments()
	catalog := &fakeCatalog{
		comments: comments,
		videos: []store.SourceVideoCandidate{
			{ID: 1, SourceSystem: "olehdtv", SourceID: "11", Title: "empty", Active: true},
			{ID: 2, SourceSystem: "olehdtv", SourceID: "22", Title: "later", Active: true},
		},
		checks: []store.BackfillCheckpoint{{
			SourceSystem: "olehdtv", Mode: store.BackfillModeFull, VideoID: 1, SourceVideoID: "11", Status: store.BackfillCompleted,
		}},
	}
	fetcher := &scriptPages{body: map[string]map[int]string{
		"11": {1: ""},
		"22": {1: ""},
	}}
	// The completed video is pre-seeded, so this run should only see video 2.
	catalog.checks[0].Status = store.BackfillCompleted
	summary, err := RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 10, Now: time.Unix(5, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fetcher.calls, ",") != "22:1" {
		t.Fatalf("calls=%v", fetcher.calls)
	}
	if summary.Completed != 1 || summary.Discovered != 0 {
		t.Fatalf("summary %+v", summary)
	}
	row, ok := catalog.checkpoint(2)
	if !ok || row.Status != store.BackfillCompleted || row.Discovered != 0 || row.Parents != 0 || row.Replies != 0 {
		t.Fatalf("zero checkpoint %+v ok=%v", row, ok)
	}
	if row.StartedAt.IsZero() || row.CompletedAt.IsZero() {
		t.Fatal("timestamps not stored")
	}
}

func TestFailedCheckpointRequiresExplicitRetry(t *testing.T) {
	comments := store.NewMemorySourceComments()
	catalog := &fakeCatalog{
		comments: comments,
		videos: []store.SourceVideoCandidate{
			{ID: 4, SourceSystem: "olehdtv", SourceID: "40", Title: "bad", Active: true},
			{ID: 5, SourceSystem: "olehdtv", SourceID: "50", Title: "next", Active: true},
		},
		checks: []store.BackfillCheckpoint{{
			SourceSystem: "olehdtv", Mode: store.BackfillModeFull, VideoID: 4, SourceVideoID: "40", Status: store.BackfillFailed, ErrorMessage: "source unavailable",
		}},
	}
	fetcher := &scriptPages{body: map[string]map[int]string{
		"40": {1: parentPage("9", "again", 1, 1)},
		"50": {1: parentPage("8", "new", 1, 1)},
	}}
	if _, err := RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 10}); !errors.Is(err, ErrFailedCheckpoint) {
		t.Fatalf("err=%v", err)
	}
	if fetcher.calls != nil || catalog.saves != 0 {
		t.Fatalf("blocked run did work calls=%v saves=%d", fetcher.calls, catalog.saves)
	}
	summary, err := RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 10, RetryFailed: true, Now: time.Unix(9, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fetcher.calls, ",") != "40:1,50:1" {
		t.Fatalf("retry calls=%v", fetcher.calls)
	}
	if summary.Completed != 2 || catalog.statusOf(4) != store.BackfillCompleted {
		t.Fatalf("summary %+v status=%s", summary, catalog.statusOf(4))
	}
}

func TestBackfillDoesNotDuplicateExistingImport(t *testing.T) {
	comments := store.NewMemorySourceComments()
	now := time.Unix(50, 0).UTC()
	if _, _, _, err := comments.UpsertSourceComments(9863, "olehdtv", []store.SourceCommentInput{{
		SourceCommentID: "413210", SourceVideoID: "82861", AuthorName: "游客", Body: "already", SourceCreatedRaw: "2026-10-19 01:56:45",
	}}, now); err != nil {
		t.Fatal(err)
	}
	catalog := &fakeCatalog{comments: comments, videos: []store.SourceVideoCandidate{{
		ID: 9863, SourceSystem: "olehdtv", SourceID: "82861", Title: "功夫女足", Active: true,
	}}}
	fetcher := &scriptPages{body: map[string]map[int]string{
		"82861": {1: parentPage("413210", "already", 1, 1)},
	}}
	summary, err := RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 1, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(comments.Snapshot()) != 1 {
		t.Fatalf("rows=%d", len(comments.Snapshot()))
	}
	if summary.Inserted != 0 || summary.Skipped != 1 {
		t.Fatalf("summary %+v", summary)
	}
	for _, row := range comments.Snapshot() {
		if row.SourceCommentID == "413210" && row.Body != "already" {
			t.Fatal("existing row was replaced")
		}
	}
	if catalog.statusOf(9863) != store.BackfillCompleted {
		t.Fatal("imported video was not checkpointed")
	}
}

func TestBackfillPagerTotalDoesNotRejectReplies(t *testing.T) {
	comments := store.NewMemorySourceComments()
	catalog := &fakeCatalog{comments: comments, videos: []store.SourceVideoCandidate{{
		ID: 7, SourceSystem: "olehdtv", SourceID: "82861", Title: "movie", Active: true,
	}}}
	page := `<li class="comm_each"><strong class="text_line">游客</strong><div class="comm_content">parent</div><a class="comment_reply" data-id="10"></a><div class="comm_reply_child"><span class="text_line">游客 ／ 10:28</span><div class="comm_content">reply</div><a class="digg_link" data-id="11" data-type="up"><em class="digg_num">1</em></a><a class="digg_link" data-id="11" data-type="down"><em class="digg_num">0</em></a></div></li><script>$('.mac_total').html('311');</script><a title="尾页" onclick="MAC.Comment.Show('1')">尾页</a>`
	fetcher := &scriptPages{body: map[string]map[int]string{"82861": {1: page}}}
	summary, err := RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 1, Now: time.Unix(1, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	row, _ := catalog.checkpoint(7)
	if summary.Completed != 1 || row.Parents != 1 || row.Replies != 1 {
		t.Fatalf("summary %+v checkpoint %+v", summary, row)
	}
	var out bytes.Buffer
	fetcher.calls = nil
	_, err = RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 1, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != nil {
		t.Fatal("completed video was fetched again")
	}
}

func TestBackfillRequestsStayOnCommentRead(t *testing.T) {
	comments := store.NewMemorySourceComments()
	catalog := &fakeCatalog{comments: comments, videos: []store.SourceVideoCandidate{{
		ID: 3, SourceSystem: "olehdtv", SourceID: "82861", Title: "one", Active: true,
	}}}
	fetcher := &scriptPages{body: map[string]map[int]string{"82861": {1: ""}}}
	if _, err := RunBackfill(context.Background(), comments, catalog, fetcher, BackfillOptions{SourceSystem: "olehdtv", Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("calls=%v", fetcher.calls)
	}
	raw, err := commenthtml.ListURL("https://www.olehdtv.com", "82861", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "/index.php/comment/ajax.html") {
		t.Fatal(raw)
	}
	for _, banned := range []string{"savedata", "verify", "digg", "report", "gbook"} {
		if strings.Contains(strings.ToLower(raw), banned) {
			t.Fatalf("prohibited %s in %s", banned, raw)
		}
	}
}

type fakeCatalog struct {
	comments *store.MemorySourceComments
	videos   []store.SourceVideoCandidate
	checks   []store.BackfillCheckpoint
	statuses map[uint64]string
	saves    int
}

func (f *fakeCatalog) ListActiveSourceVideos(sourceSystem string) ([]store.SourceVideoCandidate, error) {
	var out []store.SourceVideoCandidate
	for _, video := range f.videos {
		if video.SourceSystem == sourceSystem {
			out = append(out, video)
		}
	}
	return out, nil
}

func (f *fakeCatalog) ListBackfillCheckpoints(sourceSystem, mode string) ([]store.BackfillCheckpoint, error) {
	var out []store.BackfillCheckpoint
	for _, row := range f.checks {
		if row.SourceSystem == sourceSystem && row.Mode == mode {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeCatalog) SaveBackfillCheckpoint(row store.BackfillCheckpoint, now time.Time) error {
	f.saves++
	row.SourceSystem = strings.TrimSpace(row.SourceSystem)
	if row.StartedAt.IsZero() {
		row.StartedAt = now
	}
	for i := range f.checks {
		if f.checks[i].VideoID == row.VideoID && f.checks[i].Mode == row.Mode {
			f.checks[i] = row
			return nil
		}
	}
	f.checks = append(f.checks, row)
	return nil
}

func (f *fakeCatalog) CountSourceCommentRows(videoID uint64, sourceSystem string) (int, int, error) {
	return f.comments.CountSourceCommentRows(videoID, sourceSystem)
}

func (f *fakeCatalog) checkpoint(videoID uint64) (store.BackfillCheckpoint, bool) {
	for _, row := range f.checks {
		if row.VideoID == videoID {
			return row, true
		}
	}
	return store.BackfillCheckpoint{}, false
}

func (f *fakeCatalog) statusOf(videoID uint64) string {
	row, ok := f.checkpoint(videoID)
	if !ok {
		return ""
	}
	return row.Status
}

type scriptPages struct {
	body  map[string]map[int]string
	calls []string
	errAt string
	fail  error
}

func (s *scriptPages) FetchCommentPage(_ context.Context, sourceVideoID string, page int) (string, error) {
	if s.fail != nil {
		return "", s.fail
	}
	key := fmt.Sprintf("%s:%d", sourceVideoID, page)
	s.calls = append(s.calls, key)
	if s.errAt == key {
		return "", errors.New("source unavailable")
	}
	pages := s.body[sourceVideoID]
	body, ok := pages[page]
	if !ok {
		return "", errors.New("missing page")
	}
	return body, nil
}

func parentPage(id, body string, total, last int) string {
	return `<li class="comm_each"><strong class="text_line">游客</strong><span class="part_tips">2026-10-19 01:56:45</span><div class="comm_content">` + body + `</div><a class="comment_reply" data-id="` + id + `"></a></li>` +
		fmt.Sprintf(`<script>$('.mac_total').html('%d');</script><a title="尾页" onclick="MAC.Comment.Show('%d')">尾页</a>`, total, last)
}
