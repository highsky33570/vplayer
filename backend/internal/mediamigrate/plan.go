package mediamigrate

import (
	"net/url"
	"strings"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

// Plan describes a single episode migration without performing it.
type Plan struct {
	EpisodeID          uint64   `json:"episode_id"`
	VideoID            uint64   `json:"video_id"`
	SID                int      `json:"sid"`
	NID                int      `json:"nid"`
	Title              string   `json:"title"`
	SourceType         string   `json:"source_type"`
	SourceHost         string   `json:"source_host,omitempty"`
	PlaybackStatus     string   `json:"playback_status"`
	AlreadyMigrated    bool     `json:"already_migrated"`
	SkipReason         string   `json:"skip_reason,omitempty"`
	R2Bucket           string   `json:"r2_bucket"`
	ProposedR2Key      string   `json:"proposed_r2_object_key"`
	SegmentPathPattern string   `json:"segment_object_path_pattern,omitempty"`
	ProposedCDNURL     string   `json:"proposed_cdn_url"`
	CDNURLNote         string   `json:"cdn_url_note,omitempty"`
	PlaylistKind       string   `json:"playlist_kind,omitempty"`
	SegmentCount       int      `json:"segment_count,omitempty"`
	MasterChildCount   int      `json:"master_child_count,omitempty"`
	WouldDownload      []string `json:"would_download,omitempty"`
	WouldUpload        []string `json:"would_upload,omitempty"`
	ExpectedDB         string   `json:"expected_db_changes"`
	NeedsFFmpeg        bool     `json:"needs_ffmpeg"`
	InspectError       string   `json:"inspect_error,omitempty"`
}

func ClassifySource(playbackURL string) (sourceType, host string, needsFFmpeg bool) {
	u := strings.TrimSpace(playbackURL)
	if u == "" {
		return "empty", "", false
	}
	parsed, err := url.Parse(u)
	if err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	lower := strings.ToLower(u)
	switch {
	case strings.Contains(lower, ".m3u8"):
		return "hls_m3u8", host, false
	case strings.HasSuffix(lower, ".mp4") || strings.Contains(lower, ".mp4?"):
		return "mp4", host, true
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		return "http_url", host, true
	default:
		return "object_key", "", false
	}
}

func BuildPlan(ep store.MigratableEpisode, cdn media.CDNURLOptions) Plan {
	p := Plan{
		EpisodeID:      ep.ID,
		VideoID:        ep.VideoID,
		SID:            ep.SID,
		NID:            ep.NID,
		Title:          ep.Title,
		PlaybackStatus: ep.PlaybackStatus,
		R2Bucket:       cdn.Bucket,
	}
	p.SourceType, p.SourceHost, p.NeedsFFmpeg = ClassifySource(ep.PlaybackURL)

	status := strings.ToLower(strings.TrimSpace(ep.PlaybackStatus))
	switch status {
	case "unavailable", "member", "vip", "restricted", "forbidden":
		p.SkipReason = "unauthorized_or_unavailable"
		return p
	}
	if strings.TrimSpace(ep.PlaybackURL) == "" {
		p.SkipReason = "empty_playback_url"
		return p
	}
	if ep.HLSObjectKey != "" && (ep.MigrationStatus == "done" || ep.MigrationStatus == "migrated") {
		p.AlreadyMigrated = true
		p.ProposedR2Key = ep.HLSObjectKey
		p.SegmentPathPattern = EpisodePrefix(ep.VideoID, ep.ID) + "segmentNNN.*"
		p.ProposedCDNURL = media.BuildMediaURL(cdn, ep.HLSObjectKey)
		p.CDNURLNote = cdnURLNote(cdn.BaseURL)
		p.SkipReason = "already_migrated"
		p.ExpectedDB = "none"
		return p
	}

	p.ProposedR2Key = media.EpisodeHLSObjectKey(ep.VideoID, ep.ID)
	p.SegmentPathPattern = EpisodePrefix(ep.VideoID, ep.ID) + "segmentNNN.ts"
	p.ProposedCDNURL = media.BuildMediaURL(cdn, p.ProposedR2Key)
	p.CDNURLNote = cdnURLNote(cdn.BaseURL)
	p.ExpectedDB = "video_episodes.hls_object_key=" + p.ProposedR2Key +
		"; storage_provider=r2; migration_status=done; migrated_at=<now>; playback_url preserved"
	return p
}

func cdnURLNote(base string) string {
	b := strings.ToLower(strings.TrimSpace(base))
	if b == "" || strings.Contains(b, "example.com") || strings.Contains(b, "localhost") {
		return "MEDIA_CDN_BASE_URL is a placeholder; set the production media hostname before serving clients"
	}
	return ""
}
