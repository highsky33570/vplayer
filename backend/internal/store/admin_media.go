package store

import (
	"database/sql"
	"strings"
	"time"
)

// AdminMediaOverview is a read-only media migration snapshot.
type AdminMediaOverview struct {
	EpisodesTotal    int
	MigratedCount    int
	FailedCount      int
	NotMigratedCount int
	EligibleCount    int
	StorageR2Count   int
	LatestMigratedAt *time.Time
}

// AdminMediaEpisode is a safe admin view of one episode (no URLs/keys/errors).
type AdminMediaEpisode struct {
	EpisodeID       uint64
	VideoID         uint64
	VideoTitle      string
	SID             int
	NID             int
	EpisodeTitle    string
	StorageProvider string
	MigrationStatus string
	MigratedAt      *time.Time
	HasError        bool
	PlaybackStatus  string
	HasHLSObject    bool
}

// Eligibility matches ListMigratableEpisodes (without per-run LIMIT).
const adminMediaEligibleWhere = `
		e.is_active=1
		AND COALESCE(v.is_active,1)=1
		AND v.status IN ('ready','demo')
		AND TRIM(e.playback_url) <> ''
		AND LOWER(COALESCE(e.playback_status,'ok')) IN ('ok','available','')
		AND LOWER(COALESCE(e.migration_status,'')) NOT IN ('done','migrated')
		AND LOWER(COALESCE(e.playback_status,'')) NOT IN ('unavailable','member','vip','restricted','forbidden')`

const adminMediaEpisodeSelect = `
		SELECT e.id, e.video_id, COALESCE(v.title,''), e.sid, e.nid, COALESCE(e.title,''),
		       COALESCE(e.storage_provider,''), COALESCE(e.migration_status,''), e.migrated_at,
		       (e.migration_error IS NOT NULL AND TRIM(e.migration_error) <> '') AS has_error,
		       COALESCE(e.playback_status,'ok'),
		       (TRIM(COALESCE(e.hls_object_key,'')) <> '') AS has_hls_object
		FROM video_episodes e
		INNER JOIN videos v ON v.id = e.video_id`

// AdminMediaOverviewStats returns aggregate media migration counts.
func (m *MySQL) AdminMediaOverviewStats() (*AdminMediaOverview, error) {
	out := &AdminMediaOverview{}
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM video_episodes`).Scan(&out.EpisodesTotal); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_episodes
		WHERE LOWER(COALESCE(migration_status,'')) IN ('done','migrated')`).Scan(&out.MigratedCount); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_episodes
		WHERE LOWER(COALESCE(migration_status,'')) = 'failed'`).Scan(&out.FailedCount); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_episodes
		WHERE LOWER(COALESCE(migration_status,'')) NOT IN ('done','migrated','failed')`).Scan(&out.NotMigratedCount); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_episodes e
		INNER JOIN videos v ON v.id = e.video_id
		WHERE ` + adminMediaEligibleWhere).Scan(&out.EligibleCount); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_episodes
		WHERE LOWER(COALESCE(storage_provider,'')) = 'r2'`).Scan(&out.StorageR2Count); err != nil {
		return nil, err
	}
	var latest sql.NullTime
	if err := m.DB.QueryRow(`SELECT MAX(migrated_at) FROM video_episodes`).Scan(&latest); err != nil {
		return nil, err
	}
	if latest.Valid {
		t := latest.Time
		out.LatestMigratedAt = &t
	}
	return out, nil
}

// ListAdminMediaEpisodesFiltered lists episodes for admin media with filters.
// migrationStatus: ""=all, "done"=done|migrated, "failed", "pending"=not done/migrated/failed.
func (m *MySQL) ListAdminMediaEpisodesFiltered(migrationStatus, storageProvider, query string, limit, offset int) ([]AdminMediaEpisode, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := adminMediaEpisodeSelect + ` WHERE 1=1`
	args := []any{}
	q, args = appendAdminMediaFilters(q, args, migrationStatus, storageProvider, query)
	q += ` ORDER BY e.id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminMediaEpisode
	for rows.Next() {
		ep, err := scanAdminMediaEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ep)
	}
	return out, rows.Err()
}

// CountAdminMediaEpisodesFiltered counts episodes matching list filters.
func (m *MySQL) CountAdminMediaEpisodesFiltered(migrationStatus, storageProvider, query string) (int, error) {
	q := `
		SELECT COUNT(*) FROM video_episodes e
		INNER JOIN videos v ON v.id = e.video_id
		WHERE 1=1`
	args := []any{}
	q, args = appendAdminMediaFilters(q, args, migrationStatus, storageProvider, query)
	var n int
	if err := m.DB.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// GetAdminMediaEpisodeByID returns one safe episode view, or nil if missing.
func (m *MySQL) GetAdminMediaEpisodeByID(id uint64) (*AdminMediaEpisode, error) {
	row := m.DB.QueryRow(adminMediaEpisodeSelect+` WHERE e.id=? LIMIT 1`, id)
	return scanAdminMediaEpisode(row)
}

func appendAdminMediaFilters(q string, args []any, migrationStatus, storageProvider, query string) (string, []any) {
	switch strings.ToLower(strings.TrimSpace(migrationStatus)) {
	case "done":
		q += ` AND LOWER(COALESCE(e.migration_status,'')) IN ('done','migrated')`
	case "failed":
		q += ` AND LOWER(COALESCE(e.migration_status,'')) = 'failed'`
	case "pending":
		q += ` AND LOWER(COALESCE(e.migration_status,'')) NOT IN ('done','migrated','failed')`
	}

	if storageProvider = strings.TrimSpace(storageProvider); storageProvider != "" {
		q += ` AND LOWER(COALESCE(e.storage_provider,'')) = LOWER(?)`
		args = append(args, storageProvider)
	}

	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND (v.title LIKE ? OR COALESCE(e.title,'') LIKE ?)`
		args = append(args, like, like)
	}
	return q, args
}

type adminMediaEpisodeScannable interface {
	Scan(dest ...any) error
}

func scanAdminMediaEpisode(row adminMediaEpisodeScannable) (*AdminMediaEpisode, error) {
	var ep AdminMediaEpisode
	var migrated sql.NullTime
	var hasError, hasHLS int
	err := row.Scan(
		&ep.EpisodeID, &ep.VideoID, &ep.VideoTitle, &ep.SID, &ep.NID, &ep.EpisodeTitle,
		&ep.StorageProvider, &ep.MigrationStatus, &migrated,
		&hasError, &ep.PlaybackStatus, &hasHLS,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if migrated.Valid {
		t := migrated.Time
		ep.MigratedAt = &t
	}
	ep.HasError = hasError != 0
	ep.HasHLSObject = hasHLS != 0
	return &ep, nil
}
