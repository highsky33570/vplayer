package store

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (m *MySQL) EnsureMigrationsTable() error {
	_, err := m.DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(64) NOT NULL PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (m *MySQL) appliedVersions() (map[string]bool, error) {
	if err := m.EnsureMigrationsTable(); err != nil {
		return nil, err
	}
	rows, err := m.DB.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// ApplyMigrationsDir runs *.sql files in order, once each.
func (m *MySQL) ApplyMigrationsDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	applied, err := m.appliedVersions()
	if err != nil {
		return err
	}
	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		if applied[version] {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		log.Printf("applying migration %s", version)
		if err := m.Migrate(string(raw)); err != nil {
			// Tolerate re-add of columns/indexes when partially applied.
			if !isIgnorableMigrationErr(err) {
				return fmt.Errorf("migration %s: %w", version, err)
			}
			log.Printf("migration %s: ignored: %v", version, err)
		}
		if _, err := m.DB.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			return err
		}
	}
	return nil
}

func isIgnorableMigrationErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "duplicate key name") ||
		strings.Contains(msg, "already exists")
}

func FindMigrationsDir() string {
	candidates := []string{
		"migrations",
		filepath.Join("backend", "migrations"),
		filepath.Join("..", "migrations"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}
