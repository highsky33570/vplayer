package httpapi

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

// Fixed production worker paths (never browser-supplied).
const (
	WorkerBinDir           = "/opt/vplayer/backend/bin"
	WorkerWorkingDir       = "/opt/vplayer/backend"
	SyncIncrementalBin     = WorkerBinDir + "/syncincremental"
	EnrichPendingBin       = WorkerBinDir + "/enrichpending"
	AdminIncrementalLimit  = 5
	AdminEnrichmentLimit   = 15
	AdminEnrichmentConc    = 1
)

// WorkerLauncher starts a fixed worker process asynchronously (mockable in tests).
type WorkerLauncher interface {
	Start(path string, args []string, workDir string) error
}

// ExecWorkerLauncher uses os/exec with no shell.
type ExecWorkerLauncher struct {
	Log *slog.Logger
}

func (l ExecWorkerLauncher) Start(path string, args []string, workDir string) error {
	cmd := exec.Command(path, args...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Inherit API process environment (includes secrets from systemd EnvironmentFile).
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		if waitErr := cmd.Wait(); waitErr != nil && l.Log != nil {
			l.Log.Info("admin worker process exited", "err", waitErr.Error(), "path", path)
		}
	}()
	return nil
}

func incrementalAdminArgs(adminUserID uint64) []string {
	return []string{
		"--limit", strconv.Itoa(AdminIncrementalLimit),
		"--trigger=admin",
		"--requested-by-user-id=" + strconv.FormatUint(adminUserID, 10),
	}
}

func enrichmentAdminArgs(adminUserID uint64) []string {
	return []string{
		"--limit", strconv.Itoa(AdminEnrichmentLimit),
		"--concurrency", strconv.Itoa(AdminEnrichmentConc),
		"--trigger=admin",
		"--requested-by-user-id=" + strconv.FormatUint(adminUserID, 10),
	}
}

func formatLaunchLog(path string, args []string) string {
	return fmt.Sprintf("%s %v", path, args)
}
