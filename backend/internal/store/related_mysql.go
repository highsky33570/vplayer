package store

import (
	"strings"

	"github.com/tycdn/vplayer/internal/model"
)

// relatedVideosSQL ranks public catalog rows. It does not apply homepage midnight exclusion.
// Playback columns are scanned only so the shared video scanner stays aligned; handlers
// return a card DTO and do not send playback URLs.
var relatedVideosSQL = `
		SELECT id, COALESCE(source_system,''), COALESCE(source_id,''), category_id, COALESCE(source_category_id,''),
		       COALESCE(leaf_source_id,''),
		       title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		       COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		       duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), COALESCE(play_sid,1), COALESCE(play_nid,1),
		       COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		       COALESCE(is_active,1), status, created_at
		FROM videos
		WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1 AND id<>?` +
	MembershipOnlyCatalogExclusionSQL("videos.id") + `
		ORDER BY
		  CASE
		    WHEN ? <> '' AND category_id=? AND leaf_source_id=? THEN 0
		    WHEN category_id=? THEN 1
		    ELSE 2
		  END,
		  created_at DESC, id DESC
		LIMIT ?`

// ListRelatedVideos returns up to limit public videos ranked like RankRelatedVideos.
// Homepage midnight exclusion is not applied.
func (m *MySQL) ListRelatedVideos(current model.Video, limit int) ([]model.Video, error) {
	if limit <= 0 || limit > RelatedVideoLimit {
		limit = RelatedVideoLimit
	}
	leaf := strings.TrimSpace(current.LeafSourceID)
	rows, err := m.DB.Query(relatedVideosSQL, current.ID, leaf, current.CategoryID, leaf, current.CategoryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Video
	seen := map[uint64]bool{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		if v == nil || seen[v.ID] || v.ID == current.ID {
			continue
		}
		seen[v.ID] = true
		out = append(out, *v)
	}
	return out, rows.Err()
}
