package store

import (
	"fmt"
	"strings"
	"time"
)

// PendingEnrichmentLockName is the MySQL advisory lock for enrichpending workers.
const PendingEnrichmentLockName = "vplayer_olehdtv_pending_enrichment"

// PendingEnrichmentEpisode is a backlog row eligible for deferred play-page enrichment.
type PendingEnrichmentEpisode struct {
	ID             uint64
	VideoID        uint64
	SourceSystem   string
	SourceVideoID  string
	SID            int
	NID            int
	Title          string
	PlaybackSource string
	PlaybackURL    string
	PlayPageURL    string
	PlaybackStatus string
	HlsObjectKey   string
	StorageProvider string
	MigrationStatus string
	IsActive       bool
	CreatedAt      time.Time
	// OkEpisodeCount is how many episodes on the same video already have status=ok + URL.
	OkEpisodeCount int
}

// ListPendingEnrichmentEpisodes returns candidate backlog rows (caller applies fair batching).
// fetchLimit should be modestly larger than the final batch size (e.g. limit*4).
func (m *MySQL) ListPendingEnrichmentEpisodes(system string, fetchLimit int) ([]PendingEnrichmentEpisode, error) {
	if fetchLimit <= 0 {
		fetchLimit = 20
	}
	if fetchLimit > 80 {
		fetchLimit = 80
	}
	rows, err := m.DB.Query(`
		SELECT e.id, e.video_id, COALESCE(e.source_system,''), COALESCE(e.source_video_id,''),
		       e.sid, e.nid, COALESCE(e.title,''),
		       COALESCE(e.playback_source,''), COALESCE(e.playback_url,''), COALESCE(e.play_page_url,''),
		       COALESCE(e.playback_status,''), COALESCE(e.hls_object_key,''), COALESCE(e.storage_provider,''),
		       COALESCE(e.migration_status,''), e.is_active, e.created_at,
		       (
		         SELECT COUNT(*)
		         FROM video_episodes o
		         WHERE o.video_id = e.video_id
		           AND LOWER(COALESCE(o.playback_status,'')) = 'ok'
		           AND COALESCE(o.playback_url,'') <> ''
		       ) AS ok_count
		FROM video_episodes e
		WHERE e.source_system = ?
		  AND LOWER(COALESCE(e.playback_status,'')) = 'pending_enrichment'
		  AND COALESCE(e.playback_url,'') = ''
		  AND COALESCE(e.play_page_url,'') <> ''
		ORDER BY ok_count ASC, e.video_id ASC, e.nid ASC, e.id ASC
		LIMIT ?`, system, fetchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingEnrichmentEpisode
	for rows.Next() {
		var e PendingEnrichmentEpisode
		var active int
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.SourceSystem, &e.SourceVideoID,
			&e.SID, &e.NID, &e.Title,
			&e.PlaybackSource, &e.PlaybackURL, &e.PlayPageURL,
			&e.PlaybackStatus, &e.HlsObjectKey, &e.StorageProvider,
			&e.MigrationStatus, &active, &e.CreatedAt, &e.OkEpisodeCount,
		); err != nil {
			return nil, err
		}
		e.IsActive = active == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// ApplyPlaybackEnrichment sets playback_url/status only when the row is still pending+empty.
// Never touches hls_object_key, storage_provider, migration_status, or other migration fields.
// Returns true when a row was updated.
func (m *MySQL) ApplyPlaybackEnrichment(id uint64, playbackURL, playbackSource, status string) (bool, error) {
	playbackURL = strings.TrimSpace(playbackURL)
	status = strings.TrimSpace(status)
	if status == "" {
		return false, fmt.Errorf("playback status required")
	}
	now := time.Now()
	res, err := m.DB.Exec(`
		UPDATE video_episodes SET
		  playback_url = ?,
		  playback_source = CASE WHEN ? <> '' THEN ? ELSE playback_source END,
		  playback_status = ?,
		  is_active = CASE WHEN ? = 'ok' AND ? <> '' THEN 1 ELSE is_active END,
		  last_synced_at = ?,
		  sync_status = 'ok'
		WHERE id = ?
		  AND LOWER(COALESCE(playback_status,'')) = 'pending_enrichment'
		  AND COALESCE(playback_url,'') = ''`,
		playbackURL,
		playbackSource, playbackSource,
		status,
		status, playbackURL,
		now,
		id,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
