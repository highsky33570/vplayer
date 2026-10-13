package store

import (
	"database/sql"
	"strings"
)

// AdminSyncOverview is a read-only operational snapshot for /admin/sync.
// It intentionally omits timer/worker/lock/enrichment-run claims.
type AdminSyncOverview struct {
	LatestSync               *AdminSyncRunSummary
	LatestIncremental        *AdminSyncRunSummary
	LatestFull               *AdminSyncRunSummary
	PendingEnrichmentCount   int
	VideosSyncErrorCount     int
	CategoriesSyncErrorCount int
	VideosActive             int
	VideosInactive           int
}

const adminSyncRunSelect = `
		SELECT id, source_system, trigger_type, started_at, finished_at,
		       scanned, created_count, updated_count, unchanged_count, failed_count, deactivated_count,
		       (error_text IS NOT NULL AND TRIM(error_text) <> '') AS has_error
		FROM sync_runs`

// AdminSyncOverviewStats returns aggregate cards for the Sync admin page.
func (m *MySQL) AdminSyncOverviewStats() (*AdminSyncOverview, error) {
	out := &AdminSyncOverview{}

	latest, err := m.latestAdminSyncRun(nil)
	if err != nil {
		return nil, err
	}
	out.LatestSync = latest

	inc := "incremental"
	latestInc, err := m.latestAdminSyncRun(&inc)
	if err != nil {
		return nil, err
	}
	out.LatestIncremental = latestInc

	latestFull, err := m.latestAdminSyncRunFull()
	if err != nil {
		return nil, err
	}
	out.LatestFull = latestFull

	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_episodes
		WHERE source_system = ?
		  AND LOWER(COALESCE(playback_status,'')) = 'pending_enrichment'
		  AND COALESCE(playback_url,'') = ''
		  AND COALESCE(play_page_url,'') <> ''`, "olehdtv").Scan(&out.PendingEnrichmentCount); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM videos
		WHERE LOWER(COALESCE(sync_status,'')) = 'error'`).Scan(&out.VideosSyncErrorCount); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM categories
		WHERE LOWER(COALESCE(sync_status,'')) = 'error'`).Scan(&out.CategoriesSyncErrorCount); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM videos WHERE COALESCE(is_active,1)=1`).Scan(&out.VideosActive); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM videos WHERE COALESCE(is_active,1)=0`).Scan(&out.VideosInactive); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *MySQL) latestAdminSyncRun(trigger *string) (*AdminSyncRunSummary, error) {
	q := adminSyncRunSelect + ` WHERE 1=1`
	args := []any{}
	if trigger != nil {
		q += ` AND trigger_type=?`
		args = append(args, *trigger)
	}
	q += ` ORDER BY started_at DESC, id DESC LIMIT 1`
	row := m.DB.QueryRow(q, args...)
	return scanAdminSyncRun(row)
}

func (m *MySQL) latestAdminSyncRunFull() (*AdminSyncRunSummary, error) {
	row := m.DB.QueryRow(adminSyncRunSelect + `
		WHERE trigger_type <> 'incremental'
		ORDER BY started_at DESC, id DESC
		LIMIT 1`)
	return scanAdminSyncRun(row)
}

// ListAdminSyncRunsFiltered lists sync_runs for admin history.
// triggerType empty means all. limit is clamped to 1..100.
func (m *MySQL) ListAdminSyncRunsFiltered(triggerType string, limit, offset int) ([]AdminSyncRunSummary, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := adminSyncRunSelect + ` WHERE 1=1`
	args := []any{}
	if triggerType = strings.TrimSpace(triggerType); triggerType != "" {
		q += ` AND trigger_type=?`
		args = append(args, triggerType)
	}
	q += ` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminSyncRunSummary
	for rows.Next() {
		s, err := scanAdminSyncRun(rows)
		if err != nil {
			return nil, err
		}
		if s != nil {
			out = append(out, *s)
		}
	}
	return out, rows.Err()
}

// CountAdminSyncRunsFiltered counts rows matching ListAdminSyncRunsFiltered filters.
func (m *MySQL) CountAdminSyncRunsFiltered(triggerType string) (int, error) {
	q := `SELECT COUNT(*) FROM sync_runs WHERE 1=1`
	args := []any{}
	if triggerType = strings.TrimSpace(triggerType); triggerType != "" {
		q += ` AND trigger_type=?`
		args = append(args, triggerType)
	}
	var n int
	if err := m.DB.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// GetAdminSyncRunByID returns one safe sync run summary, or nil if missing.
func (m *MySQL) GetAdminSyncRunByID(id uint64) (*AdminSyncRunSummary, error) {
	row := m.DB.QueryRow(adminSyncRunSelect+` WHERE id=? LIMIT 1`, id)
	return scanAdminSyncRun(row)
}

type adminSyncRunScannable interface {
	Scan(dest ...any) error
}

func scanAdminSyncRun(row adminSyncRunScannable) (*AdminSyncRunSummary, error) {
	var s AdminSyncRunSummary
	var finished sql.NullTime
	var hasError int
	err := row.Scan(
		&s.ID, &s.SourceSystem, &s.TriggerType, &s.StartedAt, &finished,
		&s.Scanned, &s.CreatedCount, &s.UpdatedCount, &s.UnchangedCount, &s.FailedCount, &s.DeactivatedCount,
		&hasError,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finished.Valid {
		t := finished.Time
		s.FinishedAt = &t
	}
	s.HasError = hasError != 0
	return &s, nil
}
