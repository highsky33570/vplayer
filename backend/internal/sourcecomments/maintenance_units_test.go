package sourcecomments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaintenanceUnitsAndDeployDoNotStartWork(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	incSvc := readRepoFile(t, root, "deploy/systemd/vplayer-sourcecomments-incremental.service")
	incTimer := readRepoFile(t, root, "deploy/systemd/vplayer-sourcecomments-incremental.timer")
	fullSvc := readRepoFile(t, root, "deploy/systemd/vplayer-sourcecomments-full.service")
	fullTimer := readRepoFile(t, root, "deploy/systemd/vplayer-sourcecomments-full.timer")
	assertOneshot(t, incSvc, "/opt/vplayer/backend/bin/maintainsourcecomments --source-system olehdtv --mode incremental --confirm=MAINTENANCE", "12h")
	assertOneshot(t, fullSvc, "/opt/vplayer/backend/bin/maintainsourcecomments --source-system olehdtv --mode full --confirm=MAINTENANCE", "36h")
	assertTimer(t, incTimer, "vplayer-sourcecomments-incremental.service", "*-*-* 00/3:50:00", "20min")
	assertTimer(t, fullTimer, "vplayer-sourcecomments-full.service", "Sun *-*-* 05:15:00", "3h")
	for _, text := range []string{incSvc, fullSvc} {
		if !strings.Contains(text, "vplayer_source_comment_sync") {
			t.Fatal("service must name the shared advisory lock")
		}
		if strings.Contains(text, "concurrency") || strings.Contains(text, "--drain") {
			t.Fatal("service must not add concurrency or historical drain")
		}
	}

	deploy := readRepoFile(t, root, "deploy/deploy-production.sh")
	body := functionBody(t, deploy, "install_source_comment_maintenance_units")
	if !strings.Contains(body, "systemctl daemon-reload") {
		t.Fatal("deploy must reload systemd after installing maintenance units")
	}
	for _, banned := range []string{"systemctl enable", "systemctl start", "systemctl restart", "enable --now"} {
		if strings.Contains(body, banned) {
			t.Fatalf("maintenance install must not %s", banned)
		}
	}
	if !strings.Contains(deploy, `go build -o "${STAGING}/bin/maintainsourcecomments"`) {
		t.Fatal("deploy must build the maintenance command")
	}

	for _, rel := range []string{
		"backend/cmd/maintainsourcecomments/main.go",
		"backend/cmd/backfillsourcecomments/main.go",
		"backend/cmd/syncsourcecomments/main.go",
	} {
		text := readRepoFile(t, root, rel)
		if !strings.Contains(text, "MySQLCommentLocker") && !strings.Contains(text, "SourceCommentLockName") {
			t.Fatalf("%s does not take the shared source-comment lock", rel)
		}
		if strings.Contains(text, "saveData") || strings.Contains(text, "gbook") {
			t.Fatalf("%s references a prohibited comment endpoint", rel)
		}
	}
}

func assertOneshot(t *testing.T, text, exec, timeout string) {
	t.Helper()
	if !strings.Contains(text, "Type=oneshot") {
		t.Fatal("service must be oneshot")
	}
	if !strings.Contains(text, "WorkingDirectory=/opt/vplayer") {
		t.Fatal("WorkingDirectory must load /opt/vplayer/.env")
	}
	if strings.Contains(text, "\nUser=") || strings.Contains(text, "\nGroup=") || strings.Contains(text, "\nEnvironmentFile=") || strings.Contains(text, "\nEnvironment=") {
		t.Fatal("service must not set User, Group, or environment secrets")
	}
	if !strings.Contains(text, "ExecStart="+exec) {
		t.Fatalf("ExecStart=%s missing", exec)
	}
	if !strings.Contains(text, "TimeoutStartSec="+timeout) {
		t.Fatalf("TimeoutStartSec=%s missing", timeout)
	}
}

func assertTimer(t *testing.T, text, unit, calendar, delay string) {
	t.Helper()
	if !strings.Contains(text, "Persistent=true") || !strings.Contains(text, "RandomizedDelaySec="+delay) {
		t.Fatal("timer must persist and randomize its start")
	}
	if !strings.Contains(text, "OnCalendar="+calendar) || !strings.Contains(text, "Unit="+unit) {
		t.Fatalf("timer schedule/unit mismatch: %s %s", calendar, unit)
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Requires=") || strings.HasPrefix(trimmed, "OnBootSec=") {
			t.Fatal("timer must not Requires= the oneshot or set OnBootSec")
		}
	}
}

func functionBody(t *testing.T, script, name string) string {
	t.Helper()
	start := strings.Index(script, name+"() {")
	if start < 0 {
		t.Fatalf("missing function %s", name)
	}
	rest := script[start:]
	next := strings.Index(rest[len(name)+4:], "\n# ")
	if next < 0 {
		t.Fatalf("cannot isolate %s", name)
	}
	return rest[:len(name)+4+next]
}

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
