package mediamigrate

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

// ObjectStore is the subset of media.ObjectStore used by migration.
type ObjectStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	Put(ctx context.Context, key string, body []byte, contentType string) error
}

// EpisodeRepo persists migration outcomes without wiping playback_url.
type EpisodeRepo interface {
	MarkEpisodeMigrated(id uint64, objectKey, provider string) error
	MarkEpisodeMigrationFailed(id uint64, msg string) error
	GetEpisodeMigrationState(id uint64) (hlsKey, migrationStatus, playbackURL string, err error)
}

// HTTPDoer downloads source playlists/segments.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Migrator performs authorized HLS → private R2 migration.
type Migrator struct {
	Store      ObjectStore
	Episodes   EpisodeRepo
	HTTP       HTTPDoer
	CDN        media.CDNURLOptions
	MaxBytes   int64
	Log        *log.Logger
	Provider   string

	// Source download reliability (0 = defaults).
	SourceMaxAttempts  int
	SourceRetryBase    time.Duration
	SourceHTTPTimeout  time.Duration
}

type MigrateResult struct {
	Plan           Plan
	Downloaded     []string // redacted source URLs
	UploadedKeys   []string
	PlaylistKind   PlaylistKind
	AlreadyMigrated bool
	Skipped        bool
	Error          string
}

func (m *Migrator) logger() *log.Logger {
	if m.Log != nil {
		return m.Log
	}
	return log.Default()
}

func (m *Migrator) maxBytes() int64 {
	if m.MaxBytes > 0 {
		return m.MaxBytes
	}
	return 256 << 20 // 256 MiB per object
}

func (m *Migrator) provider() string {
	if m.Provider != "" {
		return m.Provider
	}
	return "r2"
}

// InspectSource downloads the playlist (read-only) and enriches the plan.
func (m *Migrator) InspectSource(ctx context.Context, ep store.MigratableEpisode) (Plan, error) {
	plan := BuildPlan(ep, m.CDN)
	if plan.SkipReason != "" || plan.AlreadyMigrated {
		return plan, nil
	}
	if plan.NeedsFFmpeg || plan.SourceType != "hls_m3u8" {
		plan.SkipReason = "unsupported_source_type"
		plan.PlaylistKind = string(PlaylistUnknown)
		return plan, nil
	}

	body, err := m.fetchBytesLabeled(ctx, ep.PlaybackURL, "playlist", -1, "index")
	if err != nil {
		plan.InspectError = err.Error()
		plan.PlaylistKind = string(PlaylistUnknown)
		return plan, nil
	}
	parsed, playlistURL, err := m.resolveMediaPlaylist(ctx, ep.PlaybackURL, body)
	if err != nil {
		plan.InspectError = err.Error()
		if strings.Contains(err.Error(), "master_playlist") {
			plan.SkipReason = "master_playlist_unsupported"
			plan.PlaylistKind = string(PlaylistMaster)
		}
		return plan, nil
	}
	plan.PlaylistKind = string(parsed.Kind)
	if playlistURL != ep.PlaybackURL {
		plan.PlaylistKind = "master_single_variant→media"
	}
	plan.SegmentPathPattern = EpisodePrefix(ep.VideoID, ep.ID) + "segmentNNN.ts"
	plan.WouldDownload = []string{RedactURL(ep.PlaybackURL)}
	if playlistURL != ep.PlaybackURL {
		plan.WouldDownload = append(plan.WouldDownload, RedactURL(playlistURL))
	}
	plan.WouldUpload = []string{plan.ProposedR2Key}

	switch parsed.Kind {
	case PlaylistMedia:
		for _, seg := range parsed.Segments {
			plan.WouldDownload = append(plan.WouldDownload, RedactURL(seg.SourceURL))
			plan.WouldUpload = append(plan.WouldUpload, EpisodePrefix(ep.VideoID, ep.ID)+seg.LocalName)
		}
		plan.SegmentCount = len(parsed.Segments)
	default:
		plan.SkipReason = "unknown_playlist_structure"
	}
	return plan, nil
}

// MigrateEpisode downloads, rewrites, uploads, verifies, then updates DB.
// On any failure before successful verification, playback_url is left unchanged.
func (m *Migrator) MigrateEpisode(ctx context.Context, ep store.MigratableEpisode) MigrateResult {
	res := MigrateResult{}
	plan := BuildPlan(ep, m.CDN)
	res.Plan = plan

	if plan.AlreadyMigrated {
		res.AlreadyMigrated = true
		res.Skipped = true
		m.logger().Printf("episode=%d already migrated key=%s", ep.ID, plan.ProposedR2Key)
		return res
	}
	if plan.SkipReason != "" {
		res.Skipped = true
		return res
	}
	if plan.SourceType != "hls_m3u8" || plan.NeedsFFmpeg {
		res.Skipped = true
		res.Error = "unsupported_source_type"
		res.Plan.SkipReason = res.Error
		return res
	}

	// Idempotency: if DB already done, skip.
	if m.Episodes != nil {
		key, status, _, err := m.Episodes.GetEpisodeMigrationState(ep.ID)
		if err == nil && key != "" && (status == "done" || status == "migrated") {
			res.AlreadyMigrated = true
			res.Skipped = true
			res.Plan.AlreadyMigrated = true
			res.Plan.ProposedR2Key = key
			res.Plan.ProposedCDNURL = media.BuildMediaURL(m.CDN, key)
			res.Plan.SkipReason = "already_migrated"
			return res
		}
	}

	body, err := m.fetchBytesLabeled(ctx, ep.PlaybackURL, "playlist", -1, "index")
	if err != nil {
		res.Error = "playlist_download_failed"
		m.fail(ep.ID, res.Error)
		return res
	}
	res.Downloaded = append(res.Downloaded, RedactURL(ep.PlaybackURL))

	parsed, playlistURL, err := m.resolveMediaPlaylist(ctx, ep.PlaybackURL, body)
	if err != nil {
		res.Error = err.Error()
		if strings.Contains(res.Error, "master_playlist") {
			res.Skipped = true
			res.Plan.SkipReason = "master_playlist_unsupported"
			res.PlaylistKind = PlaylistMaster
			res.Plan.PlaylistKind = string(PlaylistMaster)
		}
		m.fail(ep.ID, res.Error)
		return res
	}
	if playlistURL != ep.PlaybackURL {
		res.Downloaded = append(res.Downloaded, RedactURL(playlistURL))
	}
	res.PlaylistKind = parsed.Kind
	res.Plan.PlaylistKind = string(parsed.Kind)
	if playlistURL != ep.PlaybackURL {
		res.Plan.PlaylistKind = "master_single_variant→media"
	}

	if parsed.Kind != PlaylistMedia || len(parsed.Segments) == 0 {
		res.Error = "unknown_or_empty_media_playlist"
		m.fail(ep.ID, res.Error)
		return res
	}
	// Rewrite uses the resolved media playlist body (not the master).
	body = []byte(parsed.Raw)

	prefix := EpisodePrefix(ep.VideoID, ep.ID)
	playlistKey := media.EpisodeHLSObjectKey(ep.VideoID, ep.ID)

	// Download segments first; keep prior successes in this attempt.
	segBodies := make([][]byte, len(parsed.Segments))
	for i, seg := range parsed.Segments {
		b, err := m.fetchBytesLabeled(ctx, seg.SourceURL, "segment", i, seg.LocalName)
		if err != nil {
			res.Error = "segment_download_failed"
			m.logger().Printf("episode=%d segment_download_failed index=%d name=%s src=%s", ep.ID, i, seg.LocalName, RedactURL(seg.SourceURL))
			m.fail(ep.ID, res.Error)
			return res
		}
		segBodies[i] = b
		res.Downloaded = append(res.Downloaded, RedactURL(seg.SourceURL))
	}

	rewritten, err := RewriteMediaPlaylist(string(body), parsed.Segments)
	if err != nil {
		res.Error = "playlist_rewrite_failed"
		m.fail(ep.ID, res.Error)
		return res
	}

	if m.Store == nil {
		res.Error = "object_store_not_configured"
		m.fail(ep.ID, res.Error)
		return res
	}
	m.wirePutAttemptLogger(ep.ID)

	segmentKeys := make([]string, len(parsed.Segments))
	uploaded := make([]string, 0, len(parsed.Segments)+1)
	for i, seg := range parsed.Segments {
		key := prefix + seg.LocalName
		segmentKeys[i] = key
		if _, err := m.ensureObject(ctx, ep.ID, key, segBodies[i], contentTypeFor(seg.LocalName)); err != nil {
			res.Error = "segment_upload_failed"
			m.logger().Printf("episode=%d segment_upload_failed key=%s", ep.ID, key)
			m.fail(ep.ID, res.Error)
			return res
		}
		uploaded = append(uploaded, key)
	}

	// All segments must be present with expected size before index.m3u8.
	for i, key := range segmentKeys {
		if err := m.verifyObjectSize(ctx, ep.ID, key, int64(len(segBodies[i]))); err != nil {
			res.Error = "verification_failed"
			m.logger().Printf("episode=%d verification_failed key=%s phase=segments", ep.ID, key)
			m.fail(ep.ID, res.Error)
			return res
		}
	}

	playlistBody := []byte(rewritten)
	if _, err := m.ensureObject(ctx, ep.ID, playlistKey, playlistBody, "application/vnd.apple.mpegurl"); err != nil {
		res.Error = "playlist_upload_failed"
		m.logger().Printf("episode=%d playlist_upload_failed key=%s", ep.ID, playlistKey)
		m.fail(ep.ID, res.Error)
		return res
	}
	uploaded = append(uploaded, playlistKey)
	res.UploadedKeys = uploaded

	if err := m.verifyObjectSize(ctx, ep.ID, playlistKey, int64(len(playlistBody))); err != nil {
		res.Error = "verification_failed"
		m.logger().Printf("episode=%d verification_failed key=%s phase=playlist", ep.ID, playlistKey)
		m.fail(ep.ID, res.Error)
		return res
	}

	if m.Episodes == nil {
		res.Error = "episode_repo_not_configured"
		return res
	}
	if err := m.Episodes.MarkEpisodeMigrated(ep.ID, playlistKey, m.provider()); err != nil {
		res.Error = "db_update_failed"
		m.logger().Printf("episode=%d db_update_failed", ep.ID)
		return res
	}

	res.Plan.ProposedR2Key = playlistKey
	res.Plan.ProposedCDNURL = media.BuildMediaURL(m.CDN, playlistKey)
	res.Plan.ExpectedDB = "video_episodes.hls_object_key=" + playlistKey +
		"; storage_provider=" + m.provider() + "; migration_status=done; playback_url preserved"
	m.logger().Printf("episode=%d migrated key=%s segments=%d", ep.ID, playlistKey, len(parsed.Segments))
	return res
}

// resolveMediaPlaylist returns a media playlist. A master with exactly one
// variant is followed; multi-variant masters are rejected.
func (m *Migrator) resolveMediaPlaylist(ctx context.Context, playlistURL string, body []byte) (*ParsedPlaylist, string, error) {
	parsed, err := ParseMediaPlaylist(playlistURL, string(body))
	if err != nil {
		return nil, "", fmt.Errorf("playlist_parse_failed")
	}
	if parsed.Kind == PlaylistMedia {
		return parsed, playlistURL, nil
	}
	if parsed.Kind != PlaylistMaster {
		return nil, "", fmt.Errorf("unknown_playlist_structure")
	}
	parsed.MasterRefs = uniqueNonEmpty(parsed.MasterRefs)
	if len(parsed.MasterRefs) != 1 {
		return nil, "", fmt.Errorf("master_playlist_unsupported:%d_variants", len(parsed.MasterRefs))
	}
	childURL := parsed.MasterRefs[0]
	childBody, err := m.fetchBytesLabeled(ctx, childURL, "media_playlist", -1, "variant")
	if err != nil {
		return nil, "", fmt.Errorf("variant_playlist_download_failed")
	}
	child, err := ParseMediaPlaylist(childURL, string(childBody))
	if err != nil {
		return nil, "", fmt.Errorf("variant_playlist_parse_failed")
	}
	if child.Kind == PlaylistMaster {
		return nil, "", fmt.Errorf("master_playlist_unsupported:nested_master")
	}
	if child.Kind != PlaylistMedia || len(child.Segments) == 0 {
		return nil, "", fmt.Errorf("unknown_or_empty_media_playlist")
	}
	return child, childURL, nil
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (m *Migrator) fail(episodeID uint64, code string) {
	if m.Episodes == nil {
		return
	}
	// Record failure status but never clear playback_url.
	_ = m.Episodes.MarkEpisodeMigrationFailed(episodeID, code)
}

func contentTypeFor(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".m3u8"):
		return "application/vnd.apple.mpegurl"
	case strings.HasSuffix(lower, ".m4s"):
		return "video/iso.segment"
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	default:
		return "video/mp2t"
	}
}
