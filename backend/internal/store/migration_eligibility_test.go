package store

import (
	"os"
	"strings"
	"testing"
	"time"
)

func readStoreFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func baseEligible() MigratableEligibilityInput {
	return MigratableEligibilityInput{
		EpisodeActive:   true,
		VideoActive:     true,
		VideoStatus:     "ready",
		PlaybackURL:     "https://source.example/a/index.m3u8",
		PlaybackStatus:  "ok",
		MigrationStatus: "",
	}
}

func TestMatchesMigratableEligibility_fixtureMatrix(t *testing.T) {
	fixtures := []struct {
		name string
		in   MigratableEligibilityInput
		want bool
	}{
		{name: "eligible_ok", in: baseEligible(), want: true},
		{name: "eligible_demo_video", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.VideoStatus = "demo"
			return in
		}(), want: true},
		{name: "eligible_available_status", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackStatus = "available"
			return in
		}(), want: true},
		{name: "eligible_empty_playback_status", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackStatus = ""
			return in
		}(), want: true},
		{name: "B_done_excluded", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.MigrationStatus = "done"
			return in
		}(), want: false},
		{name: "B_migrated_excluded", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.MigrationStatus = "migrated"
			return in
		}(), want: false},
		{name: "C_failed_remains_eligible", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.MigrationStatus = "failed"
			return in
		}(), want: true},
		{name: "D_inactive_episode", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.EpisodeActive = false
			return in
		}(), want: false},
		{name: "E_inactive_video", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.VideoActive = false
			return in
		}(), want: false},
		{name: "F_video_status_draft", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.VideoStatus = "draft"
			return in
		}(), want: false},
		{name: "G_empty_playback_url", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackURL = "   "
			return in
		}(), want: false},
		{name: "H_unavailable", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackStatus = "unavailable"
			return in
		}(), want: false},
		{name: "I_pending_enrichment", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackStatus = "pending_enrichment"
			return in
		}(), want: false},
		{name: "hls_key_without_done_still_eligible", in: func() MigratableEligibilityInput {
			in := baseEligible()
			in.HlsObjectKey = "videos/1/2/index.m3u8"
			in.MigrationStatus = ""
			return in
		}(), want: true},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesMigratableEligibility(tc.in); got != tc.want {
				t.Fatalf("got %v want %v for %+v", got, tc.want, tc.in)
			}
		})
	}
}

func TestCountAndListMigratable_sameEligibleSet(t *testing.T) {
	rows := []MigratableEligibilityInput{
		baseEligible(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.MigrationStatus = "done"
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.MigrationStatus = "migrated"
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.MigrationStatus = "failed"
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.EpisodeActive = false
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.VideoActive = false
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.VideoStatus = "disabled"
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackURL = ""
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackStatus = "unavailable"
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.PlaybackStatus = "pending_enrichment"
			return in
		}(),
		func() MigratableEligibilityInput {
			in := baseEligible()
			in.VideoStatus = "demo"
			in.PlaybackStatus = "available"
			return in
		}(),
	}
	listed := filterMigratableEligibility(rows)
	counted := 0
	for _, r := range rows {
		if MatchesMigratableEligibility(r) {
			counted++
		}
	}
	if counted != len(listed) {
		t.Fatalf("count=%d list=%d", counted, len(listed))
	}
	if counted != 3 {
		t.Fatalf("expected 3 eligible (base, failed retry, demo), got %d", counted)
	}
}

func TestListAndCountQueriesShareCanonicalWhere(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	listQ, listArgs := listMigratableEpisodesQuery(10, 0, now)
	countQ := countMigratableEpisodesQuery()
	join := migratableEpisodesJoinFrom()
	if !strings.Contains(listQ, MigratableEpisodeWhereSQL) {
		t.Fatal("list query missing canonical WHERE")
	}
	if !strings.Contains(countQ, MigratableEpisodeWhereSQL) {
		t.Fatal("count query missing canonical WHERE")
	}
	if !strings.Contains(listQ, join) || !strings.Contains(countQ, join) {
		t.Fatal("list/count must share migratableEpisodesJoinFrom")
	}
	if !strings.Contains(listQ, "ORDER BY e.id ASC LIMIT") {
		t.Fatal("list must preserve ORDER BY e.id ASC LIMIT")
	}
	if !strings.Contains(listQ, MigratableWorkerCooldownSQL) {
		t.Fatal("batch list must include worker failure cooldown")
	}
	if strings.Contains(countQ, MigratableWorkerCooldownSQL) {
		t.Fatal("count/Admin eligible must NOT apply worker cooldown")
	}
	if len(listArgs) != 2 {
		t.Fatalf("list args want cooldown cutoff + limit, got %d", len(listArgs))
	}
	epListQ, epArgs := listMigratableEpisodesQuery(1, 99, now)
	if strings.Contains(epListQ, MigratableWorkerCooldownSQL) {
		t.Fatal("explicit episode-id list must bypass worker cooldown")
	}
	if len(epArgs) != 2 || epArgs[0] != uint64(99) {
		t.Fatalf("episode-id args=%v", epArgs)
	}
}

func TestWorkerCooldownSelection(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	neverFailed := baseEligible()

	recentFailed := baseEligible()
	recentFailed.MigrationStatus = "failed"
	recent := now.Add(-1 * time.Hour)
	recentFailed.MigrationFailedAt = &recent

	cooledFailed := baseEligible()
	cooledFailed.MigrationStatus = "failed"
	old := now.Add(-25 * time.Hour)
	cooledFailed.MigrationFailedAt = &old

	legacyFailed := baseEligible()
	legacyFailed.MigrationStatus = "failed"
	// MigrationFailedAt nil → selectable once

	// Recent migrated_at must NOT control failure cooldown (success-only field).
	failedWithRecentMigratedAt := baseEligible()
	failedWithRecentMigratedAt.MigrationStatus = "failed"
	failedWithRecentMigratedAt.MigratedAt = &recent
	// MigrationFailedAt nil → still selectable (legacy / cooldown uses failed_at only)

	done := baseEligible()
	done.MigrationStatus = "done"

	rows := []MigratableEligibilityInput{
		neverFailed, recentFailed, cooledFailed, legacyFailed, failedWithRecentMigratedAt, done,
	}

	general := filterMigratableEligibility(rows)
	if len(general) != 5 { // all except done
		t.Fatalf("general eligible=%d want 5", len(general))
	}
	if !MatchesMigratableEligibility(recentFailed) {
		t.Fatal("recent failed must remain generally eligible (Admin eligible_count)")
	}

	worker := filterMigratableWorkerSelection(rows, now)
	if len(worker) != 4 { // never, cooled, legacy, failedWithRecentMigratedAt — not recentFailed, not done
		t.Fatalf("worker selectable=%d want 4", len(worker))
	}
	if MatchesMigratableWorkerSelection(recentFailed, now) {
		t.Fatal("recently failed must be excluded from batch worker selection")
	}
	if !MatchesMigratableWorkerSelection(cooledFailed, now) {
		t.Fatal("failed older than cooldown must be selectable again")
	}
	if !MatchesMigratableWorkerSelection(neverFailed, now) {
		t.Fatal("never-failed eligible must remain selectable")
	}
	if !MatchesMigratableWorkerSelection(legacyFailed, now) {
		t.Fatal("legacy failed without migration_failed_at must remain selectable")
	}
	if !MatchesMigratableWorkerSelection(failedWithRecentMigratedAt, now) {
		t.Fatal("migrated_at must not control failure cooldown")
	}
}

func TestWorkerCooldownSQLUsesMigrationFailedAt(t *testing.T) {
	if !strings.Contains(MigratableWorkerCooldownSQL, "migration_failed_at") {
		t.Fatal("cooldown SQL must use migration_failed_at")
	}
	if strings.Contains(MigratableWorkerCooldownSQL, "migrated_at") {
		t.Fatal("cooldown SQL must not use migrated_at")
	}
	if strings.Contains(MigratableWorkerCooldownSQL, "updated_at") ||
		strings.Contains(MigratableWorkerCooldownSQL, "last_synced_at") {
		t.Fatal("cooldown SQL must not use updated_at or last_synced_at")
	}
}

func TestDefaultListLimitIsOne(t *testing.T) {
	now := time.Now()
	_, args := listMigratableEpisodesQuery(0, 0, now)
	if len(args) != 2 {
		t.Fatalf("args=%v", args)
	}
	if args[1] != 1 {
		t.Fatalf("default limit want 1 got %v", args[1])
	}
}

func TestAdminEligibleVsFailedAndNotMigratedSemantics(t *testing.T) {
	failedOK := baseEligible()
	failedOK.MigrationStatus = "failed"

	pending := baseEligible()
	pending.MigrationStatus = ""

	done := baseEligible()
	done.MigrationStatus = "done"

	inactive := baseEligible()
	inactive.EpisodeActive = false
	inactive.MigrationStatus = ""

	rows := []MigratableEligibilityInput{failedOK, pending, done, inactive}

	eligible := 0
	failed := 0
	notMigrated := 0
	for _, r := range rows {
		if MatchesMigratableEligibility(r) {
			eligible++
		}
		if adminFailed(r.MigrationStatus) {
			failed++
		}
		if adminNotMigrated(r.MigrationStatus) {
			notMigrated++
		}
	}
	if eligible != 2 {
		t.Fatalf("eligible=%d want 2", eligible)
	}
	if failed != 1 {
		t.Fatalf("failed=%d want 1", failed)
	}
	if notMigrated != 2 {
		t.Fatalf("not_migrated=%d want 2", notMigrated)
	}
	if !MatchesMigratableEligibility(failedOK) {
		t.Fatal("failed+valid playback must remain eligible for retry")
	}
	if adminNotMigrated(failedOK.MigrationStatus) {
		t.Fatal("failed must not count as not_migrated")
	}
}

func TestAdminMediaOverviewStats_usesCountMigratableEpisodes(t *testing.T) {
	src := readStoreFile(t, "admin_media.go")
	if strings.Contains(src, "adminMediaEligibleWhere") {
		t.Fatal("adminMediaEligibleWhere must be removed")
	}
	if !strings.Contains(src, "CountMigratableEpisodes()") {
		t.Fatal("AdminMediaOverviewStats must use CountMigratableEpisodes()")
	}
}
