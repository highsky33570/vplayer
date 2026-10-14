package store

import (
	"strings"
	"time"
)

// MediaMigrateLockName is the MySQL advisory lock for migratemedia workers.
const MediaMigrateLockName = "vplayer_media_migrate"

// MediaMigrationFailureCooldown skips recently failed episodes from batch worker
// selection only (ListMigratableEpisodes). CountMigratableEpisodes / Admin
// eligible_count do not apply this cooldown.
const MediaMigrationFailureCooldown = 24 * time.Hour

// MigratableEpisodeWhereSQL is the canonical general eligibility predicate
// (CountMigratableEpisodes / AdminMediaOverviewStats.eligible_count).
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

// MigratableWorkerCooldownSQL excludes recently failed episodes from batch selection.
// Uses migration_failed_at only (migrated_at remains success-only).
// Legacy failed rows with NULL migration_failed_at remain selectable once.
const MigratableWorkerCooldownSQL = `
		AND (
			LOWER(COALESCE(e.migration_status,'')) <> 'failed'
			OR e.migration_failed_at IS NULL
			OR e.migration_failed_at <= ?
		)`

func migratableEpisodesJoinFrom() string {
	return `
		FROM video_episodes e
		INNER JOIN videos v ON v.id = e.video_id
		WHERE ` + MigratableEpisodeWhereSQL
}

func listMigratableEpisodesQuery(limit int, episodeID uint64, now time.Time) (string, []any) {
	if limit <= 0 {
		limit = 1
	}
	q := `
		SELECT e.id, e.video_id, e.sid, e.nid, e.title, e.playback_source, e.playback_url,
		       COALESCE(e.playback_status,'ok'), COALESCE(e.hls_object_key,''), COALESCE(e.migration_status,'')` +
		migratableEpisodesJoinFrom()
	args := []any{}
	// Explicit --episode-id bypasses worker failure cooldown (inspect/retry).
	if episodeID == 0 {
		q += MigratableWorkerCooldownSQL
		args = append(args, now.Add(-MediaMigrationFailureCooldown))
	} else {
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
	// MigratedAt is success-only (unused by worker failure cooldown).
	MigratedAt *time.Time
	// MigrationFailedAt is last migration failure time for batch cooldown. Nil = legacy/never failed.
	MigrationFailedAt *time.Time
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

// MatchesMigratableWorkerSelection is general eligibility plus failure cooldown
// (batch ListMigratableEpisodes twin). Explicit --episode-id bypasses this.
func MatchesMigratableWorkerSelection(in MigratableEligibilityInput, now time.Time) bool {
	if !MatchesMigratableEligibility(in) {
		return false
	}
	if strings.ToLower(in.MigrationStatus) != "failed" {
		return true
	}
	if in.MigrationFailedAt == nil {
		return true // legacy failed without failure timestamp → selectable once
	}
	return !in.MigrationFailedAt.After(now.Add(-MediaMigrationFailureCooldown))
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

func filterMigratableWorkerSelection(rows []MigratableEligibilityInput, now time.Time) []MigratableEligibilityInput {
	out := make([]MigratableEligibilityInput, 0, len(rows))
	for _, r := range rows {
		if MatchesMigratableWorkerSelection(r, now) {
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
