package store

import (
	"strings"
)

const adminOperationalRunSelect = `
		SELECT id, job_type, trigger_type, status, requested_by_user_id,
		       requested_at, started_at, finished_at,
		       scanned, created_count, updated_count, unchanged_count, failed_count,
		       selected_count, succeeded_count, skipped_count, unavailable_count,
		       COALESCE(error_code,''), has_error, sync_run_id
		FROM operational_runs`

// ListAdminOperationalRunsFiltered lists operational_runs for admin history.
// Empty filter strings mean all. Order: started_at DESC, id DESC.
func (m *MySQL) ListAdminOperationalRunsFiltered(jobType, triggerType, status string, limit, offset int) ([]OperationalRunRow, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := adminOperationalRunSelect + ` WHERE 1=1`
	args := []any{}
	if jobType = strings.TrimSpace(jobType); jobType != "" {
		q += ` AND job_type=?`
		args = append(args, jobType)
	}
	if triggerType = strings.TrimSpace(triggerType); triggerType != "" {
		q += ` AND trigger_type=?`
		args = append(args, triggerType)
	}
	if status = strings.TrimSpace(status); status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	q += ` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OperationalRunRow
	for rows.Next() {
		r, err := scanOperationalRun(rows)
		if err != nil {
			return nil, err
		}
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, rows.Err()
}

// CountAdminOperationalRunsFiltered counts rows matching ListAdminOperationalRunsFiltered filters.
func (m *MySQL) CountAdminOperationalRunsFiltered(jobType, triggerType, status string) (int, error) {
	q := `SELECT COUNT(*) FROM operational_runs WHERE 1=1`
	args := []any{}
	if jobType = strings.TrimSpace(jobType); jobType != "" {
		q += ` AND job_type=?`
		args = append(args, jobType)
	}
	if triggerType = strings.TrimSpace(triggerType); triggerType != "" {
		q += ` AND trigger_type=?`
		args = append(args, triggerType)
	}
	if status = strings.TrimSpace(status); status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	var n int
	if err := m.DB.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// GetAdminOperationalRunByID returns one operational run, or nil if missing.
func (m *MySQL) GetAdminOperationalRunByID(id uint64) (*OperationalRunRow, error) {
	return m.GetOperationalRunByID(id)
}
