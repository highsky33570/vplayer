package server

import (
	"os"
	"strings"
	"testing"
)

func TestAPIStartupMigrationFailureIsFatal(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	if strings.Contains(src, "migrate warning") {
		t.Fatal("migration failure must not be logged and ignored")
	}
	if !strings.Contains(src, `return nil, fmt.Errorf("migrate: %w", err)`) {
		t.Fatal("ApplyMigrationsDir failure must fail server.New")
	}
}
