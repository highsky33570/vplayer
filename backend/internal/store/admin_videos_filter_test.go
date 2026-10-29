package store

import (
	"strings"
	"testing"
)

func TestAppendAdminVideosWhere_requiresPlayableEpisode(t *testing.T) {
	q, args := appendAdminVideosWhere(`SELECT id FROM videos WHERE 1=1`, nil, CategoryBrowseFilter{}, "", nil, nil)
	if !strings.Contains(q, "playable_ep.playback_url") {
		t.Fatalf("admin list must require a playable episode: %s", q)
	}
	if !strings.Contains(q, "restriction_reason = '"+RestrictionReasonMembership+"'") {
		t.Fatalf("admin list must hide membership-only unplayable titles: %s", q)
	}
	if !strings.Contains(q, "olehdtv") {
		t.Fatalf("admin list must still exclude OLEHDTV VIP: %s", q)
	}
	// OLEHDTV VIP placeholders only (parent 6 + leaves 601–613).
	if len(args) != 14 {
		t.Fatalf("unexpected args len=%d: %v", len(args), args)
	}
}

func TestAppendAdminVideosWhere_leafBrowse(t *testing.T) {
	q, args := appendAdminVideosWhere(`SELECT id FROM videos WHERE 1=1`, nil, CategoryBrowseFilter{
		CanonicalCategoryID: 16,
		LeafSourceID:        "506",
	}, "", nil, nil)
	if !strings.Contains(q, "leaf_source_id=?") {
		t.Fatalf("child category must filter leaf_source_id: %s", q)
	}
	if len(args) < 16 || args[14] != uint64(16) || args[15] != "506" {
		t.Fatalf("args=%v", args)
	}
}
