package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const IncrementalSyncLockName = "vplayer_olehdtv_incremental_sync"

// EpisodeSyncRow is the episode state needed for incremental change detection.
type EpisodeSyncRow struct {
	ID              uint64
	VideoID         uint64
	SID             int
	NID             int
	Title           string
	PlaybackSource  string
	PlaybackURL     string
	PlayPageURL     string
	PlaybackStatus  string
	HlsObjectKey    string
	StorageProvider string
	MigrationStatus string
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TryAdvisoryLock acquires a MySQL named lock (non-blocking when timeoutSec=0).
func (m *MySQL) TryAdvisoryLock(name string, timeoutSec int) (bool, error) {
	var got sql.NullInt64
	err := m.DB.QueryRow(`SELECT GET_LOCK(?, ?)`, name, timeoutSec).Scan(&got)
	if err != nil {
		return false, err
	}
	return got.Valid && got.Int64 == 1, nil
}

// ReleaseAdvisoryLock releases a MySQL named lock.
func (m *MySQL) ReleaseAdvisoryLock(name string) error {
	var n sql.NullInt64
	return m.DB.QueryRow(`SELECT RELEASE_LOCK(?)`, name).Scan(&n)
}

func (m *MySQL) GetEpisodeForSync(system, sourceVideoID string, sid, nid int) (*EpisodeSyncRow, error) {
	var e EpisodeSyncRow
	var active int
	err := m.DB.QueryRow(`
		SELECT id, video_id, sid, nid, title,
		       COALESCE(playback_source,''), COALESCE(playback_url,''), COALESCE(play_page_url,''),
		       COALESCE(playback_status,'ok'), COALESCE(hls_object_key,''), COALESCE(storage_provider,''),
		       COALESCE(migration_status,''), is_active, created_at, updated_at
		FROM video_episodes
		WHERE source_system=? AND source_video_id=? AND sid=? AND nid=?`,
		system, sourceVideoID, sid, nid).
		Scan(&e.ID, &e.VideoID, &e.SID, &e.NID, &e.Title,
			&e.PlaybackSource, &e.PlaybackURL, &e.PlayPageURL,
			&e.PlaybackStatus, &e.HlsObjectKey, &e.StorageProvider,
			&e.MigrationStatus, &active, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.IsActive = active == 1
	return &e, nil
}

// InsertVideoCatalog inserts a new video. created_at uses DB default.
func (m *MySQL) InsertVideoCatalog(in VideoUpsert) (uint64, error) {
	now := time.Now()
	if in.Status == "" {
		in.Status = "ready"
	}
	if in.SyncStatus == "" {
		in.SyncStatus = "ok"
	}
	coverKey := in.CoverKey
	if coverKey == "" {
		coverKey = in.CoverR2Key
	}
	res, err := m.DB.Exec(`
		INSERT INTO videos (
		  source_system, source_id, category_id, source_category_id, title, description,
		  year, area, director, actors, rating, cover_key, poster_source_url, cover_r2_key,
		  duration_sec, view_count, hls_master_key, playback_source, play_sid, play_nid,
		  playback_url, source_updated_at, last_synced_at, sync_status, sync_error, is_active, status
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.SourceSystem, in.SourceID, in.CategoryID, in.SourceCategoryID, in.Title, in.Description,
		in.Year, in.Area, in.Director, in.Actors, in.Rating, coverKey, in.PosterSourceURL, in.CoverR2Key,
		in.DurationSec, in.ViewCount, in.PlaybackURL, in.PlaybackSource, in.PlaySID, in.PlayNID,
		in.PlaybackURL, in.SourceUpdatedAt, now, in.SyncStatus, nullStr(in.SyncError), boolToInt(in.IsActive), in.Status,
	)
	if err != nil {
		return 0, err
	}
	lid, _ := res.LastInsertId()
	return uint64(lid), nil
}

// UpdateVideoCatalog updates catalog fields only when called (caller must detect changes).
// Does not modify created_at.
func (m *MySQL) UpdateVideoCatalog(id uint64, in VideoUpsert) error {
	now := time.Now()
	if in.Status == "" {
		in.Status = "ready"
	}
	if in.SyncStatus == "" {
		in.SyncStatus = "ok"
	}
	coverKey := in.CoverKey
	if coverKey == "" {
		coverKey = in.CoverR2Key
	}
	_, err := m.DB.Exec(`
		UPDATE videos SET
		  category_id=?, source_category_id=?, title=?, description=?,
		  year=?, area=?, director=?, actors=?, rating=?,
		  cover_key=?, poster_source_url=?, cover_r2_key=?,
		  duration_sec=?, view_count=?, hls_master_key=?,
		  playback_source=?, play_sid=?, play_nid=?, playback_url=?,
		  source_updated_at=?, last_synced_at=?, sync_status=?, sync_error=?,
		  is_active=?, status=?
		WHERE id=?`,
		in.CategoryID, in.SourceCategoryID, in.Title, in.Description,
		in.Year, in.Area, in.Director, in.Actors, in.Rating,
		coverKey, in.PosterSourceURL, in.CoverR2Key,
		in.DurationSec, in.ViewCount, in.PlaybackURL,
		in.PlaybackSource, in.PlaySID, in.PlayNID, in.PlaybackURL,
		in.SourceUpdatedAt, now, in.SyncStatus, nullStr(in.SyncError),
		boolToInt(in.IsActive), in.Status, id,
	)
	return err
}

// InsertEpisodeCatalog inserts a new episode. Migration fields stay at defaults.
func (m *MySQL) InsertEpisodeCatalog(in EpisodeUpsert) error {
	now := time.Now()
	status := in.PlaybackStatus
	if status == "" {
		status = "ok"
	}
	_, err := m.DB.Exec(`
		INSERT INTO video_episodes (
		  video_id, source_system, source_video_id, sid, nid, title,
		  playback_source, playback_url, play_page_url, playback_status,
		  is_active, last_synced_at, sync_status
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?, 'ok')`,
		in.VideoID, in.SourceSystem, in.SourceVideoID, in.SID, in.NID, in.Title,
		in.PlaybackSource, in.PlaybackURL, in.PlayPageURL, status,
		boolToInt(in.IsActive), now,
	)
	return err
}

// UpdateEpisodeCatalog updates source/catalog episode fields only.
// Never touches hls_object_key, storage_provider, migration_status, migrated_at.
func (m *MySQL) UpdateEpisodeCatalog(id uint64, in EpisodeUpsert) error {
	now := time.Now()
	status := in.PlaybackStatus
	if status == "" {
		status = "ok"
	}
	_, err := m.DB.Exec(`
		UPDATE video_episodes SET
		  video_id=?, title=?, playback_source=?, playback_url=?,
		  play_page_url=?, playback_status=?, is_active=?,
		  last_synced_at=?, sync_status='ok'
		WHERE id=?`,
		in.VideoID, in.Title, in.PlaybackSource, in.PlaybackURL,
		in.PlayPageURL, status, boolToInt(in.IsActive),
		now, id,
	)
	return err
}

// InsertSyncRunDetailed writes a sync_runs row with arbitrary summary JSON.
func (m *MySQL) InsertSyncRunDetailed(system, trigger string, scanned, created, updated, unchanged, failed, deactivated int, summary any, errText string) error {
	b, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = m.DB.Exec(`
		INSERT INTO sync_runs (
		  source_system, trigger_type, started_at, finished_at,
		  scanned, created_count, updated_count, unchanged_count, failed_count, deactivated_count,
		  summary_json, error_text
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		system, trigger, now, now,
		scanned, created, updated, unchanged, failed, deactivated,
		string(b), nullStr(errText),
	)
	return err
}

// GetVideoUpdatedAt is used by tests / verification.
func (m *MySQL) GetVideoTimestamps(id uint64) (createdAt, updatedAt time.Time, err error) {
	err = m.DB.QueryRow(`SELECT created_at, updated_at FROM videos WHERE id=?`, id).Scan(&createdAt, &updatedAt)
	return
}

func (m *MySQL) GetEpisodeMigrationFields(id uint64) (hlsKey, provider, status string, err error) {
	err = m.DB.QueryRow(`
		SELECT COALESCE(hls_object_key,''), COALESCE(storage_provider,''), COALESCE(migration_status,'')
		FROM video_episodes WHERE id=?`, id).Scan(&hlsKey, &provider, &status)
	if err == sql.ErrNoRows {
		return "", "", "", fmt.Errorf("episode %d not found", id)
	}
	return
}
