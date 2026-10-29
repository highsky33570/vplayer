package store

import (
	"strings"
	"testing"
)

func TestAppendAdminVideosWhere_requiresPlayableEpisode(t *testing.T) {
	q, args := appendAdminVideosWhere(`SELECT id FROM videos WHERE 1=1`, nil, 0, "", nil, nil)
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
