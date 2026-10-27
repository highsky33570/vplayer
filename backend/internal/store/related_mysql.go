package store

import (
	"strings"

	"github.com/tycdn/vplayer/internal/model"
)

// relatedVideosSQL ranks public catalog rows. It does not apply homepage midnight exclusion.
// Playback columns are scanned only so the shared video scanner stays aligned; handlers
// return a card DTO and do not send playback URLs.
func relatedVideosSQL(includeMembershipRequired bool) string {
	q := `SELECT ` + videoScanSelect + `
		FROM videos
		WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1 AND id<>?`
	if !includeMembershipRequired {
		q += ` AND COALESCE(membership_required,0)=0`
	}
	q += MembershipOnlyCatalogExclusionSQL("videos.id")
	q += PlayableEpisodeCatalogSQL("videos.id") + `
		ORDER BY
		  CASE
		    WHEN ? <> '' AND category_id=? AND leaf_source_id=? THEN 0
		    WHEN category_id=? THEN 1
		    ELSE 2
		  END,
		  created_at DESC, id DESC
		LIMIT ?`
	return q
}

// ListRelatedVideos returns up to limit public videos ranked like RankRelatedVideos.
// Homepage midnight exclusion is not applied.
func (m *MySQL) ListRelatedVideos(current model.Video, limit int) ([]model.Video, error) {
	return m.ListRelatedVideosForViewer(current, limit, false)
}

// ListRelatedVideosForViewer is ListRelatedVideos with optional membership-gated titles.
func (m *MySQL) ListRelatedVideosForViewer(current model.Video, limit int, includeMembershipRequired bool) ([]model.Video, error) {
	if limit <= 0 || limit > RelatedVideoLimit {
		limit = RelatedVideoLimit
	}
	leaf := strings.TrimSpace(current.LeafSourceID)
	rows, err := m.DB.Query(relatedVideosSQL(includeMembershipRequired), current.ID, leaf, current.CategoryID, leaf, current.CategoryID, limit)
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
