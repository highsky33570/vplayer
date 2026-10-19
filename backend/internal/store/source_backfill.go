package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Backfill checkpoint statuses. Pending means a video was started and did not finish.
const (
	BackfillPending   = "pending"
	BackfillCompleted = "completed"
	BackfillFailed    = "failed"
	BackfillModeFull  = "full"
)

// SourceVideoCandidate is one active local video eligible for comment import.
type SourceVideoCandidate struct {
	ID           uint64
	SourceSystem string
	SourceID     string
	Title        string
	Active       bool
}

// BackfillCheckpoint is the resume row for one video and one backfill mode.
type BackfillCheckpoint struct {
	SourceSystem  string
	Mode          string
	VideoID       uint64
	SourceVideoID string
	Status        string
	PagesFetched  int
	Discovered    int
	Parents       int
	Replies       int
	Inserted      int
	Updated       int
	Skipped       int
	ErrorMessage  string
	StartedAt     time.Time
	CompletedAt   time.Time
}

const listActiveSourceVideosSQL = `
	SELECT id, source_system, source_id, COALESCE(title,'')
	FROM videos
	WHERE source_system = ?
	  AND source_id IS NOT NULL AND source_id <> ''
	  AND COALESCE(is_active,1) = 1
	ORDER BY id ASC`

const listBackfillCheckpointsSQL = `
	SELECT video_id, source_video_id, status, pages_fetched, comments_discovered,
	       parents, replies, inserted_count, updated_count, skipped_count,
	       COALESCE(error_message,''),
	       DATE_FORMAT(started_at, '%Y-%m-%d %H:%i:%s'),
	       DATE_FORMAT(completed_at, '%Y-%m-%d %H:%i:%s')
	FROM source_comment_backfill
	WHERE source_system=? AND mode=?
	ORDER BY video_id ASC`

const saveBackfillCheckpointSQL = `
	INSERT INTO source_comment_backfill (
		source_system, mode, video_id, source_video_id, status,
		pages_fetched, comments_discovered, parents, replies,
		inserted_count, updated_count, skipped_count, error_message,
		started_at, completed_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		source_video_id=VALUES(source_video_id),
		status=VALUES(status),
		pages_fetched=VALUES(pages_fetched),
		comments_discovered=VALUES(comments_discovered),
		parents=VALUES(parents),
		replies=VALUES(replies),
		inserted_count=VALUES(inserted_count),
		updated_count=VALUES(updated_count),
		skipped_count=VALUES(skipped_count),
		error_message=VALUES(error_message),
		started_at=VALUES(started_at),
		completed_at=VALUES(completed_at),
		updated_at=VALUES(updated_at)`

const listPendingBackfillSQL = `
	SELECT v.id, v.source_system, v.source_id, COALESCE(v.title,'')
	FROM videos v
	LEFT JOIN source_comment_backfill b
	  ON b.source_system = v.source_system AND b.video_id = v.id AND b.mode = ?
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1
	  AND v.id > ?
	  AND (b.video_id IS NULL OR b.status = 'pending')
	ORDER BY v.id ASC
	LIMIT ?`

const listFailedBackfillSQL = `
	SELECT v.id, v.source_system, v.source_id, COALESCE(v.title,'')
	FROM videos v
	INNER JOIN source_comment_backfill b
	  ON b.source_system = v.source_system AND b.video_id = v.id AND b.mode = ? AND b.status = 'failed'
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1
	ORDER BY v.id ASC`

const firstFailedBackfillSQL = `
	SELECT video_id, source_video_id, COALESCE(error_message,'')
	FROM source_comment_backfill
	WHERE source_system=? AND mode=? AND status='failed'
	ORDER BY video_id ASC
	LIMIT 1`

const countBackfillStatusSQL = `
	SELECT CAST(COALESCE(SUM(status='completed'),0) AS SIGNED),
	       CAST(COALESCE(SUM(status='failed'),0) AS SIGNED)
	FROM source_comment_backfill
	WHERE source_system=? AND mode=?`

const countRemainingBackfillSQL = `
	SELECT COUNT(*)
	FROM videos v
	WHERE v.source_system = ?
	  AND v.source_id IS NOT NULL AND v.source_id <> ''
	  AND COALESCE(v.is_active,1) = 1
	  AND NOT EXISTS (
	    SELECT 1 FROM source_comment_backfill b
	    WHERE b.source_system = v.source_system
	      AND b.video_id = v.id
	      AND b.mode = ?
	      AND b.status = 'completed'
	  )`

const countSourceCommentRowsSQL = `
	SELECT CAST(COALESCE(SUM(parent_source_comment_id IS NULL OR parent_source_comment_id=''),0) AS SIGNED),
	       CAST(COALESCE(SUM(parent_source_comment_id IS NOT NULL AND parent_source_comment_id<>''),0) AS SIGNED)
	FROM video_source_comments
	WHERE video_id=? AND source_system=?`

// ListActiveSourceVideos returns OLEHDTV (or another named system) videos in id order.
// Inactive videos and rows without a source id are excluded. Catalog status is not read or written.
func (m *MySQL) ListActiveSourceVideos(sourceSystem string) ([]SourceVideoCandidate, error) {
	sourceSystem = strings.TrimSpace(sourceSystem)
	if sourceSystem == "" {
		return nil, errMissingSystem
	}
	rows, err := m.DB.Query(listActiveSourceVideosSQL, sourceSystem)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceVideoCandidate
	for rows.Next() {
		var row SourceVideoCandidate
		if err := rows.Scan(&row.ID, &row.SourceSystem, &row.SourceID, &row.Title); err != nil {
			return nil, err
		}
		row.Active = true
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListBackfillCheckpoints loads every resume row for one system and mode.
func (m *MySQL) ListBackfillCheckpoints(sourceSystem, mode string) ([]BackfillCheckpoint, error) {
	rows, err := m.DB.Query(listBackfillCheckpointsSQL, strings.TrimSpace(sourceSystem), strings.TrimSpace(mode))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackfillCheckpoint
	for rows.Next() {
		var row BackfillCheckpoint
		var started, completed sql.NullString
		if err := rows.Scan(
			&row.VideoID, &row.SourceVideoID, &row.Status, &row.PagesFetched, &row.Discovered,
			&row.Parents, &row.Replies, &row.Inserted, &row.Updated, &row.Skipped,
			&row.ErrorMessage, &started, &completed,
		); err != nil {
			return nil, err
		}
		row.SourceSystem = sourceSystem
		row.Mode = mode
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
		out = append(out, row)
	}
	return out, rows.Err()
}

// SaveBackfillCheckpoint upserts one video's resume row. It does not write videos or video_comments.
func (m *MySQL) SaveBackfillCheckpoint(row BackfillCheckpoint, now time.Time) error {
	if err := validateBackfillCheckpoint(row); err != nil {
		return err
	}
	now = now.UTC()
	stamp := formatUTCDateTime(now)
	var started any
	if !row.StartedAt.IsZero() {
		started = formatUTCDateTime(row.StartedAt)
	}
	var completed any
	if !row.CompletedAt.IsZero() {
		completed = formatUTCDateTime(row.CompletedAt)
	}
	_, err := m.DB.Exec(saveBackfillCheckpointSQL,
		row.SourceSystem, row.Mode, row.VideoID, row.SourceVideoID, row.Status,
		row.PagesFetched, row.Discovered, row.Parents, row.Replies,
		row.Inserted, row.Updated, row.Skipped, nullIfEmpty(truncateRunes(row.ErrorMessage, 512)),
		started, completed, stamp, stamp,
	)
	return err
}

// ListPendingBackfillVideos returns the next active videos that are not completed
// or failed, in id order, after afterID. The LIMIT keeps a drain from loading the catalog.
func (m *MySQL) ListPendingBackfillVideos(sourceSystem, mode string, afterID uint64, limit int) ([]SourceVideoCandidate, error) {
	if limit < 1 {
		return nil, fmt.Errorf("invalid backfill page size")
	}
	rows, err := m.DB.Query(listPendingBackfillSQL, strings.TrimSpace(mode), strings.TrimSpace(sourceSystem), afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSourceVideoCandidates(rows)
}

// ListFailedBackfillVideos returns active videos whose checkpoint is failed, in id order.
func (m *MySQL) ListFailedBackfillVideos(sourceSystem, mode string) ([]SourceVideoCandidate, error) {
	rows, err := m.DB.Query(listFailedBackfillSQL, strings.TrimSpace(mode), strings.TrimSpace(sourceSystem))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSourceVideoCandidates(rows)
}

// FirstFailedBackfill returns the lowest-id failed checkpoint, including an inactive video.
func (m *MySQL) FirstFailedBackfill(sourceSystem, mode string) (*BackfillCheckpoint, error) {
	var row BackfillCheckpoint
	err := m.DB.QueryRow(firstFailedBackfillSQL, strings.TrimSpace(sourceSystem), strings.TrimSpace(mode)).Scan(
		&row.VideoID, &row.SourceVideoID, &row.ErrorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.SourceSystem = sourceSystem
	row.Mode = mode
	row.Status = BackfillFailed
	return &row, nil
}

// CountBackfillCheckpointStatus counts completed and failed checkpoint rows.
func (m *MySQL) CountBackfillCheckpointStatus(sourceSystem, mode string) (completed, failed int, err error) {
	err = m.DB.QueryRow(countBackfillStatusSQL, strings.TrimSpace(sourceSystem), strings.TrimSpace(mode)).Scan(&completed, &failed)
	return completed, failed, err
}

// CountRemainingBackfill counts active videos that do not yet have a completed checkpoint.
func (m *MySQL) CountRemainingBackfill(sourceSystem, mode string) (int, error) {
	var n int
	err := m.DB.QueryRow(countRemainingBackfillSQL, strings.TrimSpace(sourceSystem), strings.TrimSpace(mode)).Scan(&n)
	return n, err
}

func scanSourceVideoCandidates(rows *sql.Rows) ([]SourceVideoCandidate, error) {
	var out []SourceVideoCandidate
	for rows.Next() {
		var row SourceVideoCandidate
		if err := rows.Scan(&row.ID, &row.SourceSystem, &row.SourceID, &row.Title); err != nil {
			return nil, err
		}
		row.Active = true
		out = append(out, row)
	}
	return out, rows.Err()
}

// CountSourceCommentRows counts stored parents and replies for one local video.
func (m *MySQL) CountSourceCommentRows(videoID uint64, sourceSystem string) (parents, replies int, err error) {
	err = m.DB.QueryRow(countSourceCommentRowsSQL, videoID, strings.TrimSpace(sourceSystem)).Scan(&parents, &replies)
	return parents, replies, err
}

func validateBackfillCheckpoint(row BackfillCheckpoint) error {
	switch row.Status {
	case BackfillPending, BackfillCompleted, BackfillFailed:
	default:
		return fmt.Errorf("invalid backfill status %q", row.Status)
	}
	if row.VideoID == 0 || strings.TrimSpace(row.SourceSystem) == "" || strings.TrimSpace(row.Mode) == "" {
		return fmt.Errorf("incomplete backfill checkpoint")
	}
	if strings.TrimSpace(row.SourceVideoID) == "" {
		return fmt.Errorf("incomplete backfill checkpoint")
	}
	return nil
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
