package sourcecomments

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

type fakePages struct {
	pages map[int]string
	calls []int
	errOn int
}

func (f *fakePages) FetchCommentPage(_ context.Context, _ string, page int) (string, error) {
	f.calls = append(f.calls, page)
	if f.errOn == page {
		return "", errors.New("source unavailable")
	}
	body, ok := f.pages[page]
	if !ok {
		return "", errors.New("missing page")
	}
	return body, nil
}

func commentPage(id, body string, last int) string {
	return `<li class="comm_each"><strong class="text_line">游客</strong><span class="part_tips">2026-10-19 01:56:45</span><div class="comm_content">` + body + `</div><a class="comment_reply" data-id="` + id + `"></a></li>` +
		`<span class="num">1/` + strconv.Itoa(last) + `</span><a title="尾页" onclick="MAC.Comment.Show('` + strconv.Itoa(last) + `')">尾页</a><script>$('.mac_total').html('` + strconv.Itoa(last) + `');</script>`
}

func TestFullSyncAndIncrementalReplyGap(t *testing.T) {
	repo := store.NewMemorySourceComments()
	native := store.NewMemoryComments()
	native.Seed(store.Comment{VideoID: 7, UserID: 1, Body: "stay", Status: "visible", CreatedAt: time.Unix(1, 0)})
	status := "ready"
	now := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	pages := &fakePages{pages: map[int]string{
		1: commentPage("100", "new", 3),
		2: commentPage("90", "old", 3),
		3: `<li class="comm_each"><div class="comm_content">older</div><a class="comment_reply" data-id="80"></a><div class="comm_reply_child"><span class="text_line">游客 ／ 10:28</span><div class="comm_content">late reply</div><a class="digg_link" data-id="81" data-type="up"><em class="digg_num">2</em></a><a class="digg_link" data-id="81" data-type="down"><em class="digg_num">0</em></a></div></li>` +
			`<a title="尾页" onclick="MAC.Comment.Show('3')">尾页</a><script>$('.mac_total').html('4');</script>`,
	}}
	if _, _, _, err := repo.UpsertSourceComments(7, "olehdtv", []store.SourceCommentInput{{
		SourceCommentID: "90", SourceVideoID: "82861", AuthorName: "游客", Body: "old", SourceCreatedRaw: "2026-10-19 01:56:45",
	}}, now); err != nil {
		t.Fatal(err)
	}
	inc := &fakePages{pages: pages.pages}
	result, err := Sync(context.Background(), repo, inc, 7, "olehdtv", "82861", ModeIncremental, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ints(inc.calls), ",") != "1,2" {
		t.Fatalf("incremental calls=%v", inc.calls)
	}
	if result.PagesFetched != 2 {
		t.Fatalf("pages=%d", result.PagesFetched)
	}
	for _, row := range repo.Snapshot() {
		if row.SourceCommentID == "81" {
			t.Fatal("incremental sync stored a reply from an unvisited older page")
		}
	}
	fullRepo := store.NewMemorySourceComments()
	full := &fakePages{pages: pages.pages}
	fullResult, err := Sync(context.Background(), fullRepo, full, 7, "olehdtv", "82861", ModeFull, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ints(full.calls), ",") != "1,2,3" || fullResult.PagesFetched != 3 {
		t.Fatalf("full calls=%v pages=%d", full.calls, fullResult.PagesFetched)
	}
	var reply store.SourceCommentInput
	found := false
	for _, row := range fullRepo.Snapshot() {
		if row.SourceCommentID == "81" {
			reply = row
			found = true
		}
	}
	if !found || reply.ParentSourceCommentID != "80" || reply.SourceCreatedRaw != "10:28" || reply.Body != "late reply" {
		t.Fatalf("full reply %+v found=%v", reply, found)
	}
	if status != "ready" {
		t.Fatalf("catalog status changed to %s", status)
	}
	if native.Snapshot()[0].Body != "stay" {
		t.Fatal("sync changed native comments")
	}
}

func TestSyncFailureLeavesNativeAndCatalogUntouched(t *testing.T) {
	repo := store.NewMemorySourceComments()
	native := store.NewMemoryComments()
	native.Seed(store.Comment{VideoID: 4, UserID: 2, Body: "native", Status: "visible"})
	status := "ready"
	fetcher := &fakePages{errOn: 1, pages: map[int]string{}}
	_, err := Sync(context.Background(), repo, fetcher, 4, "olehdtv", "85862", ModeFull, time.Now())
	if err == nil {
		t.Fatal("expected fetch error")
	}
	if len(repo.Snapshot()) != 0 {
		t.Fatalf("rows stored on failure: %+v", repo.Snapshot())
	}
	if native.Snapshot()[0].Body != "native" || status != "ready" {
		t.Fatalf("failure changed native=%+v status=%s", native.Snapshot(), status)
	}
}

func ints(v []int) []string {
	out := make([]string, len(v))
	for i, n := range v {
		out[i] = strconv.Itoa(n)
	}
	return out
}
