package store

import (
	"strings"
	"testing"
)

func TestUnclassifiedUnavailableSQLDoesNotGuess(t *testing.T) {
	q := strings.ToLower(UnclassifiedUnavailablePredicate())
	if !strings.Contains(q, "playback_status") || !strings.Contains(q, "= 'unavailable'") {
		t.Fatalf("candidates must already be terminal unavailable: %s", q)
	}
	if !strings.Contains(q, "restriction_reason") || !strings.Contains(q, "play_page_url") {
		t.Fatalf("%s", q)
	}
	if strings.Contains(q, "source_restricted") {
		t.Fatal("selection must not treat existing rows as already restricted")
	}
	mark := strings.ToLower(markMembershipRestrictionSQL)
	if strings.Contains(mark, "playback_status =") || strings.Contains(mark, "playback_url =") || strings.Contains(mark, "is_active") {
		t.Fatalf("mark must not rewrite playback fields: %s", mark)
	}
	if !strings.Contains(mark, "set restriction_reason") {
		t.Fatal(mark)
	}
}
