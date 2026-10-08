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

// ListPendingEnrichmentEpisodes returns diversified candidate backlog rows.
// At most 3 pending episodes per video are returned (pending_rank), then ordered by
// zero-ok priority so a single huge series cannot fill the entire LIMIT alone.
// finalLimit is the worker --limit (1–20); we fetch enough multi-video rows to fill it.
func (m *MySQL) ListPendingEnrichmentEpisodes(system string, finalLimit int) ([]PendingEnrichmentEpisode, error) {
	if finalLimit <= 0 {
		finalLimit = 20
	}
	if finalLimit > 20 {
		finalLimit = 20
	}
	// Enough for finalLimit with ≤3/video, plus a small cushion for Go-side filtering.
	fetchLimit := finalLimit + 6
	if fetchLimit > 60 {
		fetchLimit = 60
	}
	const maxPerVideo = 3
	// ROW_NUMBER caps each video at 3 pending rows before the global LIMIT, so a
	// single zero-ok series cannot monopolize the candidate window.
	rows, err := m.DB.Query(`
		SELECT id, video_id, source_system, source_video_id, sid, nid, title,
		       playback_source, playback_url, play_page_url, playback_status,
		       hls_object_key, storage_provider, migration_status, is_active, created_at, ok_count
		FROM (
		  SELECT e.id, e.video_id,
		         COALESCE(e.source_system,'') AS source_system,
		         COALESCE(e.source_video_id,'') AS source_video_id,
		         e.sid, e.nid, COALESCE(e.title,'') AS title,
		         COALESCE(e.playback_source,'') AS playback_source,
		         COALESCE(e.playback_url,'') AS playback_url,
		         COALESCE(e.play_page_url,'') AS play_page_url,
		         COALESCE(e.playback_status,'') AS playback_status,
		         COALESCE(e.hls_object_key,'') AS hls_object_key,
		         COALESCE(e.storage_provider,'') AS storage_provider,
		         COALESCE(e.migration_status,'') AS migration_status,
		         e.is_active, e.created_at,
		         (
		           SELECT COUNT(*)
		           FROM video_episodes o
		           WHERE o.video_id = e.video_id
		             AND LOWER(COALESCE(o.playback_status,'')) = 'ok'
		             AND COALESCE(o.playback_url,'') <> ''
		         ) AS ok_count,
		         ROW_NUMBER() OVER (
		           PARTITION BY e.video_id
		           ORDER BY e.nid ASC, e.id ASC
		         ) AS pending_rank
		  FROM video_episodes e
		  WHERE e.source_system = ?
		    AND LOWER(COALESCE(e.playback_status,'')) = 'pending_enrichment'
		    AND COALESCE(e.playback_url,'') = ''
		    AND COALESCE(e.play_page_url,'') <> ''
		) ranked
		WHERE pending_rank <= ?
		ORDER BY ok_count ASC, video_id ASC, nid ASC, id ASC
		LIMIT ?`, system, maxPerVideo, fetchLimit)
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
// Never touches hls_object_key, storage_provider, migration_status, is_active, or other migration fields.
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
		  last_synced_at = ?,
		  sync_status = 'ok'
		WHERE id = ?
		  AND LOWER(COALESCE(playback_status,'')) = 'pending_enrichment'
		  AND COALESCE(playback_url,'') = ''`,
		playbackURL,
		playbackSource, playbackSource,
		status,
		now,
		id,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
