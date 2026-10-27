package store

import (
	"os"
	"strings"
	"testing"
)

func TestMembershipExclusion_confirmedOnlyHidden(t *testing.T) {
	got := ClassifyCatalogVisibility([]EpisodeVisibilityFact{{
		PlaybackStatus:    "unavailable",
		RestrictionReason: RestrictionReasonMembership,
		SyncStatus:        "ok",
	}})
	if !got.HideFromCatalog || got.Class != VisibilityMembership {
		t.Fatalf("%+v", got)
	}
}

func TestMembershipExclusion_searchCategoryRelatedSharePredicate(t *testing.T) {
	list, listArgs := AppendReadyVideosWhere(
		`SELECT id FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`,
		nil, CategoryBrowseFilter{CanonicalCategoryID: 2}, "登山", "", false,
	)
	count, countArgs := AppendReadyVideosWhere(
		`SELECT COUNT(*) FROM videos WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`,
		nil, CategoryBrowseFilter{CanonicalCategoryID: 2}, "登山", "", false,
	)
	pred := "restriction_reason = '" + RestrictionReasonMembership + "'"
	related := relatedVideosSQL(false)
	if !strings.Contains(list, pred) || !strings.Contains(count, pred) || !strings.Contains(related, pred) {
		t.Fatal("list, count, and related must share the membership predicate")
	}
	if !strings.Contains(list, `COALESCE(membership_required,0)=0`) {
		t.Fatal("non-member list must exclude membership_required titles")
	}
	if strings.Contains(list, "ORDER BY") || strings.Contains(count, "LIMIT") {
		t.Fatal("predicate is WHERE-only; pagination is appended by the caller")
	}
	if len(listArgs) != len(countArgs) {
		t.Fatalf("count/list args diverged: %v vs %v", countArgs, listArgs)
	}
}

func TestMembershipExclusion_playableStaysVisible(t *testing.T) {
	got := ClassifyCatalogVisibility([]EpisodeVisibilityFact{{
		PlaybackStatus: "ok",
		PlaybackURL:    "https://cdn.example/a.m3u8",
		SyncStatus:     "ok",
		IsActive:       true,
	}})
	if got.HideFromCatalog || got.Class != VisibilityPlayable {
		t.Fatalf("%+v", got)
	}
}

func TestMembershipExclusion_mixedPlayableStaysVisible(t *testing.T) {
	got := ClassifyCatalogVisibility([]EpisodeVisibilityFact{
		{PlaybackStatus: "unavailable", RestrictionReason: RestrictionReasonMembership, SyncStatus: "ok"},
		{PlaybackStatus: "ok", PlaybackURL: "https://cdn.example/ep2.m3u8", SyncStatus: "ok", IsActive: true},
	})
	if got.HideFromCatalog || !got.HasPlayableEpisode {
		t.Fatalf("%+v", got)
	}
}

func TestMembershipExclusion_pendingNotMembership(t *testing.T) {
	for _, st := range []string{"pending_enrichment", "pending_player_parse", "observed", ""} {
		got := ClassifyCatalogVisibility([]EpisodeVisibilityFact{{
			PlaybackStatus: st,
			SyncStatus:     "ok",
		}})
		if got.HideFromCatalog || got.Class == VisibilityMembership {
			t.Fatalf("status %q: %+v", st, got)
		}
	}
	// Membership on one episode must not hide while another episode is still retryable.
	mixed := ClassifyCatalogVisibility([]EpisodeVisibilityFact{
		{PlaybackStatus: "unavailable", RestrictionReason: RestrictionReasonMembership, SyncStatus: "ok"},
		{PlaybackStatus: "pending_enrichment", SyncStatus: "ok"},
	})
	if mixed.HideFromCatalog || mixed.Class != VisibilityTemporary {
		t.Fatalf("pending sibling: %+v", mixed)
	}
	failed := ClassifyCatalogVisibility([]EpisodeVisibilityFact{{
		PlaybackStatus: "unavailable",
		SyncStatus:     "error",
	}})
	if failed.HideFromCatalog || failed.Class != VisibilityTemporary {
		t.Fatalf("sync error: %+v", failed)
	}
}

func TestMembershipExclusion_parserFailureNotHidden(t *testing.T) {
	got := ClassifyCatalogVisibility([]EpisodeVisibilityFact{{
		PlaybackStatus: "unavailable",
		SyncStatus:     "ok",
	}})
	if got.HideFromCatalog || got.Class != VisibilityParser {
		t.Fatalf("%+v", got)
	}
}

func TestMembershipExclusion_missingEpisodesNotHidden(t *testing.T) {
	got := ClassifyCatalogVisibility(nil)
	if got.HideFromCatalog || got.Class != VisibilityMissing {
		t.Fatalf("%+v", got)
	}
}

func TestMembershipExclusion_publicURLWithMarkerStaysVisible(t *testing.T) {
	got := ClassifyCatalogVisibility([]EpisodeVisibilityFact{{
		PlaybackStatus:    "unavailable",
		PlaybackURL:       "https://cdn.example/already.m3u8",
		RestrictionReason: RestrictionReasonMembership,
		SyncStatus:        "ok",
		IsActive:          false,
	}})
	if got.HideFromCatalog || !got.FalsePositiveURL || got.Class != VisibilityPublicURL {
		t.Fatalf("%+v", got)
	}
}

func TestMembershipMigrationDoesNotDelete(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/018_episode_restriction_reason.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.ToUpper(string(raw))
	for _, bad := range []string{"DELETE ", "DROP TABLE", "TRUNCATE"} {
		if strings.Contains(sqlText, bad) {
			t.Fatalf("migration must not destroy data: %s", bad)
		}
	}
	if !strings.Contains(sqlText, "ADD COLUMN RESTRICTION_REASON") {
		t.Fatal("expected additive restriction_reason column")
	}
	if !strings.Contains(sqlText, "ALGORITHM=INSTANT") {
		t.Fatal("column add must stay metadata-only")
	}
	if strings.Contains(sqlText, "AFTER ") || strings.Contains(sqlText, "ADD INDEX") || strings.Contains(sqlText, "CREATE INDEX") {
		t.Fatal("no mid-table rewrite and no extra index")
	}
}
