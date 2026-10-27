package store

import "strings"

// RestrictionReasonMembership is the only explicit membership/permission class.
// It matches maccms.PlayPageSourceRestricted. It is not inferred from category
// or from a merely missing stream.
const RestrictionReasonMembership = "source_restricted"

// MembershipOnlyCatalogExclusionSQL hides a video from public discovery when
// every episode is resolved and at least one is explicitly membership-restricted,
// and none has a playback URL or an unresolved/retryable state.
//
// Admin-gated titles (videos.membership_required=1) stay visible so the VIP
// channel can showcase them; playback is still blocked for non-members.
//
// videoIDExpr must be the outer videos primary key (unaliased queries use videos.id).
// Callers append this inside WHERE, before ORDER BY / LIMIT / OFFSET.
func MembershipOnlyCatalogExclusionSQL(videoIDExpr string) string {
	return `
AND NOT (
  EXISTS (
    SELECT 1 FROM video_episodes membership_hit
    WHERE membership_hit.video_id = ` + videoIDExpr + `
      AND membership_hit.restriction_reason = '` + RestrictionReasonMembership + `'
  )
  AND NOT EXISTS (
    SELECT 1 FROM video_episodes membership_open
    WHERE membership_open.video_id = ` + videoIDExpr + `
      AND (
        TRIM(COALESCE(membership_open.playback_url,'')) <> ''
        OR LOWER(TRIM(COALESCE(membership_open.playback_status,''))) IN ('','pending_enrichment','pending_player_parse','observed')
        OR LOWER(TRIM(COALESCE(membership_open.sync_status,'ok'))) IN ('error','failed')
      )
  )
  AND NOT EXISTS (
    SELECT 1 FROM videos membership_keep
    WHERE membership_keep.id = ` + videoIDExpr + `
      AND COALESCE(membership_keep.membership_required,0)=1
  )
)`
}

// PlayableEpisodeCatalogSQL keeps public discovery limited to titles users can
// actually play: at least one active episode with a non-empty playback_url.
// Pending enrichment / source_restricted / empty-URL rows stay in the DB for
// workers but do not appear in lists (avoids "no playback url" / unavailable UX).
// Admin membership_required VIP showcase titles remain listable without a URL.
//
// videoIDExpr is the outer videos primary key (videos.id or v.id).
func PlayableEpisodeCatalogSQL(videoIDExpr string) string {
	return `
AND (
  EXISTS (
    SELECT 1 FROM videos playable_keep
    WHERE playable_keep.id = ` + videoIDExpr + `
      AND COALESCE(playable_keep.membership_required,0)=1
  )
  OR EXISTS (
    SELECT 1 FROM video_episodes playable_ep
    WHERE playable_ep.video_id = ` + videoIDExpr + `
      AND TRIM(COALESCE(playable_ep.playback_url,'')) <> ''
      AND COALESCE(playable_ep.is_active,0)=1
  )
)`
}

// VisibilityClass is a read-only label for preserved catalog rows.
type VisibilityClass string

const (
	VisibilityMembership VisibilityClass = "explicit_membership"
	VisibilityPublicURL  VisibilityClass = "public_playable_url"
	VisibilityPlayable   VisibilityClass = "playable"
	VisibilityParser     VisibilityClass = "parser_or_classification_failure"
	VisibilityTemporary  VisibilityClass = "temporary_failure"
	VisibilityMissing    VisibilityClass = "missing_episode_data"
	VisibilityUnknown    VisibilityClass = "unknown"
)

// EpisodeVisibilityFact is one preserved episode row used for classification.
// It does not fetch or mutate source data.
type EpisodeVisibilityFact struct {
	PlaybackStatus    string
	PlaybackURL       string
	RestrictionReason string
	SyncStatus        string
	IsActive          bool
}

// CatalogVisibility is the public-discovery decision for one video.
type CatalogVisibility struct {
	Class                    VisibilityClass
	HideFromCatalog          bool
	HasExplicitMembership    bool
	HasLegitimatePlaybackURL bool
	HasPlayableEpisode       bool
	HasUnresolvedEpisode     bool
	// FalsePositiveURL is set when a membership marker coexists with an http(s)
	// playback URL already stored on the video. That row stays visible.
	FalsePositiveURL bool
}

// ClassifyCatalogVisibility decides catalog hiding from stored episode facts.
// Pending, retryable, incomplete, and unclassified rows stay visible.
// A single playable episode keeps the video visible.
func ClassifyCatalogVisibility(eps []EpisodeVisibilityFact) CatalogVisibility {
	out := CatalogVisibility{Class: VisibilityMissing}
	if len(eps) == 0 {
		return out
	}
	for _, e := range eps {
		url := strings.TrimSpace(e.PlaybackURL)
		if url != "" {
			out.HasLegitimatePlaybackURL = true
		}
		if legitimatePublicURL(url) && strings.TrimSpace(e.RestrictionReason) == RestrictionReasonMembership {
			out.FalsePositiveURL = true
		}
		st := strings.ToLower(strings.TrimSpace(e.PlaybackStatus))
		if e.IsActive && url != "" && st != "unavailable" {
			out.HasPlayableEpisode = true
		}
		if strings.TrimSpace(e.RestrictionReason) == RestrictionReasonMembership {
			out.HasExplicitMembership = true
		}
		if episodeUnresolved(e) {
			out.HasUnresolvedEpisode = true
		}
	}
	out.HideFromCatalog = out.HasExplicitMembership &&
		!out.HasLegitimatePlaybackURL &&
		!out.HasUnresolvedEpisode &&
		!out.HasPlayableEpisode
	switch {
	case out.HasPlayableEpisode && !out.FalsePositiveURL:
		out.Class = VisibilityPlayable
	case out.HasLegitimatePlaybackURL:
		out.Class = VisibilityPublicURL
	case out.HideFromCatalog:
		out.Class = VisibilityMembership
	case out.HasUnresolvedEpisode:
		out.Class = VisibilityTemporary
	case terminalUnusableOnly(eps):
		out.Class = VisibilityParser
	default:
		out.Class = VisibilityUnknown
	}
	return out
}

func episodeUnresolved(e EpisodeVisibilityFact) bool {
	st := strings.ToLower(strings.TrimSpace(e.PlaybackStatus))
	if st == "" || st == "pending_enrichment" || st == "pending_player_parse" || st == "observed" {
		return true
	}
	syncSt := strings.ToLower(strings.TrimSpace(e.SyncStatus))
	return syncSt == "error" || syncSt == "failed"
}

func legitimatePublicURL(url string) bool {
	u := strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func terminalUnusableOnly(eps []EpisodeVisibilityFact) bool {
	if len(eps) == 0 {
		return false
	}
	for _, e := range eps {
		st := strings.ToLower(strings.TrimSpace(e.PlaybackStatus))
		if st != "unavailable" || strings.TrimSpace(e.PlaybackURL) != "" {
			return false
		}
		if strings.TrimSpace(e.RestrictionReason) == RestrictionReasonMembership {
			return false
		}
		if episodeUnresolved(e) {
			return false
		}
	}
	return true
}
