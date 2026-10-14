package store

import "strings"

// MigratableEpisodeWhereSQL is the canonical eligibility predicate for automatic/batch
// R2 migration candidates (ListMigratableEpisodes / CountMigratableEpisodes /
// AdminMediaOverviewStats.eligible_count).
//
// Joined as:
//
//	FROM video_episodes e
//	INNER JOIN videos v ON v.id = e.video_id
//	WHERE <MigratableEpisodeWhereSQL>
//
// Do not simplify or reorder these conditions even if some appear redundant —
// production selection behavior must remain identical.
const MigratableEpisodeWhereSQL = `
		e.is_active=1
		AND COALESCE(v.is_active,1)=1
		AND v.status IN ('ready','demo')
		AND TRIM(e.playback_url) <> ''
		AND LOWER(COALESCE(e.playback_status,'ok')) IN ('ok','available','')
		AND LOWER(COALESCE(e.migration_status,'')) NOT IN ('done','migrated')
		AND LOWER(COALESCE(e.playback_status,'')) NOT IN ('unavailable','member','vip','restricted','forbidden')`

func migratableEpisodesJoinFrom() string {
	return `
		FROM video_episodes e
		INNER JOIN videos v ON v.id = e.video_id
		WHERE ` + MigratableEpisodeWhereSQL
}

func listMigratableEpisodesQuery(limit int, episodeID uint64) (string, []any) {
	if limit <= 0 {
		limit = 1
	}
	q := `
		SELECT e.id, e.video_id, e.sid, e.nid, e.title, e.playback_source, e.playback_url,
		       COALESCE(e.playback_status,'ok'), COALESCE(e.hls_object_key,''), COALESCE(e.migration_status,'')` +
		migratableEpisodesJoinFrom()
	args := []any{}
	if episodeID > 0 {
		q += ` AND e.id=?`
		args = append(args, episodeID)
	}
	q += ` ORDER BY e.id ASC LIMIT ?`
	args = append(args, limit)
	return q, args
}

func countMigratableEpisodesQuery() string {
	return `SELECT COUNT(*)` + migratableEpisodesJoinFrom()
}

// MigratableEligibilityInput mirrors columns used by MigratableEpisodeWhereSQL
// for in-memory behavior tests (no live MySQL required).
//
// PlaybackStatus and MigrationStatus model non-NULL column values (empty string
// is empty string, not SQL NULL).
type MigratableEligibilityInput struct {
	EpisodeActive   bool
	VideoActive     bool
	VideoStatus     string
	PlaybackURL     string
	PlaybackStatus  string
	MigrationStatus string
	// HlsObjectKey is unused by eligibility (SQL does not filter on it).
	HlsObjectKey string
}

// MatchesMigratableEligibility is the Go twin of MigratableEpisodeWhereSQL.
// Used only to prove list/count/admin eligible semantics on fixtures.
func MatchesMigratableEligibility(in MigratableEligibilityInput) bool {
	if !in.EpisodeActive {
		return false
	}
	if !in.VideoActive {
		return false
	}
	switch in.VideoStatus {
	case "ready", "demo":
	default:
		return false
	}
	if strings.TrimSpace(in.PlaybackURL) == "" {
		return false
	}
	// LOWER(COALESCE(e.playback_status,'ok')) — non-NULL empty stays "".
	psForAllow := strings.ToLower(in.PlaybackStatus)
	switch psForAllow {
	case "ok", "available", "":
	default:
		return false
	}
	ms := strings.ToLower(in.MigrationStatus)
	if ms == "done" || ms == "migrated" {
		return false
	}
	// LOWER(COALESCE(e.playback_status,''))
	psForDeny := strings.ToLower(in.PlaybackStatus)
	switch psForDeny {
	case "unavailable", "member", "vip", "restricted", "forbidden":
		return false
	}
	return true
}

// filterMigratableEligibility applies MatchesMigratableEligibility and returns
// matching rows in input order (list semantics without LIMIT).
func filterMigratableEligibility(rows []MigratableEligibilityInput) []MigratableEligibilityInput {
	out := make([]MigratableEligibilityInput, 0, len(rows))
	for _, r := range rows {
		if MatchesMigratableEligibility(r) {
			out = append(out, r)
		}
	}
	return out
}

// adminNotMigrated reports the independent Admin not_migrated_count predicate
// (status not in done/migrated/failed) — intentionally distinct from eligible.
func adminNotMigrated(migrationStatus string) bool {
	ms := strings.ToLower(migrationStatus)
	return ms != "done" && ms != "migrated" && ms != "failed"
}

// adminFailed reports the independent Admin failed_count predicate.
func adminFailed(migrationStatus string) bool {
	return strings.ToLower(migrationStatus) == "failed"
}
