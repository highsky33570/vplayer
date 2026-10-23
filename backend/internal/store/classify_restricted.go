package store

import "strings"

// Historical unavailable episodes do not store why they became unavailable.
// playback_status, sync_status, is_active, playback_url, and sync_error do not
// distinguish membership markers from an unusable/encrypted stream. This query
// only finds rows that still need an explicit play-page classification.
// It does not treat unavailable itself as source_restricted.
const listUnclassifiedUnavailableSQL = `
SELECT id, video_id, COALESCE(source_video_id,''), sid, nid,
       COALESCE(play_page_url,''), COALESCE(playback_status,''), COALESCE(playback_url,'')
FROM video_episodes
WHERE id > ?
  AND LOWER(COALESCE(playback_status,'')) = 'unavailable'
  AND TRIM(COALESCE(playback_url,'')) = ''
  AND TRIM(COALESCE(restriction_reason,'')) = ''
  AND TRIM(COALESCE(play_page_url,'')) <> ''
ORDER BY id ASC
LIMIT ?`

// markMembershipRestrictionSQL sets only restriction_reason, and only while the
// row is still an empty-URL terminal unavailable episode with no reason yet.
const markMembershipRestrictionSQL = `
UPDATE video_episodes
SET restriction_reason = ?
WHERE id = ?
  AND LOWER(COALESCE(playback_status,'')) = 'unavailable'
  AND TRIM(COALESCE(playback_url,'')) = ''
  AND TRIM(COALESCE(restriction_reason,'')) = ''`

// UnclassifiedUnavailableEpisode is one historical candidate for a GET-only
// membership recheck. Playback status and URL are not rewritten by that job.
type UnclassifiedUnavailableEpisode struct {
	ID             uint64
	VideoID        uint64
	SourceVideoID  string
	SID            int
	NID            int
	PlayPageURL    string
	PlaybackStatus string
	PlaybackURL    string
}

// ListUnclassifiedUnavailable returns the next bounded page of historical
// unavailable episodes that have no restriction_reason yet. Ordered by id so
// callers resume with afterID = last handled id.
func (m *MySQL) ListUnclassifiedUnavailable(afterID uint64, limit int) ([]UnclassifiedUnavailableEpisode, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := m.DB.Query(listUnclassifiedUnavailableSQL, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnclassifiedUnavailableEpisode
	for rows.Next() {
		var e UnclassifiedUnavailableEpisode
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.SourceVideoID, &e.SID, &e.NID,
			&e.PlayPageURL, &e.PlaybackStatus, &e.PlaybackURL,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkMembershipRestriction records an explicit membership classification.
// It does not change playback_status, playback_url, or is_active.
// Returns false when the row no longer matches the guarded predicate.
func (m *MySQL) MarkMembershipRestriction(id uint64) (bool, error) {
	res, err := m.DB.Exec(markMembershipRestrictionSQL, RestrictionReasonMembership, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UnclassifiedUnavailablePredicate documents the candidate rule for tests.
func UnclassifiedUnavailablePredicate() string {
	return strings.TrimSpace(listUnclassifiedUnavailableSQL)
}
