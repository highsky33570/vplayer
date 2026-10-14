package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Job / trigger / status / error_code allowlists for operational_runs.
const (
	JobIncrementalSync = "incremental_sync"
	JobEnrichment      = "enrichment"
	JobMediaMigration  = "media_migration"

	TriggerCLI   = "cli"
	TriggerTimer = "timer"
	TriggerAdmin = "admin"

	OpStatusRunning   = "running"
	OpStatusSucceeded = "succeeded"
	OpStatusFailed    = "failed"
	OpStatusRejected  = "rejected"

	ErrCodeLockBusy          = "lock_busy"
	ErrCodeTimeout           = "timeout"
	ErrCodeSourceUnavailable = "source_unavailable"
	ErrCodeDatabaseError     = "database_error"
	ErrCodePartialFailure    = "partial_failure"
	ErrCodeWorkerFailed      = "worker_failed"
	ErrCodeInternalError     = "internal_error"
)

// Stale thresholds for future Admin derived status (no auto-mutation in 4A).
const (
	IncrementalStaleAfter    = 20 * time.Minute
	EnrichmentStaleAfter     = 25 * time.Minute
	MediaMigrationStaleAfter = 45 * time.Minute // longer than CLI migrate context (30m)
)

// ValidateTriggerType accepts cli|timer|admin.
func ValidateTriggerType(raw string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(raw))
	switch t {
	case TriggerCLI, TriggerTimer, TriggerAdmin:
		return t, nil
	default:
		return "", fmt.Errorf("invalid trigger %q (allowed: cli, timer, admin)", raw)
	}
}

// ValidateTriggerRequester enforces Plan 4B requester rules before any lock/crawl:
// cli|timer => requester unset/0; admin => requester > 0.
func ValidateTriggerRequester(trigger string, requestedByUserID uint64) error {
	trig, err := ValidateTriggerType(trigger)
	if err != nil {
		return err
	}
	switch trig {
	case TriggerAdmin:
		if requestedByUserID == 0 {
			return fmt.Errorf("trigger=admin requires --requested-by-user-id > 0")
		}
	default: // cli, timer
		if requestedByUserID != 0 {
			return fmt.Errorf("trigger=%s must not set requested-by-user-id", trig)
		}
	}
	return nil
}

// RequestedByPtr returns nil for 0 so cli/timer persist NULL requester.
func RequestedByPtr(id uint64) *uint64 {
	if id == 0 {
		return nil
	}
	return &id
}

// ValidateJobType accepts incremental_sync|enrichment|media_migration.
func ValidateJobType(raw string) (string, error) {
	j := strings.ToLower(strings.TrimSpace(raw))
	switch j {
	case JobIncrementalSync, JobEnrichment, JobMediaMigration:
		return j, nil
	default:
		return "", fmt.Errorf("invalid job_type %q", raw)
	}
}

func validateOpStatus(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case OpStatusRunning, OpStatusSucceeded, OpStatusFailed, OpStatusRejected:
		return s, nil
	default:
		return "", fmt.Errorf("invalid status %q", raw)
	}
}

func validateErrorCode(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	c := strings.ToLower(strings.TrimSpace(raw))
	switch c {
	case ErrCodeLockBusy, ErrCodeTimeout, ErrCodeSourceUnavailable, ErrCodeDatabaseError,
		ErrCodePartialFailure, ErrCodeWorkerFailed, ErrCodeInternalError:
		return c, nil
	default:
		return "", fmt.Errorf("invalid error_code %q", raw)
	}
}

// StartOperationalRunInput creates a running row.
type StartOperationalRunInput struct {
	JobType           string
	TriggerType       string
	RequestedByUserID *uint64
}

// FinishOperationalRunInput finalizes a running row.
type FinishOperationalRunInput struct {
	ID               uint64
	Status           string // succeeded | failed
	Scanned          *int
	CreatedCount     *int
	UpdatedCount     *int
	UnchangedCount   *int
	FailedCount      *int
	SelectedCount    *int
	SucceededCount   *int
	SkippedCount     *int
	UnavailableCount *int
	ErrorCode        string
	HasError         bool
	SyncRunID        *uint64
}

// RejectOperationalRunInput records a lock-busy (or similar) rejection without work.
type RejectOperationalRunInput struct {
	JobType           string
	TriggerType       string
	RequestedByUserID *uint64
	ErrorCode         string // typically lock_busy
}

func requesterIDValue(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

// StartOperationalRun inserts status=running with real timestamps.
func (m *MySQL) StartOperationalRun(in StartOperationalRunInput) (uint64, error) {
	job, err := ValidateJobType(in.JobType)
	if err != nil {
		return 0, err
	}
	trig, err := ValidateTriggerType(in.TriggerType)
	if err != nil {
		return 0, err
	}
	if err := ValidateTriggerRequester(trig, requesterIDValue(in.RequestedByUserID)); err != nil {
		return 0, err
	}
	now := time.Now()
	res, err := m.DB.Exec(`
		INSERT INTO operational_runs (
		  job_type, trigger_type, status, requested_by_user_id,
		  requested_at, started_at, has_error
		) VALUES (?,?,?,?,?,?,0)`,
		job, trig, OpStatusRunning, nullUint64(in.RequestedByUserID),
		now, now,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

// FinishOperationalRun sets terminal status and safe counters.
func (m *MySQL) FinishOperationalRun(in FinishOperationalRunInput) error {
	status, err := validateOpStatus(in.Status)
	if err != nil {
		return err
	}
	if status != OpStatusSucceeded && status != OpStatusFailed {
		return fmt.Errorf("finish status must be succeeded or failed")
	}
	code, err := validateErrorCode(in.ErrorCode)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = m.DB.Exec(`
		UPDATE operational_runs SET
		  status=?, finished_at=?,
		  scanned=?, created_count=?, updated_count=?, unchanged_count=?, failed_count=?,
		  selected_count=?, succeeded_count=?, skipped_count=?, unavailable_count=?,
		  error_code=?, has_error=?, sync_run_id=?
		WHERE id=? AND status=?`,
		status, now,
		nullInt(in.Scanned), nullInt(in.CreatedCount), nullInt(in.UpdatedCount),
		nullInt(in.UnchangedCount), nullInt(in.FailedCount),
		nullInt(in.SelectedCount), nullInt(in.SucceededCount), nullInt(in.SkippedCount),
		nullInt(in.UnavailableCount),
		nullStr(code), boolToInt(in.HasError), nullUint64(in.SyncRunID),
		in.ID, OpStatusRunning,
	)
	return err
}

// RejectOperationalRun inserts a terminal rejected row (no crawl performed).
func (m *MySQL) RejectOperationalRun(in RejectOperationalRunInput) (uint64, error) {
	job, err := ValidateJobType(in.JobType)
	if err != nil {
		return 0, err
	}
	trig, err := ValidateTriggerType(in.TriggerType)
	if err != nil {
		return 0, err
	}
	if err := ValidateTriggerRequester(trig, requesterIDValue(in.RequestedByUserID)); err != nil {
		return 0, err
	}
	code, err := validateErrorCode(in.ErrorCode)
	if err != nil {
		return 0, err
	}
	if code == "" {
		code = ErrCodeLockBusy
	}
	now := time.Now()
	res, err := m.DB.Exec(`
		INSERT INTO operational_runs (
		  job_type, trigger_type, status, requested_by_user_id,
		  requested_at, started_at, finished_at,
		  error_code, has_error
		) VALUES (?,?,?,?,?,?,?,?,1)`,
		job, trig, OpStatusRejected, nullUint64(in.RequestedByUserID),
		now, now, now,
		code,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullUint64(p *uint64) any {
	if p == nil {
		return nil
	}
	return *p
}

// IntPtr helpers for callers.
func IntPtr(v int) *int { return &v }

// OperationalRunRow is a safe DB projection (no raw errors/URLs).
type OperationalRunRow struct {
	ID                uint64
	JobType           string
	TriggerType       string
	Status            string
	RequestedByUserID *uint64
	RequestedAt       time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	Scanned           *int
	CreatedCount      *int
	UpdatedCount      *int
	UnchangedCount    *int
	FailedCount       *int
	SelectedCount     *int
	SucceededCount    *int
	SkippedCount      *int
	UnavailableCount  *int
	ErrorCode         string
	HasError          bool
	SyncRunID         *uint64
}

// GetOperationalRunByID loads one row for tests / future Admin reads.
func (m *MySQL) GetOperationalRunByID(id uint64) (*OperationalRunRow, error) {
	row := m.DB.QueryRow(`
		SELECT id, job_type, trigger_type, status, requested_by_user_id,
		       requested_at, started_at, finished_at,
		       scanned, created_count, updated_count, unchanged_count, failed_count,
		       selected_count, succeeded_count, skipped_count, unavailable_count,
		       COALESCE(error_code,''), has_error, sync_run_id
		FROM operational_runs WHERE id=?`, id)
	return scanOperationalRun(row)
}

func scanOperationalRun(row interface{ Scan(dest ...any) error }) (*OperationalRunRow, error) {
	var r OperationalRunRow
	var reqBy, syncID sql.NullInt64
	var started, finished sql.NullTime
	var scanned, created, updated, unchanged, failed sql.NullInt64
	var selected, succeeded, skipped, unavailable sql.NullInt64
	var hasErr int
	err := row.Scan(
		&r.ID, &r.JobType, &r.TriggerType, &r.Status, &reqBy,
		&r.RequestedAt, &started, &finished,
		&scanned, &created, &updated, &unchanged, &failed,
		&selected, &succeeded, &skipped, &unavailable,
		&r.ErrorCode, &hasErr, &syncID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if reqBy.Valid {
		v := uint64(reqBy.Int64)
		r.RequestedByUserID = &v
	}
	if started.Valid {
		t := started.Time
		r.StartedAt = &t
	}
	if finished.Valid {
		t := finished.Time
		r.FinishedAt = &t
	}
	r.Scanned = nullInt64Ptr(scanned)
	r.CreatedCount = nullInt64Ptr(created)
	r.UpdatedCount = nullInt64Ptr(updated)
	r.UnchangedCount = nullInt64Ptr(unchanged)
	r.FailedCount = nullInt64Ptr(failed)
	r.SelectedCount = nullInt64Ptr(selected)
	r.SucceededCount = nullInt64Ptr(succeeded)
	r.SkippedCount = nullInt64Ptr(skipped)
	r.UnavailableCount = nullInt64Ptr(unavailable)
	r.HasError = hasErr != 0
	if syncID.Valid {
		v := uint64(syncID.Int64)
		r.SyncRunID = &v
	}
	return &r, nil
}

func nullInt64Ptr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}
