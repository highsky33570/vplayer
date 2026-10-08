package main

import "testing"

func TestExitCodeFromSummary_executeFailed(t *testing.T) {
	if code := exitCodeFromSummary(map[string]int{"failed": 1, "migrated": 0}, true); code == 0 {
		t.Fatalf("expected non-zero, got %d", code)
	}
}

func TestExitCodeFromSummary_executeSuccess(t *testing.T) {
	if code := exitCodeFromSummary(map[string]int{"failed": 0, "migrated": 1}, true); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestExitCodeFromSummary_dryRunIgnoresFailed(t *testing.T) {
	if code := exitCodeFromSummary(map[string]int{"failed": 1}, false); code != 0 {
		t.Fatalf("dry-run should exit 0, got %d", code)
	}
}
