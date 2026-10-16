package store

import "testing"

func TestPendingEnrichmentFetchLimit(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{15, 21},
		{20, 26},
		{40, 48},    // 40 + max(6, 8)
		{60, 72},    // exceeds old hard cap of 60
		{90, 108},   // 90 + max(6, 18)
		{0, 26},     // default finalLimit 20
		{1000, 108}, // clamped to max final 90 → 108
	}
	for _, tc := range cases {
		if got := PendingEnrichmentFetchLimit(tc.in); got != tc.want {
			t.Fatalf("in=%d got=%d want=%d", tc.in, got, tc.want)
		}
	}
	if PendingEnrichmentFetchLimit(90) > PendingEnrichmentCandidateFetchCap {
		t.Fatal("must stay within candidate fetch cap")
	}
}
