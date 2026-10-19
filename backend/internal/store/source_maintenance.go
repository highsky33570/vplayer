package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SourceCommentLockName is the MySQL advisory lock shared by source-comment
// maintenance, one-video sync, and historical backfill. Those commands must not overlap.
const SourceCommentLockName = "vplayer_source_comment_sync"

const (
	MaintenanceModeIncremental = "incremental"
	MaintenanceModeFull        = "full"
	MaintenanceRunning         = "running"
	MaintenanceCycleCompleted  = "completed"
	MaintenanceCycleFailed     = "failed"
	MaintenancePending         = "pending"
	MaintenanceVideoCompleted  = "completed"
	MaintenanceVideoFailed     = "failed"
)

// MaintenanceCycle is one incremental or full pass over a frozen video-id range.
type MaintenanceCycle struct {
	ID           uint64
	SourceSystem string
	Mode         string
	Status       string
	MaxVideoID   uint64
	ErrorMessage string
	StartedAt    time.Time
	CompletedAt  time.Time
}

// MaintenanceCheckpoint is one video inside a maintenance cycle.
type MaintenanceCheckpoint struct {
	CycleID       uint64
	SourceSystem  string
	Mode          string
	VideoID       uint64
	SourceVideoID string
	Status        string
	PagesFetched  int
	Discovered    int
	Inserted      int
	Updated       int
	Skipped       int
	ErrorMessage  string
	StartedAt     time.Time
	CompletedAt   time.Time
}

const latestMaintenanceCycleSQL = `
	SELECT id, source_system, mode, status, max_video_id, COALESCE(error_message,''),
	       DATE_FORMAT(started_at, '%Y-%m-%d %H:%i:%s'),
	       DATE_FORMAT(completed_at, '%Y-%m-%d %H:%i:%s')
	FROM source_comment_maintenance_cycles
	WHERE source_system=? AND mode=?
	ORDER BY id DESC
	LIMIT 1`

const insertMaintenanceCycleSQL = `
	INSERT INTO source_comment_maintenance_cycles (
		source_system, mode, status, max_video_id, error_message,
		started_at, completed_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

const updateMaintenanceCycleSQL = `
	UPDATE source_comment_maintenance_cycles
	SET status=?, error_message=?, completed_at=?, updated_at=?
	WHERE id=?`

const maxEligibleVideoIDSQL = `
	SELECT COALESCE(MAX(v.id), 0)
	FROM videos v
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1`

const listPendingMaintenanceSQL = `
	SELECT v.id, v.source_system, v.source_id, COALESCE(v.title,'')
	FROM videos v
	LEFT JOIN source_comment_maintenance m
	  ON m.cycle_id = ? AND m.video_id = v.id
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1
	  AND v.id <= ?
	  AND v.id > ?
	  AND (m.video_id IS NULL OR m.status = 'pending')
	ORDER BY v.id ASC
	LIMIT ?`

const listFailedMaintenanceSQL = `
	SELECT v.id, v.source_system, v.source_id, COALESCE(v.title,'')
	FROM videos v
	INNER JOIN source_comment_maintenance m
	  ON m.cycle_id = ? AND m.video_id = v.id AND m.status = 'failed'
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1
	  AND v.id <= ?
	ORDER BY v.id ASC`

const firstFailedMaintenanceSQL = `
	SELECT video_id, source_video_id, COALESCE(error_message,'')
	FROM source_comment_maintenance
	WHERE cycle_id=? AND status='failed'
	ORDER BY video_id ASC
	LIMIT 1`

const saveMaintenanceCheckpointSQL = `
	INSERT INTO source_comment_maintenance (
		cycle_id, source_system, mode, video_id, source_video_id, status,
		pages_fetched, comments_discovered, inserted_count, updated_count, skipped_count,
		error_message, last_started_at, last_completed_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		source_video_id=VALUES(source_video_id),
		status=VALUES(status),
		pages_fetched=VALUES(pages_fetched),
		comments_discovered=VALUES(comments_discovered),
		inserted_count=VALUES(inserted_count),
		updated_count=VALUES(updated_count),
		skipped_count=VALUES(skipped_count),
		error_message=VALUES(error_message),
		last_started_at=VALUES(last_started_at),
		last_completed_at=VALUES(last_completed_at),
		updated_at=VALUES(updated_at)`

const countMaintenanceStatusSQL = `
	SELECT CAST(COALESCE(SUM(status='completed'),0) AS SIGNED),
	       CAST(COALESCE(SUM(status='failed'),0) AS SIGNED)
	FROM source_comment_maintenance
	WHERE cycle_id=?`

const countRemainingMaintenanceSQL = `
	SELECT COUNT(*)
	FROM videos v
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1
	  AND v.id <= ?
	  AND NOT EXISTS (
	    SELECT 1 FROM source_comment_maintenance m
	    WHERE m.cycle_id = ?
	      AND m.video_id = v.id
	      AND m.status = 'completed'
	  )`

const countVideosAboveBoundarySQL = `
	SELECT COUNT(*)
	FROM videos v
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1
	  AND v.id > ?`

// LatestMaintenanceCycle returns the newest cycle for one system and mode.
func (m *MySQL) LatestMaintenanceCycle(sourceSystem, mode string) (*MaintenanceCycle, error) {
	var row MaintenanceCycle
	var started, completed sql.NullString
	err := m.DB.QueryRow(latestMaintenanceCycleSQL, strings.TrimSpace(sourceSystem), strings.TrimSpace(mode)).Scan(
		&row.ID, &row.SourceSystem, &row.Mode, &row.Status, &row.MaxVideoID, &row.ErrorMessage, &started, &completed,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if started.Valid && started.String != "" {
		row.StartedAt, err = parseUTCDateTime(started.String)
		if err != nil {
			return nil, err
		}
	}
	if completed.Valid && completed.String != "" {
		row.CompletedAt, err = parseUTCDateTime(completed.String)
		if err != nil {
			return nil, err
		}
	}
	return &row, nil
}

// CreateMaintenanceCycle inserts a new cycle and returns it with its id.
// max_video_id is the stable catalog boundary and is not changed later.
func (m *MySQL) CreateMaintenanceCycle(row MaintenanceCycle, now time.Time) (MaintenanceCycle, error) {
	if err := validateMaintenanceCycle(row); err != nil {
		return MaintenanceCycle{}, err
	}
	now = now.UTC()
	stamp := formatUTCDateTime(now)
	var started any
	if !row.StartedAt.IsZero() {
		started = formatUTCDateTime(row.StartedAt.UTC())
	} else {
		started = stamp
		row.StartedAt = now
	}
	var completed any
	if !row.CompletedAt.IsZero() {
		completed = formatUTCDateTime(row.CompletedAt.UTC())
	}
	res, err := m.DB.Exec(insertMaintenanceCycleSQL,
		row.SourceSystem, row.Mode, row.Status, row.MaxVideoID, nullIfEmpty(truncateRunes(row.ErrorMessage, 512)),
		started, completed, stamp, stamp,
	)
	if err != nil {
		return MaintenanceCycle{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return MaintenanceCycle{}, err
	}
	row.ID = uint64(id)
	return row, nil
}

// SaveMaintenanceCycleStatus updates status only. It does not change max_video_id.
func (m *MySQL) SaveMaintenanceCycleStatus(id uint64, status, errorMessage string, completedAt, now time.Time) error {
	if id == 0 {
		return fmt.Errorf("incomplete maintenance cycle")
	}
	switch status {
	case MaintenanceRunning, MaintenanceCycleCompleted, MaintenanceCycleFailed:
	default:
		return fmt.Errorf("invalid maintenance cycle status %q", status)
	}
	now = now.UTC()
	var completed any
	if !completedAt.IsZero() {
		completed = formatUTCDateTime(completedAt.UTC())
	}
	_, err := m.DB.Exec(updateMaintenanceCycleSQL, status, nullIfEmpty(truncateRunes(errorMessage, 512)), completed, formatUTCDateTime(now), id)
	return err
}

// MaxEligibleVideoID is the highest active OLEHDTV video id at cycle start.
func (m *MySQL) MaxEligibleVideoID(sourceSystem string) (uint64, error) {
	var id uint64
	err := m.DB.QueryRow(maxEligibleVideoIDSQL, strings.TrimSpace(sourceSystem)).Scan(&id)
	return id, err
}

// ListPendingMaintenanceVideos returns the next unfinished videos inside the cycle boundary.
func (m *MySQL) ListPendingMaintenanceVideos(cycleID, maxVideoID, afterID uint64, sourceSystem string, limit int) ([]SourceVideoCandidate, error) {
	if limit < 1 {
		return nil, fmt.Errorf("invalid maintenance page size")
	}
	rows, err := m.DB.Query(listPendingMaintenanceSQL, cycleID, strings.TrimSpace(sourceSystem), maxVideoID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSourceVideoCandidates(rows)
}

// ListFailedMaintenanceVideos returns active failed videos inside the cycle boundary, lowest id first.
func (m *MySQL) ListFailedMaintenanceVideos(cycleID, maxVideoID uint64, sourceSystem string) ([]SourceVideoCandidate, error) {
	rows, err := m.DB.Query(listFailedMaintenanceSQL, cycleID, strings.TrimSpace(sourceSystem), maxVideoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSourceVideoCandidates(rows)
}

// FirstFailedMaintenance returns the lowest-id failed checkpoint in the cycle, including an inactive video.
func (m *MySQL) FirstFailedMaintenance(cycleID uint64) (*MaintenanceCheckpoint, error) {
	var row MaintenanceCheckpoint
	err := m.DB.QueryRow(firstFailedMaintenanceSQL, cycleID).Scan(&row.VideoID, &row.SourceVideoID, &row.ErrorMessage)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.CycleID = cycleID
	row.Status = MaintenanceVideoFailed
	return &row, nil
}

// SaveMaintenanceCheckpoint upserts one video inside a cycle. It does not write videos or video_comments.
func (m *MySQL) SaveMaintenanceCheckpoint(row MaintenanceCheckpoint, now time.Time) error {
	if err := validateMaintenanceCheckpoint(row); err != nil {
		return err
	}
	now = now.UTC()
	stamp := formatUTCDateTime(now)
	var started any
	if !row.StartedAt.IsZero() {
		started = formatUTCDateTime(row.StartedAt.UTC())
	}
	var completed any
	if !row.CompletedAt.IsZero() {
		completed = formatUTCDateTime(row.CompletedAt.UTC())
	}
	_, err := m.DB.Exec(saveMaintenanceCheckpointSQL,
		row.CycleID, row.SourceSystem, row.Mode, row.VideoID, row.SourceVideoID, row.Status,
		row.PagesFetched, row.Discovered, row.Inserted, row.Updated, row.Skipped,
		nullIfEmpty(truncateRunes(row.ErrorMessage, 512)),
		started, completed, stamp, stamp,
	)
	return err
}

// CountMaintenanceCheckpointStatus counts completed and failed videos in one cycle.
func (m *MySQL) CountMaintenanceCheckpointStatus(cycleID uint64) (completed, failed int, err error) {
	err = m.DB.QueryRow(countMaintenanceStatusSQL, cycleID).Scan(&completed, &failed)
	return completed, failed, err
}

// CountRemainingMaintenance counts eligible videos inside the boundary that are not completed.
func (m *MySQL) CountRemainingMaintenance(cycleID, maxVideoID uint64, sourceSystem string) (int, error) {
	var n int
	err := m.DB.QueryRow(countRemainingMaintenanceSQL, strings.TrimSpace(sourceSystem), maxVideoID, cycleID).Scan(&n)
	return n, err
}

// CountVideosAboveBoundary counts active source videos inserted after the cycle boundary.
func (m *MySQL) CountVideosAboveBoundary(sourceSystem string, maxVideoID uint64) (int, error) {
	var n int
	err := m.DB.QueryRow(countVideosAboveBoundarySQL, strings.TrimSpace(sourceSystem), maxVideoID).Scan(&n)
	return n, err
}

func validateMaintenanceCycle(row MaintenanceCycle) error {
	switch row.Mode {
	case MaintenanceModeIncremental, MaintenanceModeFull:
	default:
		return fmt.Errorf("invalid maintenance mode %q", row.Mode)
	}
	switch row.Status {
	case MaintenanceRunning, MaintenanceCycleCompleted, MaintenanceCycleFailed:
	default:
		return fmt.Errorf("invalid maintenance cycle status %q", row.Status)
	}
	if strings.TrimSpace(row.SourceSystem) == "" {
		return fmt.Errorf("incomplete maintenance cycle")
	}
	return nil
}

func validateMaintenanceCheckpoint(row MaintenanceCheckpoint) error {
	if row.CycleID == 0 || row.VideoID == 0 || strings.TrimSpace(row.SourceSystem) == "" || strings.TrimSpace(row.SourceVideoID) == "" {
		return fmt.Errorf("incomplete maintenance checkpoint")
	}
	switch row.Mode {
	case MaintenanceModeIncremental, MaintenanceModeFull:
	default:
		return fmt.Errorf("invalid maintenance mode %q", row.Mode)
	}
	switch row.Status {
	case MaintenancePending, MaintenanceVideoCompleted, MaintenanceVideoFailed:
	default:
		return fmt.Errorf("invalid maintenance status %q", row.Status)
	}
	return nil
}
