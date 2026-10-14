package store

import (
	"database/sql"
	"strings"
	"time"
)

const AdminActionCooldown = 60 * time.Second

// HasNonStaleRunningOperationalRun reports a running row for jobType whose age
// is within the job stale threshold (Plan 4A constants). Stale running rows return false.
func (m *MySQL) HasNonStaleRunningOperationalRun(jobType string, now time.Time) (bool, error) {
	job, err := ValidateJobType(jobType)
	if err != nil {
		return false, err
	}
	threshold, ok := staleThresholdForJob(job)
	if !ok {
		return false, nil
	}
	cutoff := now.Add(-threshold)
	var id uint64
	err = m.DB.QueryRow(`
		SELECT id FROM operational_runs
		WHERE job_type=? AND status=?
		  AND COALESCE(started_at, requested_at) >= ?
		ORDER BY COALESCE(started_at, requested_at) DESC, id DESC
		LIMIT 1`,
		job, OpStatusRunning, cutoff,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// HasRecentAdminOperationalRequest reports an admin-triggered row for jobType
// with requested_at within cooldown (default 60s).
func (m *MySQL) HasRecentAdminOperationalRequest(jobType string, now time.Time, cooldown time.Duration) (bool, error) {
	job, err := ValidateJobType(jobType)
	if err != nil {
		return false, err
	}
	if cooldown <= 0 {
		cooldown = AdminActionCooldown
	}
	cutoff := now.Add(-cooldown)
	var id uint64
	err = m.DB.QueryRow(`
		SELECT id FROM operational_runs
		WHERE job_type=? AND trigger_type=? AND requested_at >= ?
		ORDER BY requested_at DESC, id DESC
		LIMIT 1`,
		job, TriggerAdmin, cutoff,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func staleThresholdForJob(jobType string) (time.Duration, bool) {
	switch strings.TrimSpace(jobType) {
	case JobIncrementalSync:
		return IncrementalStaleAfter, true
	case JobEnrichment:
		return EnrichmentStaleAfter, true
	case JobMediaMigration:
		return MediaMigrationStaleAfter, true
	default:
		return 0, false
	}
}
