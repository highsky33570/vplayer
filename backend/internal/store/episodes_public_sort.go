package store

import "sort"

const publicEpisodesSQL = `
		SELECT sid, nid, COALESCE(title,'')
		FROM video_episodes
		WHERE video_id=? AND COALESCE(is_active,1)=1
		ORDER BY sid ASC, nid ASC`

// SortPublicEpisodes orders episodes by sid, then nid.
func SortPublicEpisodes(in []PublicEpisode) []PublicEpisode {
	out := append([]PublicEpisode(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].SID != out[j].SID {
			return out[i].SID < out[j].SID
		}
		return out[i].NID < out[j].NID
	})
	return out
}
