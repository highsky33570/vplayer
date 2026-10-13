package store

import (
	"testing"
	"time"
)

func TestStaleThresholdForJob(t *testing.T) {
	d, ok := staleThresholdForJob(JobIncrementalSync)
	if !ok || d != IncrementalStaleAfter {
		t.Fatalf("%v %v", d, ok)
	}
	d, ok = staleThresholdForJob(JobEnrichment)
	if !ok || d != EnrichmentStaleAfter {
		t.Fatalf("%v %v", d, ok)
	}
	if _, ok := staleThresholdForJob("mystery"); ok {
		t.Fatal("unknown job must not invent threshold")
	}
	if AdminActionCooldown != 60*time.Second {
		t.Fatalf("cooldown=%v", AdminActionCooldown)
	}
}
