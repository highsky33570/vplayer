package store

import (
	"testing"
	"time"
)

func TestPendingUTCDateTimeRoundTrip(t *testing.T) {
	in := time.Date(2026, 10, 16, 8, 49, 0, 0, time.UTC)
	s := formatUTCDateTime(in)
	if s != "2026-10-16 08:49:00" {
		t.Fatalf("formatted %q", s)
	}
	out, err := parseUTCDateTime(s)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Equal(in) || out.Location() != time.UTC {
		t.Fatalf("parsed %v loc %v", out, out.Location())
	}
	// A local clock with the same wall time must not be treated as UTC.
	local := time.Date(2026, 10, 16, 8, 49, 0, 0, time.FixedZone("CST", 8*3600))
	if formatUTCDateTime(local) == "2026-10-16 08:49:00" {
		t.Fatal("local time was not converted to UTC before formatting")
	}
}
