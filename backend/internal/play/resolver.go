package play

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

// Resolver returns normalized playback info for the VPlayer frontend.
// It uses authorized stored playback metadata only — no OLEHDTV page parsing.
type Resolver struct {
	Cfg   config.Config
	Store *store.MySQL
}

func (r *Resolver) Resolve(videoID uint64, sid, nid int) (*model.PlaybackInfo, error) {
	if r.Store == nil {
		return demoPlayback(videoID, r.Cfg), nil
	}
	v, err := r.Store.GetVideo(videoID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, fmt.Errorf("video not ready")
	}
	if v.Status != "ready" && v.Status != "demo" {
		return nil, fmt.Errorf("video not ready")
	}

	source := v.PlaybackSource
	url := v.PlaybackURL
	if url == "" {
		url = v.HLSMasterKey
	}
	outSID, outNID := v.PlaySID, v.PlayNID
	if outSID == 0 {
		outSID = 1
	}
	if outNID == 0 {
		outNID = 1
	}

	if ep, err := r.findEpisode(videoID, sid, nid); err == nil && ep != nil {
		url = ep.PlaybackURL
		source = ep.PlaybackSource
		outSID, outNID = ep.SID, ep.NID
	} else if ep, err := r.Store.GetDefaultEpisode(videoID); err == nil && ep != nil {
		if url == "" {
			url = ep.PlaybackURL
			source = ep.PlaybackSource
			outSID, outNID = ep.SID, ep.NID
		}
	}

	if url == "" {
		if v.Status == "demo" {
			return demoPlayback(videoID, r.Cfg), nil
		}
		return nil, fmt.Errorf("no playback url")
	}

	playType := "hls"
	if !strings.Contains(strings.ToLower(url), ".m3u8") {
		playType = "url"
	}

	final := url
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		final = SignURL(r.Cfg.CDNBaseURL, r.Cfg.CDNSignSecret, url, r.Cfg.PlayTicketTTL)
	}

	return &model.PlaybackInfo{
		Type:   playType,
		URL:    final,
		Source: source,
		SID:    outSID,
		NID:    outNID,
		Ticket: NewTicket(videoID, r.Cfg.CDNSignSecret, r.Cfg.PlayTicketTTL),
	}, nil
}

func (r *Resolver) findEpisode(videoID uint64, sid, nid int) (*model.Episode, error) {
	if r.Store == nil || sid <= 0 || nid <= 0 {
		return nil, nil
	}
	var e model.Episode
	var active int
	err := r.Store.DB.QueryRow(`
		SELECT id, video_id, sid, nid, title, playback_source, playback_url, is_active
		FROM video_episodes
		WHERE video_id=? AND sid=? AND nid=? AND is_active=1`, videoID, sid, nid).
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

func demoPlayback(videoID uint64, cfg config.Config) *model.PlaybackInfo {
	return &model.PlaybackInfo{
		Type:   "hls",
		URL:    "https://test-streams.mux.dev/x36xhzz/x36xhzz.m3u8",
		Source: "demo",
		SID:    1,
		NID:    1,
		Ticket: NewTicket(videoID, cfg.CDNSignSecret, cfg.PlayTicketTTL),
	}
}
