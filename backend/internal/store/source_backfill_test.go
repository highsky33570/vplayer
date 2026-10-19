package store

import (
	"strings"
	"testing"
)

func TestBackfillSQLDoesNotTouchCatalogOrNativeComments(t *testing.T) {
	sqlText := listActiveSourceVideosSQL + listBackfillCheckpointsSQL + saveBackfillCheckpointSQL + countSourceCommentRowsSQL +
		listPendingBackfillSQL + listFailedBackfillSQL + firstFailedBackfillSQL + countBackfillStatusSQL + countRemainingBackfillSQL
	lower := strings.ToLower(sqlText)
	for _, banned := range []string{"video_comments", "user_id", "email", "password_hash", "delete from", "update videos"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("backfill SQL contains %s", banned)
		}
	}
	if !strings.Contains(lower, "order by id asc") {
		t.Fatal("video selection is not ordered by id")
	}
	if !strings.Contains(lower, "coalesce(is_active,1) = 1") || !strings.Contains(lower, "source_system = ?") {
		t.Fatal("selection does not require an active source video")
	}
	if strings.Contains(strings.ToLower(saveBackfillCheckpointSQL), "update videos") {
		t.Fatal("checkpoint write updates videos")
	}
	pending := strings.ToLower(listPendingBackfillSQL)
	if !strings.Contains(pending, "limit ?") || !strings.Contains(pending, "v.id > ?") {
		t.Fatal("pending drain selection is not a bounded id cursor")
	}
	if !strings.Contains(pending, "b.status = 'pending'") {
		t.Fatal("pending selection must skip completed and failed checkpoints")
	}
}
