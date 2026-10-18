package store

// PublicEpisode is the guest-safe episode row. Playback URLs are never selected.
type PublicEpisode struct {
	SID   int    `json:"sid"`
	NID   int    `json:"nid"`
	Title string `json:"title"`
}

// ListPublicEpisodes returns active episodes for a video, ordered sid, nid.
func (m *MySQL) ListPublicEpisodes(videoID uint64) ([]PublicEpisode, error) {
	rows, err := m.DB.Query(publicEpisodesSQL, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublicEpisode
	for rows.Next() {
		var ep PublicEpisode
		if err := rows.Scan(&ep.SID, &ep.NID, &ep.Title); err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return SortPublicEpisodes(out), nil
}
