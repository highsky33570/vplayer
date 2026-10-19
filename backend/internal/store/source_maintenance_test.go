package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaintenanceSQLDoesNotTouchHistoricalOrNativeComments(t *testing.T) {
	sqlText := latestMaintenanceCycleSQL + insertMaintenanceCycleSQL + updateMaintenanceCycleSQL +
		maxEligibleVideoIDSQL + listPendingMaintenanceSQL + listFailedMaintenanceSQL +
		firstFailedMaintenanceSQL + saveMaintenanceCheckpointSQL + countMaintenanceStatusSQL +
		countRemainingMaintenanceSQL + countVideosAboveBoundarySQL
	lower := strings.ToLower(sqlText)
	for _, banned := range []string{
		"source_comment_backfill", "video_comments", "user_id", "email", "password_hash",
		"delete from", "update videos", "drop table",
	} {
		if strings.Contains(lower, banned) {
			t.Fatalf("maintenance SQL contains %s", banned)
		}
	}
	if strings.Contains(lower, "collate ") {
		t.Fatal("query-level COLLATE hides a schema mismatch")
	}
	pending := strings.ToLower(listPendingMaintenanceSQL)
	if !strings.Contains(pending, "m.cycle_id = ?") || !strings.Contains(pending, "m.video_id = v.id") {
		t.Fatal("pending selection must join on cycle and video id")
	}
	if !strings.Contains(pending, "v.id <= ?") || !strings.Contains(pending, "v.id > ?") || !strings.Contains(pending, "limit ?") {
		t.Fatal("pending selection is not bounded by the cycle id cursor")
	}
	if !strings.Contains(pending, "m.status = 'pending'") {
		t.Fatal("pending selection must skip completed and failed videos")
	}
	if strings.Contains(pending, "b.source_system = v.source_system") || strings.Contains(pending, "m.source_system = v.source_system") {
		t.Fatal("maintenance selection must not compare source_system across tables")
	}
	if SourceCommentLockName != "vplayer_source_comment_sync" {
		t.Fatalf("lock name=%q", SourceCommentLockName)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "015_source_comment_maintenance.sql"))
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	if strings.Contains(strings.ToLower(migration), "alter table source_comment_backfill") ||
		strings.Contains(strings.ToLower(migration), "update source_comment_backfill") ||
		strings.Contains(strings.ToLower(migration), "delete from source_comment_backfill") ||
		strings.Contains(strings.ToLower(migration), "drop table source_comment_backfill") {
		t.Fatal("migration 015 must not alter historical checkpoints")
	}
	if strings.Contains(strings.ToLower(migration), "alter table videos") {
		t.Fatal("migration 015 must not alter videos")
	}
	for _, column := range []string{"source_system", "mode", "source_video_id", "status", "error_message"} {
		if !maintenanceColumnUsesGeneralCI(migration, column) {
			t.Fatalf("migration 015 leaves %s without utf8mb4_general_ci", column)
		}
	}
	if !strings.Contains(migration, "UNIQUE KEY uq_source_comment_maintenance (cycle_id, video_id)") {
		t.Fatal("per-video checkpoint unique key changed")
	}
}

func maintenanceColumnUsesGeneralCI(sqlText, column string) bool {
	for _, line := range strings.Split(sqlText, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, column+" ") && strings.Contains(trimmed, "utf8mb4_general_ci") {
			return true
		}
	}
	return false
}
