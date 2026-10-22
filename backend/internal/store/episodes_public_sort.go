package store

import "sort"

const publicEpisodesSQL = `
		SELECT sid, nid, COALESCE(title,'')
		FROM video_episodes
		WHERE video_id=? AND COALESCE(is_active,1)=1
		ORDER BY sid ASC, nid ASC`

// SortPublicEpisodes orders episodes by sid, then nid.
// Always returns a non-nil slice so JSON encodes empty results as [] not null.
func SortPublicEpisodes(in []PublicEpisode) []PublicEpisode {
	out := make([]PublicEpisode, 0, len(in))
	out = append(out, in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].SID != out[j].SID {
			return out[i].SID < out[j].SID
		}
		return out[i].NID < out[j].NID
	})
	return out
}
