package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceCommentBackfillCollationMatchesVideos(t *testing.T) {
	fresh := readMigration(t, "013_source_comment_backfill.sql")
	upgrade := readMigration(t, "014_source_comment_backfill_collation.sql")

	for _, column := range []string{"source_system", "mode", "source_video_id", "status", "error_message"} {
		if !columnUsesGeneralCI(fresh, column) {
			t.Fatalf("fresh migration 013 leaves %s without utf8mb4_general_ci", column)
		}
	}
	if !strings.Contains(fresh, "DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci") {
		t.Fatal("fresh table default collation must be utf8mb4_general_ci")
	}
	if !strings.Contains(fresh, "UNIQUE KEY uq_source_comment_backfill (source_system, video_id, mode)") {
		t.Fatal("unique checkpoint key changed")
	}
	if !strings.Contains(fresh, "KEY idx_source_comment_backfill_status (source_system, mode, status, video_id)") {
		t.Fatal("status index changed")
	}

	lowerUpgrade := strings.ToLower(upgrade)
	if !strings.Contains(lowerUpgrade, "alter table source_comment_backfill") {
		t.Fatal("014 must alter source_comment_backfill")
	}
	if !strings.Contains(lowerUpgrade, "convert to character set utf8mb4 collate utf8mb4_general_ci") {
		t.Fatal("014 must convert the checkpoint table to utf8mb4_general_ci")
	}
	for _, banned := range []string{"alter table videos", "delete from", "drop table", "truncate"} {
		if strings.Contains(lowerUpgrade, banned) {
			t.Fatalf("014 contains %s", banned)
		}
	}

	for _, query := range []string{listPendingBackfillSQL, listFailedBackfillSQL, countRemainingBackfillSQL} {
		if !strings.Contains(query, "b.source_system = v.source_system") {
			t.Fatal("backfill selection no longer compares checkpoint and video source_system")
		}
		if strings.Contains(strings.ToLower(query), "collate") {
			t.Fatal("query-level COLLATE hides the schema mismatch")
		}
	}
}

func columnUsesGeneralCI(sqlText, column string) bool {
	for _, line := range strings.Split(sqlText, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, column+" ") && strings.Contains(trimmed, "utf8mb4_general_ci") {
			return true
		}
	}
	return false
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
