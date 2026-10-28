package httpapi

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseAdminExpiresAt(t *testing.T) {
	empty := "  "
	if got, msg := parseAdminExpiresAt(&empty); got != nil || msg != "" {
		t.Fatalf("empty: got=%v msg=%q", got, msg)
	}
	if got, msg := parseAdminExpiresAt(nil); got != nil || msg != "" {
		t.Fatalf("nil: got=%v msg=%q", got, msg)
	}
	rfc := "2030-01-02T03:04:05Z"
	got, msg := parseAdminExpiresAt(&rfc)
	if msg != "" || got == nil || !got.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("rfc3339: got=%v msg=%q", got, msg)
	}
	day := "2030-01-02"
	got, msg = parseAdminExpiresAt(&day)
	if msg != "" || got == nil || got.Day() != 2 || got.Hour() != 23 {
		t.Fatalf("date: got=%v msg=%q", got, msg)
	}
	bad := "tomorrow"
	if _, msg := parseAdminExpiresAt(&bad); msg == "" {
		t.Fatal("expected error for invalid time")
	}
}

func TestAdminYearString(t *testing.T) {
	cases := map[string]string{`"2024"`: "2024", `2023`: "2023", ` "  1999 " `: "1999"}
	for in, want := range cases {
		got, ok := adminYearString(json.RawMessage(in))
		if !ok || got == nil || *got != want {
			t.Fatalf("%s: got=%v ok=%v want %q", in, got, ok, want)
		}
	}
	if got, ok := adminYearString(nil); !ok || got != nil {
		t.Fatalf("absent year should be a no-op, got=%v ok=%v", got, ok)
	}
	if got, ok := adminYearString(json.RawMessage(`null`)); !ok || got != nil {
		t.Fatalf("null year should be a no-op, got=%v ok=%v", got, ok)
	}
	if _, ok := adminYearString(json.RawMessage(`{"a":1}`)); ok {
		t.Fatal("object year should be invalid")
	}
}
