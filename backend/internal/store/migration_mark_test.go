package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkEpisodeMigratedSQL_successOnlyMigratedAtClearsFailedAt(t *testing.T) {
	src := readStoreFile(t, "sync.go")
	// Locate MarkEpisodeMigrated body roughly.
	idx := strings.Index(src, "func (m *MySQL) MarkEpisodeMigrated")
	if idx < 0 {
		t.Fatal("MarkEpisodeMigrated missing")
	}
	end := strings.Index(src[idx:], "func (m *MySQL) MarkEpisodeMigrationFailed")
	if end < 0 {
		t.Fatal("MarkEpisodeMigrationFailed missing after MarkEpisodeMigrated")
	}
	body := src[idx : idx+end]
	if !strings.Contains(body, "migrated_at=?") {
		t.Fatal("success must set migrated_at")
	}
	if !strings.Contains(body, "migration_failed_at=NULL") {
		t.Fatal("success must clear migration_failed_at")
	}
	if strings.Contains(body, "migration_failed_at=?") {
		t.Fatal("success must not set migration_failed_at to a timestamp")
	}
}

func TestMarkEpisodeMigrationFailedSQL_setsFailedAtNotMigratedAt(t *testing.T) {
	src := readStoreFile(t, "sync.go")
	idx := strings.Index(src, "func (m *MySQL) MarkEpisodeMigrationFailed")
	if idx < 0 {
		t.Fatal("MarkEpisodeMigrationFailed missing")
	}
	end := strings.Index(src[idx:], "func (m *MySQL) GetEpisodeMigrationState")
	if end < 0 {
		t.Fatal("GetEpisodeMigrationState missing after MarkEpisodeMigrationFailed")
	}
	body := src[idx : idx+end]
	setIdx := strings.Index(body, "SET ")
	if setIdx < 0 {
		t.Fatal("UPDATE SET missing")
	}
	setClause := body[setIdx:]
	whereIdx := strings.Index(setClause, "WHERE")
	if whereIdx > 0 {
		setClause = setClause[:whereIdx]
	}
	if !strings.Contains(setClause, "migration_status='failed'") {
		t.Fatal("failure must set migration_status=failed")
	}
	if !strings.Contains(setClause, "migration_error=?") {
		t.Fatal("failure must preserve migration_error write")
	}
	if !strings.Contains(setClause, "migration_failed_at=?") {
		t.Fatal("failure must set migration_failed_at")
	}
	if strings.Contains(setClause, "migrated_at") {
		t.Fatal("failure must not write migrated_at")
	}
}

func TestMigration007AddsMigrationFailedAt(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "007_media_migration_failed_at.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b := string(raw)
	if !strings.Contains(b, "migration_failed_at DATETIME NULL") {
		t.Fatal("007 must add migration_failed_at DATETIME NULL")
	}
	if strings.Contains(strings.ToLower(b), "index") {
		t.Fatal("007 must not add an index unless required")
	}
}
