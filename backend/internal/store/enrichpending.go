package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PendingEnrichmentLockName is the MySQL advisory lock for enrichpending workers.
const PendingEnrichmentLockName = "vplayer_olehdtv_pending_enrichment"

// Absolute bounds for ListPendingEnrichmentEpisodes (must match catalogsync catch-up max).
const (
	PendingEnrichmentMaxFinalLimit     = 90
	PendingEnrichmentCandidateFetchCap = 120
	PendingEnrichmentDefaultFinalLimit = 20
)

// PendingEnrichmentEpisode is a backlog row eligible for deferred play-page enrichment.
type PendingEnrichmentEpisode struct {
	ID                uint64
	VideoID           uint64
	SourceSystem      string
	SourceVideoID     string
	SID               int
	NID               int
	Title             string
	PlaybackSource    string
	PlaybackURL       string
	PlayPageURL       string
	PlaybackStatus    string
	RestrictionReason string
	HlsObjectKey      string
	StorageProvider   string
	MigrationStatus   string
	IsActive          bool
	CreatedAt         time.Time
	// OkEpisodeCount is how many episodes on the same video already have status=ok + URL.
	OkEpisodeCount int
}

// PendingEnrichmentFetchLimit sizes the SQL candidate window for a worker --limit.
// Normal limits keep a small cushion (finalLimit+6). Catch-up uses a proportional
// cushion but never exceeds PendingEnrichmentCandidateFetchCap (120).
func PendingEnrichmentFetchLimit(finalLimit int) int {
	if finalLimit <= 0 {
		finalLimit = PendingEnrichmentDefaultFinalLimit
	}
	if finalLimit > PendingEnrichmentMaxFinalLimit {
		finalLimit = PendingEnrichmentMaxFinalLimit
	}
	cushion := 6
	if d := finalLimit / 5; d > cushion {
		cushion = d
	}
	fetchLimit := finalLimit + cushion
	if fetchLimit > PendingEnrichmentCandidateFetchCap {
		fetchLimit = PendingEnrichmentCandidateFetchCap
	}
	return fetchLimit
}

// ListPendingEnrichmentEpisodes returns diversified candidate backlog rows.
// At most 3 pending episodes per video are returned (pending_rank), then ordered by
// zero-ok priority so a single huge series cannot fill the entire LIMIT alone.
// finalLimit is the worker --limit (1–90 absolute store bound); fetch window is bounded.
// When includeMembershipRetry is true (operator auth configured), also includes empty-URL
// unavailable rows labeled source_restricted so authorized re-enrich can clear them.
func (m *MySQL) ListPendingEnrichmentEpisodes(system string, finalLimit int, includeMembershipRetry bool) ([]PendingEnrichmentEpisode, error) {
	if finalLimit <= 0 {
		finalLimit = PendingEnrichmentDefaultFinalLimit
	}
	if finalLimit > PendingEnrichmentMaxFinalLimit {
		finalLimit = PendingEnrichmentMaxFinalLimit
	}
	fetchLimit := PendingEnrichmentFetchLimit(finalLimit)
	const maxPerVideo = 3
	membershipFlag := 0
	if includeMembershipRetry {
		membershipFlag = 1
	}
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
		    AND COALESCE(e.playback_url,'') = ''
		    AND COALESCE(e.play_page_url,'') <> ''
		    AND (
		      LOWER(COALESCE(e.playback_status,'')) = 'pending_enrichment'
		      OR (
		        ? = 1
		        AND LOWER(COALESCE(e.playback_status,'')) = 'unavailable'
		        AND COALESCE(e.restriction_reason,'') = ?
		      )
		    )
		) ranked
		WHERE pending_rank <= ?
		ORDER BY ok_count ASC, video_id ASC, nid ASC, id ASC
		LIMIT ?`, system, membershipFlag, RestrictionReasonMembership, maxPerVideo, fetchLimit)
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
// Never touches hls_object_key, storage_provider, or migration_status.
// When status is ok and playbackURL is non-empty, sets is_active=1 so Resolver can serve it.
// Unavailable / empty-URL outcomes leave is_active unchanged (typically 0 for pending inserts).
// restrictionReason is stored only for explicit membership (source_restricted); any other
// value is cleared so parser failures and successful playback are not labeled membership.
// playback_status itself stays "unavailable" for membership so watch playback is unchanged.
// Returns true when a row was updated.
func (m *MySQL) ApplyPlaybackEnrichment(id uint64, playbackURL, playbackSource, status, restrictionReason string) (bool, error) {
	playbackURL = strings.TrimSpace(playbackURL)
	status = strings.TrimSpace(status)
	if status == "" {
		return false, fmt.Errorf("playback status required")
	}
	if restrictionReason != RestrictionReasonMembership {
		restrictionReason = ""
	}
	activate := 0
	if strings.EqualFold(status, "ok") && playbackURL != "" {
		activate = 1
	}
	now := time.Now()
	res, err := m.DB.Exec(`
		UPDATE video_episodes SET
		  playback_url = ?,
		  playback_source = CASE WHEN ? <> '' THEN ? ELSE playback_source END,
		  playback_status = ?,
		  restriction_reason = ?,
		  is_active = CASE WHEN ? = 1 THEN 1 ELSE is_active END,
		  last_synced_at = ?,
		  sync_status = 'ok'
		WHERE id = ?
		  AND COALESCE(playback_url,'') = ''
		  AND (
		    LOWER(COALESCE(playback_status,'')) = 'pending_enrichment'
		    OR (
		      LOWER(COALESCE(playback_status,'')) = 'unavailable'
		      AND COALESCE(restriction_reason,'') = ?
		      AND COALESCE(play_page_url,'') <> ''
		    )
		  )`,
		playbackURL,
		playbackSource, playbackSource,
		status,
		restrictionReason,
		activate,
		now,
		id,
		RestrictionReasonMembership,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// EpisodePlaybackState is the resolve-time status of one episode (any is_active).
type EpisodePlaybackState struct {
	PlaybackStatus string
	PlaybackURL    string
	Found          bool
}

// GetEpisodePlaybackState loads playback_status/url for a video episode without requiring is_active=1.
// Used so confirmed-unavailable (inactive) primary episodes can drive watch UX while siblings remain pending.
func (m *MySQL) GetEpisodePlaybackState(videoID uint64, sid, nid int) (EpisodePlaybackState, error) {
	var out EpisodePlaybackState
	err := m.DB.QueryRow(`
		SELECT COALESCE(playback_status,''), COALESCE(playback_url,'')
		FROM video_episodes
		WHERE video_id=? AND sid=? AND nid=?
		LIMIT 1`, videoID, sid, nid).
		Scan(&out.PlaybackStatus, &out.PlaybackURL)
	if err == sql.ErrNoRows {
		return EpisodePlaybackState{}, nil
	}
	if err != nil {
		return EpisodePlaybackState{}, err
	}
	out.Found = true
	return out, nil
}
