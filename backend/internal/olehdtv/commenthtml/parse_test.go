package commenthtml

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	p := filepath.Join(filepath.Dir(file), "testdata", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func findComment(page Page, id string) (SourceComment, bool) {
	for _, c := range page.Comments {
		if c.SourceCommentID == id {
			return c, true
		}
	}
	return SourceComment{}, false
}

func TestParseMovieHTMLAndJSONString(t *testing.T) {
	htmlPage, err := Parse(readFixture(t, "comment-ajax-82861.html"), "82861")
	if err != nil {
		t.Fatal(err)
	}
	jsonPage, err := Parse(readFixture(t, "comment-ajax-82861-xhr.body"), "82861")
	if err != nil {
		t.Fatal(err)
	}
	if htmlPage.Total != 311 || htmlPage.LastPage != 63 || htmlPage.CurrentPage != 1 {
		t.Fatalf("pager total=%d last=%d current=%d", htmlPage.Total, htmlPage.LastPage, htmlPage.CurrentPage)
	}
	if jsonPage.Total != htmlPage.Total || jsonPage.LastPage != htmlPage.LastPage {
		t.Fatalf("json pager %+v html %+v", jsonPage, htmlPage)
	}
	parents := 0
	var ids []string
	for _, c := range htmlPage.Comments {
		if c.ParentSourceCommentID != "" {
			t.Fatalf("movie page 1 reply %+v", c)
		}
		parents++
		ids = append(ids, c.SourceCommentID)
		if c.SourceVideoID != "82861" {
			t.Fatalf("video id %s", c.SourceVideoID)
		}
	}
	if parents != 5 {
		t.Fatalf("parents=%d ids=%v", parents, ids)
	}
	wantIDs := []string{"413210", "413209", "413196", "413181", "413174"}
	if strings.Join(ids, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("ids=%v", ids)
	}
	var jsonIDs []string
	for _, c := range jsonPage.Comments {
		jsonIDs = append(jsonIDs, c.SourceCommentID)
	}
	if strings.Join(jsonIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("json ids=%v", jsonIDs)
	}
	first, ok := findComment(htmlPage, "413210")
	if !ok {
		t.Fatal("missing 413210")
	}
	if first.AuthorName != "游客" {
		t.Fatalf("author=%q", first.AuthorName)
	}
	if first.AvatarPath != "/static/images/touxiang.png" {
		t.Fatalf("avatar=%q", first.AvatarPath)
	}
	if first.Body != "初三（21號）和雪野（20號）長相太神似，以為是同一個人分飾兩角 :P" {
		t.Fatalf("body=%q", first.Body)
	}
	if first.SourceCreatedRaw != "2026-10-19 01:56:45" {
		t.Fatalf("time=%q", first.SourceCreatedRaw)
	}
	if first.DiggUp != 63 || first.DiggDown != 0 {
		t.Fatalf("digg up=%d down=%d", first.DiggUp, first.DiggDown)
	}
	if strings.Contains(first.SourceCreatedRaw, "Z") || strings.Contains(first.SourceCreatedRaw, "UTC") {
		t.Fatalf("timestamp was rewritten: %q", first.SourceCreatedRaw)
	}
}

func TestParseVarietyReply(t *testing.T) {
	page, err := Parse(readFixture(t, "comment-ajax-83449.html"), "83449")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 122 || page.LastPage != 25 {
		t.Fatalf("pager total=%d last=%d", page.Total, page.LastPage)
	}
	parent, ok := findComment(page, "412878")
	if !ok {
		t.Fatal("missing parent 412878")
	}
	if parent.ParentSourceCommentID != "" {
		t.Fatalf("parent nested under %s", parent.ParentSourceCommentID)
	}
	reply, ok := findComment(page, "413117")
	if !ok {
		t.Fatal("missing reply 413117")
	}
	if reply.ParentSourceCommentID != "412878" {
		t.Fatalf("parent=%s", reply.ParentSourceCommentID)
	}
	if reply.AuthorName != "游客" {
		t.Fatalf("reply author=%q", reply.AuthorName)
	}
	if reply.SourceCreatedRaw != "10:28" {
		t.Fatalf("reply time=%q", reply.SourceCreatedRaw)
	}
	if strings.Contains(reply.SourceCreatedRaw, "-") {
		t.Fatal("reply timestamp invented a date")
	}
	if reply.AvatarPath != "" {
		t.Fatalf("reply avatar=%q", reply.AvatarPath)
	}
	if reply.DiggUp != 2 || reply.DiggDown != 0 {
		t.Fatalf("reply digg up=%d down=%d", reply.DiggUp, reply.DiggDown)
	}
	if reply.Body == "" || strings.Contains(reply.Body, "<") {
		t.Fatalf("reply body=%q", reply.Body)
	}
}

func TestParseEmptyMalformedAndSafety(t *testing.T) {
	empty, err := Parse("   ", "1")
	if err != nil || len(empty.Comments) != 0 || empty.Total != 0 || empty.LastPage != 0 {
		t.Fatalf("empty page=%+v err=%v", empty, err)
	}
	if _, err := Parse(`{"code":1,"data":"<div></div>"}`, "1"); !errors.Is(err, ErrUnexpectedPayload) {
		t.Fatalf("object err=%v", err)
	}
	if _, err := Parse(`"unterminated`, "1"); !errors.Is(err, ErrMalformedPayload) {
		t.Fatalf("malformed err=%v", err)
	}
	missingAuthor, err := Parse(`<ul><li class="comm_each"><div class="comm_cont"><div class="comm_content">still here</div><div class="gw_action"><a class="comment_reply" data-id="15"></a></div></div></li></ul>`, "9")
	if err != nil {
		t.Fatal(err)
	}
	if len(missingAuthor.Comments) != 1 || missingAuthor.Comments[0].AuthorName != "" || missingAuthor.Comments[0].Body != "still here" {
		t.Fatalf("missing author %+v", missingAuthor.Comments)
	}
	htmlBody, err := Parse(`<li class="comm_each"><strong class="text_line">A&amp;B</strong><div class="comm_content">hello<script>alert(1)</script><b>there</b></div><a class="comment_reply" data-id="9"></a><a class="digg_link" data-id="9" data-type="up"><em class="digg_num">4</em></a><a class="digg_link" data-id="9" data-type="down"><em class="digg_num">1</em></a></li>`, "9")
	if err != nil {
		t.Fatal(err)
	}
	if len(htmlBody.Comments) != 1 {
		t.Fatalf("comments=%d", len(htmlBody.Comments))
	}
	got := htmlBody.Comments[0]
	if got.AuthorName != "A&B" || got.Body != "hellothere" || strings.Contains(got.Body, "alert") || strings.Contains(got.Body, "script") {
		t.Fatalf("sanitized %+v", got)
	}
	if got.DiggUp != 4 || got.DiggDown != 1 {
		t.Fatalf("digg %+v", got)
	}
	dup, err := Parse(`<li class="comm_each"><div class="comm_content">one</div><a class="comment_reply" data-id="9"></a></li><li class="comm_each"><div class="comm_content">two</div><a class="comment_reply" data-id="9"></a></li>`, "9")
	if err != nil {
		t.Fatal(err)
	}
	if len(dup.Comments) != 1 || dup.Comments[0].Body != "one" {
		t.Fatalf("dup %+v", dup.Comments)
	}
	scriptOnly, err := Parse(`<li class="comm_each"><div class="comm_content"><script>alert(1)</script></div><a class="comment_reply" data-id="3"></a></li>`, "9")
	if err != nil {
		t.Fatal(err)
	}
	if len(scriptOnly.Comments) != 0 {
		t.Fatalf("script body kept %+v", scriptOnly.Comments)
	}
	badPager, err := Parse(`<li class="comm_each"><div class="comm_content">only</div><a class="comment_reply" data-id="4"></a></li><script>$('.mac_total').html('nope');</script><a title="尾页" onclick="MAC.Comment.Show('zz')">尾页</a>`, "9")
	if err != nil {
		t.Fatal(err)
	}
	if badPager.Total != 1 || badPager.LastPage != 1 || len(badPager.Comments) != 1 {
		t.Fatalf("bad pager %+v", badPager)
	}
	jsAvatar, err := Parse(`<li class="comm_each"><img class="comm_avat" src="javascript:alert(1)"><div class="comm_content">ok</div><a class="comment_reply" data-id="8"></a></li>`, "9")
	if err != nil {
		t.Fatal(err)
	}
	if jsAvatar.Comments[0].AvatarPath != "" {
		t.Fatalf("avatar %q", jsAvatar.Comments[0].AvatarPath)
	}
}
