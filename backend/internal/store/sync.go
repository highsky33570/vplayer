package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

type CategoryUpsert struct {
	SourceSystem string
	SourceID     string
	Name         string
	Slug         string
	Sort         int
	IsActive     bool
}

type VideoUpsert struct {
	SourceSystem     string
	SourceID         string
	CategoryID       uint64
	SourceCategoryID string
	Title            string
	Description      string
	Year             string
	Area             string
	Director         string
	Actors           string
	Rating           float64
	PosterSourceURL  string
	CoverR2Key       string
	CoverKey         string
	DurationSec      int
	ViewCount        int64
	PlaybackSource   string
	PlaySID          int
	PlayNID          int
	PlaybackURL      string
	SourceUpdatedAt  *time.Time
	IsActive         bool
	Status           string
	SyncStatus       string
	SyncError        string
}

type EpisodeUpsert struct {
	VideoID         uint64
	SourceSystem    string
	SourceVideoID   string
	SID             int
	NID             int
	Title           string
	PlaybackSource  string
	PlaybackURL     string
	PlayPageURL     string
	PlaybackStatus  string
	IsActive        bool
}

func (m *MySQL) GetCategoryBySource(system, sourceID string) (*model.Category, error) {
	var c model.Category
	var srcSys, srcID sql.NullString
	var last sql.NullTime
	var active int
	err := m.DB.QueryRow(`
		SELECT id, name, slug, sort, COALESCE(source_system,''), COALESCE(source_id,''),
		       is_active, last_synced_at, created_at
		FROM categories WHERE source_system = ? AND source_id = ?`, system, sourceID).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Sort, &srcSys, &srcID, &active, &last, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.SourceSystem = srcSys.String
	c.SourceID = srcID.String
	c.IsActive = active == 1
	if last.Valid {
		t := last.Time
		c.LastSyncedAt = &t
	}
	return &c, nil
}

func (m *MySQL) GetCategoryBySlug(slug string) (*model.Category, error) {
	var c model.Category
	var srcSys, srcID sql.NullString
	var last sql.NullTime
	var active int
	err := m.DB.QueryRow(`
		SELECT id, name, slug, sort, COALESCE(source_system,''), COALESCE(source_id,''),
		       is_active, last_synced_at, created_at
		FROM categories WHERE slug = ? LIMIT 1`, slug).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Sort, &srcSys, &srcID, &active, &last, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.SourceSystem = srcSys.String
	c.SourceID = srcID.String
	c.IsActive = active == 1
	if last.Valid {
		t := last.Time
		c.LastSyncedAt = &t
	}
	return &c, nil
}

// ReassignVideosCategory moves videos from one category to another.
func (m *MySQL) ReassignVideosCategory(fromID, toID uint64) error {
	if fromID == 0 || toID == 0 || fromID == toID {
		return nil
	}
	_, err := m.DB.Exec(`UPDATE videos SET category_id=? WHERE category_id=?`, toID, fromID)
	return err
}

// UpsertCategoryResult returns id and whether the row was newly created or content-changed.
// Prefer merging into an existing seed category with the same slug (e.g. movie)
// instead of creating duplicates like movie-1.
func (m *MySQL) UpsertCategoryResult(in CategoryUpsert) (id uint64, created, updated bool, err error) {
	now := time.Now()
	active := boolToInt(in.IsActive)

	existing, err := m.GetCategoryBySource(in.SourceSystem, in.SourceID)
	if err != nil {
		return 0, false, false, err
	}

	// Prefer canonical slug row (seed "movie") over previous duplicate "movie-1".
	canonical, err := m.GetCategoryBySlug(in.Slug)
	if err != nil {
		return 0, false, false, err
	}

	if existing != nil && canonical != nil && existing.ID != canonical.ID {
		if err := m.ReassignVideosCategory(existing.ID, canonical.ID); err != nil {
			return 0, false, false, err
		}
		// Clear source keys before claiming the canonical row (unique source constraint).
		_, _ = m.DB.Exec(`
			UPDATE categories SET source_system=NULL, source_id=NULL, is_active=0, sync_status='merged'
			WHERE id=?`, existing.ID)
		existing = nil
	}

	if existing == nil && canonical != nil {
		_, err = m.DB.Exec(`
			UPDATE categories SET name=?, sort=?, source_system=?, source_id=?,
			  last_synced_at=?, sync_status='ok', is_active=?
			WHERE id=?`,
			in.Name, in.Sort, in.SourceSystem, in.SourceID, now, active, canonical.ID)
		if err != nil {
			return 0, false, false, err
		}
		return canonical.ID, false, true, nil
	}

	if existing == nil {
		res, err := m.DB.Exec(`
			INSERT INTO categories (name, slug, sort, source_system, source_id, last_synced_at, sync_status, is_active)
			VALUES (?, ?, ?, ?, ?, ?, 'ok', ?)`,
			in.Name, in.Slug, in.Sort, in.SourceSystem, in.SourceID, now, active)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			// Race: slug claimed — attach source to that row.
			if again, e2 := m.GetCategoryBySlug(in.Slug); e2 == nil && again != nil {
				_, err = m.DB.Exec(`
					UPDATE categories SET name=?, sort=?, source_system=?, source_id=?,
					  last_synced_at=?, sync_status='ok', is_active=?
					WHERE id=?`,
					in.Name, in.Sort, in.SourceSystem, in.SourceID, now, active, again.ID)
				return again.ID, false, true, err
			}
			return 0, false, false, err
		}
		if err != nil {
			return 0, false, false, err
		}
		lid, _ := res.LastInsertId()
		return uint64(lid), true, false, nil
	}

	changed := existing.Name != in.Name || existing.Sort != in.Sort || existing.IsActive != in.IsActive
	_, err = m.DB.Exec(`
		UPDATE categories SET name=?, sort=?, last_synced_at=?, sync_status='ok', is_active=?
		WHERE id=?`, in.Name, in.Sort, now, active, existing.ID)
	return existing.ID, false, changed, err
}

func (m *MySQL) GetVideoBySource(system, sourceID string) (*model.Video, error) {
	row := m.DB.QueryRow(`
		SELECT id, COALESCE(source_system,''), COALESCE(source_id,''), category_id, COALESCE(source_category_id,''),
		       title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		       COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		       duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), play_sid, play_nid,
		       COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		       is_active, status, created_at
		FROM videos WHERE source_system = ? AND source_id = ?`, system, sourceID)
	return scanVideo(row)
}

func (m *MySQL) UpsertVideoResult(in VideoUpsert) (id uint64, created, updated bool, err error) {
	existing, err := m.GetVideoBySource(in.SourceSystem, in.SourceID)
	if err != nil {
		return 0, false, false, err
	}
	now := time.Now()
	if in.Status == "" {
		in.Status = "ready"
	}
	if in.SyncStatus == "" {
		in.SyncStatus = "ok"
	}
	active := boolToInt(in.IsActive)
	coverKey := in.CoverKey
	if coverKey == "" {
		coverKey = in.CoverR2Key
	}
	if existing == nil {
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
			in.PlaybackURL, in.SourceUpdatedAt, now, in.SyncStatus, nullStr(in.SyncError), active, in.Status,
		)
		if err != nil {
			return 0, false, false, err
		}
		lid, _ := res.LastInsertId()
		return uint64(lid), true, false, nil
	}

	changed := existing.Title != in.Title ||
		existing.Description != in.Description ||
		existing.CategoryID != in.CategoryID ||
		existing.Year != in.Year ||
		existing.Area != in.Area ||
		existing.Director != in.Director ||
		existing.Actors != in.Actors ||
		existing.Rating != in.Rating ||
		existing.PosterSourceURL != in.PosterSourceURL ||
		existing.PlaybackURL != in.PlaybackURL ||
		existing.PlaybackSource != in.PlaybackSource ||
		existing.PlaySID != in.PlaySID ||
		existing.PlayNID != in.PlayNID ||
		existing.ViewCount != in.ViewCount ||
		existing.IsActive != in.IsActive

	if in.CoverR2Key == "" {
		in.CoverR2Key = existing.CoverR2Key
	}
	if coverKey == "" {
		coverKey = existing.CoverKey
		if coverKey == "" {
			coverKey = existing.CoverR2Key
		}
	}

	_, err = m.DB.Exec(`
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
		active, in.Status, existing.ID,
	)
	return existing.ID, false, changed, err
}

func (m *MySQL) UpdateVideoPoster(id uint64, r2Key, posterURL string) error {
	_, err := m.DB.Exec(`
		UPDATE videos SET cover_key=?, cover_r2_key=?, poster_source_url=?, last_synced_at=?
		WHERE id=?`, r2Key, r2Key, posterURL, time.Now(), id)
	return err
}

func (m *MySQL) MarkVideoSyncError(id uint64, msg string) error {
	_, err := m.DB.Exec(`UPDATE videos SET sync_status='error', sync_error=?, last_synced_at=? WHERE id=?`,
		msg, time.Now(), id)
	return err
}

func (m *MySQL) UpsertEpisode(in EpisodeUpsert) error {
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
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?, 'ok')
		ON DUPLICATE KEY UPDATE
		  video_id=VALUES(video_id), title=VALUES(title),
		  playback_source=VALUES(playback_source), playback_url=VALUES(playback_url),
		  play_page_url=VALUES(play_page_url), playback_status=VALUES(playback_status),
		  is_active=VALUES(is_active), last_synced_at=VALUES(last_synced_at), sync_status='ok'`,
		in.VideoID, in.SourceSystem, in.SourceVideoID, in.SID, in.NID, in.Title,
		in.PlaybackSource, in.PlaybackURL, in.PlayPageURL, status,
		boolToInt(in.IsActive), now,
	)
	return err
}

func (m *MySQL) TouchVideoSeen(id uint64) error {
	_, err := m.DB.Exec(`UPDATE videos SET last_seen_at=? WHERE id=?`, time.Now(), id)
	return err
}

func (m *MySQL) CountReadyVideos(categoryID uint64) (int, error) {
	return m.CountReadyVideosFiltered(categoryID, "")
}

func (m *MySQL) CountReadyVideosFiltered(categoryID uint64, query string) (int, error) {
	q := `SELECT COUNT(*) FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	args := []any{}
	if categoryID > 0 {
		q += ` AND category_id=?`
		args = append(args, categoryID)
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND (title LIKE ? OR COALESCE(description,'') LIKE ? OR COALESCE(actors,'') LIKE ?)`
		args = append(args, like, like, like)
	}
	var n int
	err := m.DB.QueryRow(q, args...).Scan(&n)
	return n, err
}

func (m *MySQL) DeactivateMissingVideos(system string, keepSourceIDs []string) (int, error) {
	if len(keepSourceIDs) == 0 {
		res, err := m.DB.Exec(`UPDATE videos SET is_active=0, status='disabled' WHERE source_system=? AND is_active=1`, system)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	// Build NOT IN list safely with placeholders.
	args := make([]any, 0, len(keepSourceIDs)+1)
	args = append(args, system)
	ph := make([]byte, 0, len(keepSourceIDs)*2)
	for i, id := range keepSourceIDs {
		if i > 0 {
			ph = append(ph, ',')
		}
		ph = append(ph, '?')
		args = append(args, id)
	}
	q := fmt.Sprintf(`UPDATE videos SET is_active=0, status='disabled'
		WHERE source_system=? AND is_active=1 AND source_id NOT IN (%s)`, string(ph))
	res, err := m.DB.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (m *MySQL) InsertSyncRun(system, trigger string, sum model.SyncSummary, errText string) error {
	b, _ := json.Marshal(sum)
	now := time.Now()
	_, err := m.DB.Exec(`
		INSERT INTO sync_runs (
		  source_system, trigger_type, started_at, finished_at,
		  scanned, created_count, updated_count, unchanged_count, failed_count, deactivated_count,
		  summary_json, error_text
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		system, trigger, now, now,
		sum.Scanned, sum.Created, sum.Updated, sum.Unchanged, sum.Failed, sum.Deactivated,
		string(b), nullStr(errText),
	)
	return err
}

func (m *MySQL) GetEpisodeBySource(system, sourceVideoID string, sid, nid int) (*model.Episode, error) {
	var e model.Episode
	var active int
	err := m.DB.QueryRow(`
		SELECT id, video_id, sid, nid, title, playback_source, playback_url, is_active
		FROM video_episodes
		WHERE source_system=? AND source_video_id=? AND sid=? AND nid=?`,
		system, sourceVideoID, sid, nid).
		Scan(&e.ID, &e.VideoID, &e.SID, &e.NID, &e.Title, &e.PlaybackSource, &e.PlaybackURL, &active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.IsActive = active == 1
	return &e, nil
}

func (m *MySQL) GetDefaultEpisode(videoID uint64) (*model.Episode, error) {
	var e model.Episode
	var active int
	var hlsKey sql.NullString
	err := m.DB.QueryRow(`
		SELECT id, video_id, sid, nid, title, playback_source, playback_url,
		       COALESCE(hls_object_key,''), is_active
		FROM video_episodes
		WHERE video_id=? AND is_active=1
		ORDER BY sid ASC, nid ASC LIMIT 1`, videoID).
		Scan(&e.ID, &e.VideoID, &e.SID, &e.NID, &e.Title, &e.PlaybackSource, &e.PlaybackURL, &hlsKey, &active)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		err = m.DB.QueryRow(`
			SELECT id, video_id, sid, nid, title, playback_source, playback_url, is_active
			FROM video_episodes
			WHERE video_id=? AND is_active=1
			ORDER BY sid ASC, nid ASC LIMIT 1`, videoID).
			Scan(&e.ID, &e.VideoID, &e.SID, &e.NID, &e.Title, &e.PlaybackSource, &e.PlaybackURL, &active)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	} else {
		e.HLSObjectKey = hlsKey.String
	}
	e.IsActive = active == 1
	return &e, nil
}

// MigratableEpisode is a candidate for R2/CDN media migration (authorized streams only).
type MigratableEpisode struct {
	ID              uint64
	VideoID         uint64
	SID             int
	NID             int
	Title           string
	PlaybackSource  string
	PlaybackURL     string
	PlaybackStatus  string
	HLSObjectKey    string
	MigrationStatus string
}

func (m *MySQL) ListMigratableEpisodes(limit int, episodeID uint64) ([]MigratableEpisode, error) {
	if limit <= 0 {
		limit = 1
	}
	q := `
		SELECT e.id, e.video_id, e.sid, e.nid, e.title, e.playback_source, e.playback_url,
		       COALESCE(e.playback_status,'ok'), COALESCE(e.hls_object_key,''), COALESCE(e.migration_status,'')
		FROM video_episodes e
		INNER JOIN videos v ON v.id = e.video_id
		WHERE e.is_active=1
		  AND COALESCE(v.is_active,1)=1
		  AND v.status IN ('ready','demo')
		  AND TRIM(e.playback_url) <> ''
		  AND LOWER(COALESCE(e.playback_status,'ok')) IN ('ok','available','')
		  AND LOWER(COALESCE(e.migration_status,'')) NOT IN ('done','migrated')
		  AND LOWER(COALESCE(e.playback_status,'')) NOT IN ('unavailable','member','vip','restricted','forbidden')`
	args := []any{}
	if episodeID > 0 {
		q += ` AND e.id=?`
		args = append(args, episodeID)
	}
	q += ` ORDER BY e.id ASC LIMIT ?`
	args = append(args, limit)

	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MigratableEpisode, 0, limit)
	for rows.Next() {
		var e MigratableEpisode
		if err := rows.Scan(&e.ID, &e.VideoID, &e.SID, &e.NID, &e.Title, &e.PlaybackSource, &e.PlaybackURL,
			&e.PlaybackStatus, &e.HLSObjectKey, &e.MigrationStatus); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (m *MySQL) MarkEpisodeMigrated(id uint64, objectKey, provider string) error {
	_, err := m.DB.Exec(`
		UPDATE video_episodes
		SET hls_object_key=?, storage_provider=?, migration_status='done',
		    migration_error=NULL, migrated_at=?
		WHERE id=?`, objectKey, provider, time.Now(), id)
	return err
}

func (m *MySQL) MarkEpisodeMigrationFailed(id uint64, msg string) error {
	_, err := m.DB.Exec(`
		UPDATE video_episodes
		SET migration_status='failed', migration_error=?
		WHERE id=?`, msg, id)
	return err
}

func (m *MySQL) GetEpisodeMigrationState(id uint64) (hlsKey, migrationStatus, playbackURL string, err error) {
	err = m.DB.QueryRow(`
		SELECT COALESCE(hls_object_key,''), COALESCE(migration_status,''), COALESCE(playback_url,'')
		FROM video_episodes WHERE id=?`, id).Scan(&hlsKey, &migrationStatus, &playbackURL)
	if err == sql.ErrNoRows {
		return "", "", "", fmt.Errorf("episode %d not found", id)
	}
	return hlsKey, migrationStatus, playbackURL, err
}

// GetMigratableEpisodeByID loads one episode for planning/migration (includes already-migrated).
func (m *MySQL) GetMigratableEpisodeByID(id uint64) (*MigratableEpisode, error) {
	var e MigratableEpisode
	err := m.DB.QueryRow(`
		SELECT e.id, e.video_id, e.sid, e.nid, e.title, e.playback_source, e.playback_url,
		       COALESCE(e.playback_status,'ok'), COALESCE(e.hls_object_key,''), COALESCE(e.migration_status,'')
		FROM video_episodes e
		WHERE e.id=?`, id).
		Scan(&e.ID, &e.VideoID, &e.SID, &e.NID, &e.Title, &e.PlaybackSource, &e.PlaybackURL,
			&e.PlaybackStatus, &e.HLSObjectKey, &e.MigrationStatus)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type scannable interface {
	Scan(dest ...any) error
}

func scanVideo(row scannable) (*model.Video, error) {
	var v model.Video
	var srcUp, last sql.NullTime
	var active int
	var actors sql.NullString
	err := row.Scan(
		&v.ID, &v.SourceSystem, &v.SourceID, &v.CategoryID, &v.SourceCategoryID,
		&v.Title, &v.Description, &v.Year, &v.Area, &v.Director, &actors, &v.Rating,
		&v.CoverKey, &v.CoverR2Key, &v.PosterSourceURL,
		&v.DurationSec, &v.ViewCount, &v.HLSMasterKey, &v.PlaybackSource, &v.PlaySID, &v.PlayNID,
		&v.PlaybackURL, &srcUp, &last, &v.SyncStatus, &active, &v.Status, &v.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if actors.Valid {
		v.Actors = actors.String
	}
	v.IsActive = active == 1
	if srcUp.Valid {
		t := srcUp.Time
		v.SourceUpdatedAt = &t
	}
	if last.Valid {
		t := last.Time
		v.LastSyncedAt = &t
	}
	return &v, nil
}
