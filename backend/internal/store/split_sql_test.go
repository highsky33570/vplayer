package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The production failure. splitSQL used strings.Split on every ';', so these
// comment semicolons were executed as their own statements:
//
//	migration 018_episode_restriction_reason:
//	Error 1064 ... near 'existing rows read as '').
const migration018WithCommentSemicolons = `-- Explicit OLEHDTV membership classification. Additive only: no deletes, no index.
-- Appended as the last column with a constant default so MySQL 8.0 can apply
-- ALGORITHM=INSTANT (metadata only; existing rows read as '').
-- playback_status stays "unavailable" so watch playback keeps the existing
-- unavailable response. restriction_reason records why, and stays empty for
-- pending, parser, and playable rows. Historical unavailable rows are not
-- guessed here; they stay '' until an explicit membership classification.
ALTER TABLE video_episodes
  ADD COLUMN restriction_reason VARCHAR(32) NOT NULL DEFAULT '',
  ALGORITHM=INSTANT;
`

func TestSplitSQL_semicolonsInLineAndBlockCommentsAreNotStatements(t *testing.T) {
	stmts := splitSQL(migration018WithCommentSemicolons)
	if len(stmts) != 1 {
		t.Fatalf("comment semicolons became %d statements:\n%s", len(stmts), strings.Join(stmts, "\n---\n"))
	}
	if !strings.Contains(stmts[0], "ADD COLUMN restriction_reason") {
		t.Fatalf("lost the ALTER: %s", stmts[0])
	}
	for _, stmt := range stmts {
		trimmed := strings.TrimSpace(stmt)
		if strings.HasPrefix(trimmed, "existing rows read as") || strings.HasPrefix(trimmed, "they stay") {
			t.Fatalf("comment fragment executed as SQL: %q", trimmed)
		}
	}

	block := "/* metadata only; existing rows read as '' */\nALTER TABLE video_episodes ADD COLUMN restriction_reason VARCHAR(32) NOT NULL DEFAULT '';\n"
	stmts = splitSQL(block)
	if len(stmts) != 1 {
		t.Fatalf("block-comment semicolon became %d statements: %#v", len(stmts), stmts)
	}
	if strings.HasPrefix(strings.TrimSpace(stmts[0]), "existing rows") {
		t.Fatalf("block-comment fragment executed as SQL: %q", stmts[0])
	}

	mixed := "-- line; still a comment\nSELECT 1;\n/* block; still a comment */\nSELECT 2;\n"
	stmts = splitSQL(mixed)
	if len(stmts) != 2 {
		t.Fatalf("got %d statements: %#v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "SELECT 1") || strings.Contains(stmts[0], "SELECT 2") {
		t.Fatalf("first statement = %q", stmts[0])
	}
	if !strings.Contains(stmts[1], "SELECT 2") || strings.Contains(stmts[1], "SELECT 1") {
		t.Fatalf("second statement = %q", stmts[1])
	}
	for _, stmt := range stmts {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "still a comment" || strings.HasPrefix(trimmed, "still a comment") {
			t.Fatalf("comment tail executed as SQL: %q", trimmed)
		}
	}
}

func TestSplitSQL_semicolonsInsideQuotesStayInStatement(t *testing.T) {
	script := "INSERT INTO t (a) VALUES ('a;b'), (\"c;d\"), (`e;f`);\nSELECT 1;\n"
	stmts := splitSQL(script)
	if len(stmts) != 2 {
		t.Fatalf("got %d statements: %#v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "'a;b'") || !strings.Contains(stmts[0], `"c;d"`) || !strings.Contains(stmts[0], "`e;f`") {
		t.Fatalf("quoted semicolons were split out: %q", stmts[0])
	}
	if stmts[1] != "SELECT 1" {
		t.Fatalf("second statement = %q", stmts[1])
	}

	escaped := "SELECT 'a\\';b', 'c'';d';\n"
	stmts = splitSQL(escaped)
	if len(stmts) != 1 || !strings.Contains(stmts[0], `'a\';b'`) || !strings.Contains(stmts[0], `'c'';d'`) {
		t.Fatalf("escaped quotes split the statement: %#v", stmts)
	}
}

func TestSplitSQL_migration018FileIsOneAlter(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "018_episode_restriction_reason.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "metadata only;") || strings.Contains(text, "guessed here;") {
		t.Fatal("018 comments still contain semicolons")
	}
	if !strings.Contains(text, "metadata only, existing rows read as ''") {
		t.Fatal("missing corrected ALGORITHM comment")
	}
	if !strings.Contains(text, "guessed here, they stay '' until an explicit membership classification.") {
		t.Fatal("missing corrected classification comment")
	}
	stmts := splitSQL(text)
	if len(stmts) != 1 {
		t.Fatalf("018 split into %d statements: %#v", len(stmts), stmts)
	}
	upper := strings.ToUpper(stmts[0])
	if !strings.Contains(upper, "ADD COLUMN RESTRICTION_REASON") || !strings.Contains(upper, "ALGORITHM=INSTANT") {
		t.Fatalf("statement = %s", stmts[0])
	}
}

func TestSplitSQL_existingMigrationsKeepStatementBoundaries(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		got := splitSQL(string(raw))
		want := naiveSplitSQL(string(raw))
		if len(got) != len(want) {
			t.Fatalf("%s: comment-aware split %d statements, terminator split %d", e.Name(), len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s statement %d changed\n got: %q\nwant: %q", e.Name(), i, got[i], want[i])
			}
		}
	}
}

func naiveSplitSQL(sqlText string) []string {
	parts := strings.Split(sqlText, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
