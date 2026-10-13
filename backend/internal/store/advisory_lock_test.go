package store

import (
	"errors"
	"testing"
)

func TestErrOperationLockedSentinel(t *testing.T) {
	if !errors.Is(ErrOperationLocked, ErrOperationLocked) {
		t.Fatal("sentinel identity")
	}
	wrapped := errors.Join(ErrOperationLocked, errors.New("other"))
	if !errors.Is(wrapped, ErrOperationLocked) {
		t.Fatal("errors.Is should match ErrOperationLocked")
	}
}

func TestValidateTriggerAndJob(t *testing.T) {
	if _, err := ValidateTriggerType("cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateTriggerType("timer"); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateTriggerType("admin"); err == nil {
		t.Fatal("admin not allowed in 4A")
	}
	if _, err := ValidateTriggerType("evil"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := ValidateJobType(JobIncrementalSync); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateJobType(JobEnrichment); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateJobType("full_sync"); err == nil {
		t.Fatal("full_sync not recorded in 4A")
	}
}

func TestAdvisoryHoldReleaseNilSafe(t *testing.T) {
	var h *AdvisoryHold
	if err := h.Release(); err != nil {
		t.Fatal(err)
	}
	h2 := &AdvisoryHold{}
	if err := h2.Release(); err != nil {
		t.Fatal(err)
	}
}
