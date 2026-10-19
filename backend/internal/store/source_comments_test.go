package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSourceCommentUpsertAndScope(t *testing.T) {
	repo := NewMemorySourceComments()
	native := NewMemoryComments()
	native.AddVideo(7)
	seeded := native.Seed(Comment{VideoID: 7, UserID: 3, Body: "native", Status: "visible", CreatedAt: time.Unix(10, 0), UpdatedAt: time.Unix(10, 0)})
	now := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	row := SourceCommentInput{
		SourceCommentID: "413210", SourceVideoID: "82861", AuthorName: "游客",
		AvatarPath: "/static/images/touxiang.png", Body: "hello", SourceCreatedRaw: "2026-10-19 01:56:45",
		DiggUp: 63, DiggDown: 0,
	}
	inserted, updated, skipped, err := repo.UpsertSourceComments(7, "olehdtv", []SourceCommentInput{row}, now)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 || updated != 0 || skipped != 0 || len(repo.Snapshot()) != 1 {
		t.Fatalf("insert ins=%d upd=%d skip=%d rows=%d", inserted, updated, skipped, len(repo.Snapshot()))
	}
	created, ok := repo.CreatedAt("olehdtv", "413210")
	if !ok || !created.Equal(now) {
		t.Fatalf("created %v", created)
	}
	again, updated, skipped, err := repo.UpsertSourceComments(7, "olehdtv", []SourceCommentInput{row}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 || updated != 0 || skipped != 1 || len(repo.Snapshot()) != 1 {
		t.Fatalf("rerun ins=%d upd=%d skip=%d rows=%d", again, updated, skipped, len(repo.Snapshot()))
	}
	created2, _ := repo.CreatedAt("olehdtv", "413210")
	if !created2.Equal(created) {
		t.Fatal("rerun recreated the row")
	}
	row.DiggUp = 70
	_, updated, _, err = repo.UpsertSourceComments(7, "olehdtv", []SourceCommentInput{row}, now.Add(2*time.Minute))
	if err != nil || updated != 1 || repo.Snapshot()[0].DiggUp != 70 || len(repo.Snapshot()) != 1 {
		t.Fatalf("digg update upd=%d snap=%+v err=%v", updated, repo.Snapshot(), err)
	}
	row.Body = "changed"
	_, updated, _, err = repo.UpsertSourceComments(7, "olehdtv", []SourceCommentInput{row}, now.Add(3*time.Minute))
	if err != nil || updated != 1 || repo.Snapshot()[0].Body != "changed" {
		t.Fatalf("body update %+v err=%v", repo.Snapshot(), err)
	}
	reply := SourceCommentInput{
		SourceCommentID: "413117", SourceVideoID: "83449", ParentSourceCommentID: "412878",
		AuthorName: "游客", Body: "reply", SourceCreatedRaw: "10:28", DiggUp: 2,
	}
	if _, _, _, err := repo.UpsertSourceComments(8, "olehdtv", []SourceCommentInput{{
		SourceCommentID: "412878", SourceVideoID: "83449", AuthorName: "游客", Body: "parent", SourceCreatedRaw: "2026-10-16 22:34:16",
	}, reply}, now); err != nil {
		t.Fatal(err)
	}
	threads, parentTotal, rowTotal, err := repo.ListSourceCommentThreads(8, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if parentTotal != 1 || rowTotal != 2 || len(threads) != 1 || len(threads[0].Replies) != 1 {
		t.Fatalf("threads parent=%d rows=%d %+v", parentTotal, rowTotal, threads)
	}
	if threads[0].Replies[0].SourceCreatedRaw != "10:28" || threads[0].Replies[0].DiggUp != 2 {
		t.Fatalf("reply %+v", threads[0].Replies[0])
	}
	other := SourceCommentInput{SourceCommentID: "413210", SourceVideoID: "other-video", AuthorName: "x", Body: "other system"}
	if _, _, _, err := repo.UpsertSourceComments(9, "other", []SourceCommentInput{other}, now); err != nil {
		t.Fatal(err)
	}
	if len(repo.Snapshot()) != 4 {
		t.Fatalf("provider scope collapsed rows: %+v", repo.Snapshot())
	}
	movieThreads, _, _, err := repo.ListSourceCommentThreads(7, 20, 0)
	if err != nil || len(movieThreads) != 1 || movieThreads[0].Body != "changed" {
		t.Fatalf("video collision %+v err=%v", movieThreads, err)
	}
	if _, _, _, err := repo.ListSourceCommentThreads(9, 20, 0); err != nil {
		t.Fatal(err)
	}
	nine, _, _, err := repo.ListSourceCommentThreads(9, 20, 0)
	if err != nil || len(nine) != 1 || nine[0].Body != "other system" {
		t.Fatalf("other system %+v", nine)
	}
	snap := native.Snapshot()
	if len(snap) != 1 || snap[0].ID != seeded.ID || snap[0].Body != "native" {
		t.Fatalf("native comments changed: %+v", snap)
	}
	repo.SetListErr(errors.New("source down"))
	if _, _, _, err := repo.ListSourceCommentThreads(7, 20, 0); err == nil {
		t.Fatal("expected list error")
	}
	if native.Snapshot()[0].Body != "native" {
		t.Fatal("source list failure changed native comments")
	}
}

func TestSourceCommentSQLStaysOffNativeComments(t *testing.T) {
	sqlText := selectSourceCommentSQL + insertSourceCommentSQL + updateSourceCommentSQL + touchSourceCommentSQL
	if strings.Contains(sqlText, "video_comments") || strings.Contains(strings.ToLower(sqlText), "user_id") {
		t.Fatalf("source SQL touches native comments or users: %s", sqlText)
	}
	for _, banned := range []string{"email", "password_hash", "is_admin"} {
		if strings.Contains(strings.ToLower(sqlText), banned) {
			t.Fatalf("sql contains %s", banned)
		}
	}
	if strings.Contains(strings.ToUpper(sqlText), "DELETE FROM") {
		t.Fatal("source upsert must not delete")
	}
	if !strings.Contains(insertSourceCommentSQL, "source_system") || !strings.Contains(updateSourceCommentSQL, "digg_up=?") {
		t.Fatal("upsert SQL missing identity or digg update")
	}
	if strings.Contains(updateSourceCommentSQL, "created_at=?") {
		t.Fatal("update must keep the original created_at")
	}
}
