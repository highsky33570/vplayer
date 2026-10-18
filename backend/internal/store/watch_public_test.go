package store

import (
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

func TestRankRelatedVideos(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	current := model.Video{ID: 1, CategoryID: 10, LeafSourceID: "leaf-a", Status: "ready", IsActive: true, CreatedAt: now}
	candidates := []model.Video{
		current,
		{ID: 1, Title: "dup-current", CategoryID: 10, LeafSourceID: "leaf-a", Status: "ready", IsActive: true, CreatedAt: now},
		{ID: 2, Title: "same-leaf", CategoryID: 10, LeafSourceID: "leaf-a", Status: "ready", IsActive: true, CreatedAt: now.Add(-time.Hour)},
		{ID: 3, Title: "same-leaf-newer", CategoryID: 10, LeafSourceID: " leaf-a ", Status: "demo", IsActive: true, CreatedAt: now},
		{ID: 4, Title: "same-cat", CategoryID: 10, LeafSourceID: "other", Status: "ready", IsActive: true, CreatedAt: now.Add(2 * time.Hour)},
		{ID: 5, Title: "recent", CategoryID: 99, Status: "ready", IsActive: true, CreatedAt: now.Add(3 * time.Hour)},
		{ID: 6, Title: "older", CategoryID: 99, Status: "ready", IsActive: true, CreatedAt: now.Add(-time.Hour)},
		{ID: 7, Title: "inactive", CategoryID: 10, LeafSourceID: "leaf-a", Status: "ready", IsActive: false, CreatedAt: now.Add(9 * time.Hour)},
		{ID: 8, Title: "draft", CategoryID: 10, LeafSourceID: "leaf-a", Status: "draft", IsActive: true, CreatedAt: now.Add(9 * time.Hour)},
		{ID: 9, Title: "disabled", CategoryID: 10, Status: "disabled", IsActive: true, CreatedAt: now.Add(9 * time.Hour)},
		{ID: 3, Title: "duplicate-id", CategoryID: 99, Status: "ready", IsActive: true, CreatedAt: now.Add(8 * time.Hour)},
		{ID: 10, Title: "empty-leaf-not-tier1", CategoryID: 10, LeafSourceID: "", Status: "ready", IsActive: true, CreatedAt: now.Add(time.Hour)},
	}
	got := RankRelatedVideos(current, candidates, 10)
	var titles []string
	seen := map[uint64]bool{}
	for _, v := range got {
		titles = append(titles, v.Title)
		if seen[v.ID] {
			t.Fatalf("duplicate id %d", v.ID)
		}
		seen[v.ID] = true
		if v.ID == current.ID {
			t.Fatal("current video returned")
		}
		if !v.IsActive || (v.Status != "ready" && v.Status != "demo") {
			t.Fatalf("non-public video %s", v.Title)
		}
	}
	want := []string{"same-leaf-newer", "same-leaf", "same-cat", "empty-leaf-not-tier1", "recent", "older"}
	if strings.Join(titles, ",") != strings.Join(want, ",") {
		t.Fatalf("order=%v", titles)
	}
}

func TestRankRelatedEmptyLeafDoesNotMatchBlankLeaves(t *testing.T) {
	now := time.Now().UTC()
	current := model.Video{ID: 1, CategoryID: 4, LeafSourceID: "  ", Status: "ready", IsActive: true, CreatedAt: now}
	candidates := []model.Video{
		{ID: 2, Title: "blank", CategoryID: 4, LeafSourceID: "", Status: "ready", IsActive: true, CreatedAt: now.Add(-time.Hour)},
		{ID: 3, Title: "other-cat-newer", CategoryID: 8, Status: "ready", IsActive: true, CreatedAt: now},
	}
	got := RankRelatedVideos(current, candidates, 10)
	if len(got) != 2 || got[0].Title != "blank" || got[1].Title != "other-cat-newer" {
		t.Fatalf("got %#v", got)
	}
}

func TestRankRelatedLimitClampedTo10(t *testing.T) {
	now := time.Now().UTC()
	current := model.Video{ID: 1, Status: "ready", IsActive: true}
	var candidates []model.Video
	for i := 2; i <= 20; i++ {
		candidates = append(candidates, model.Video{
			ID: uint64(i), Status: "ready", IsActive: true, CreatedAt: now.Add(time.Duration(i) * time.Minute),
		})
	}
	if got := RankRelatedVideos(current, candidates, 50); len(got) != 10 {
		t.Fatalf("len=%d", len(got))
	}
	if got := RankRelatedVideos(current, candidates, 0); len(got) != 10 {
		t.Fatalf("default len=%d", len(got))
	}
}

func TestRelatedSQLShape(t *testing.T) {
	sqlText := relatedVideosSQL
	for _, needle := range []string{
		"status IN ('ready','demo')",
		"COALESCE(is_active,1)=1",
		"id<>?",
		"WHEN ? <> '' AND category_id=? AND leaf_source_id=? THEN 0",
		"WHEN category_id=? THEN 1",
		"created_at DESC, id DESC",
		"LIMIT ?",
	} {
		if !strings.Contains(sqlText, needle) {
			t.Fatalf("missing %s", needle)
		}
	}
	if strings.Contains(sqlText, "exclude_category") || strings.Contains(strings.ToLower(sqlText), "midnight") {
		t.Fatal("homepage exclusion leaked into related SQL")
	}
}

func TestPublicEpisodeSQLAndOrder(t *testing.T) {
	sqlText := publicEpisodesSQL
	if strings.Contains(sqlText, "playback_url") || strings.Contains(sqlText, "fallback_url") || strings.Contains(sqlText, "hls_object_key") {
		t.Fatalf("episode SQL leaks playback fields: %s", sqlText)
	}
	for _, needle := range []string{"SELECT sid, nid, COALESCE(title,'')", "COALESCE(is_active,1)=1", "ORDER BY sid ASC, nid ASC"} {
		if !strings.Contains(sqlText, needle) {
			t.Fatalf("missing %s", needle)
		}
	}
	got := SortPublicEpisodes([]PublicEpisode{
		{SID: 2, NID: 1, Title: "b"},
		{SID: 1, NID: 3, Title: "a3"},
		{SID: 1, NID: 1, Title: "a1"},
	})
	if got[0].Title != "a1" || got[1].Title != "a3" || got[2].Title != "b" {
		t.Fatalf("order=%v", got)
	}
}

func TestCommentSQLDoesNotSelectSecrets(t *testing.T) {
	sqlText := listVisibleCommentsSQL + countUserCommentsSinceSQL + softDeleteCommentSQL
	for _, banned := range []string{"email", "password_hash", "is_admin"} {
		if strings.Contains(strings.ToLower(sqlText), banned) {
			t.Fatalf("comment SQL contains %s", banned)
		}
	}
	if !strings.Contains(listVisibleCommentsSQL, "c.status='visible'") {
		t.Fatal("list must filter visible")
	}
	if !strings.Contains(listVisibleCommentsSQL, "ORDER BY c.created_at DESC, c.id DESC") {
		t.Fatal("list order")
	}
	if !strings.Contains(listVisibleCommentsSQL, "DATE_FORMAT(c.created_at, '%Y-%m-%d %H:%i:%s')") ||
		!strings.Contains(listVisibleCommentsSQL, "DATE_FORMAT(c.updated_at, '%Y-%m-%d %H:%i:%s')") {
		t.Fatal("comment timestamps must be formatted before scan, matching pending registrations")
	}
	if !strings.Contains(countUserCommentsSinceSQL, "FROM video_comments WHERE user_id=?") {
		t.Fatal("rate count must include every status")
	}
	if !strings.Contains(softDeleteCommentSQL, "status='deleted'") || strings.Contains(strings.ToUpper(softDeleteCommentSQL), "DELETE FROM") {
		t.Fatal("delete must be soft")
	}
}
