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
	if _, err := ValidateTriggerType("admin"); err != nil {
		t.Fatal(err)
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
		t.Fatal("full_sync not recorded")
	}
}

func TestValidateTriggerRequester(t *testing.T) {
	if err := ValidateTriggerRequester(TriggerCLI, 0); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTriggerRequester(TriggerTimer, 0); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTriggerRequester(TriggerCLI, 9); err == nil {
		t.Fatal("cli must reject requester")
	}
	if err := ValidateTriggerRequester(TriggerTimer, 9); err == nil {
		t.Fatal("timer must reject requester")
	}
	if err := ValidateTriggerRequester(TriggerAdmin, 0); err == nil {
		t.Fatal("admin requires requester > 0")
	}
	if err := ValidateTriggerRequester(TriggerAdmin, 42); err != nil {
		t.Fatal(err)
	}
	if RequestedByPtr(0) != nil {
		t.Fatal("zero requester ptr must be nil")
	}
	if p := RequestedByPtr(7); p == nil || *p != 7 {
		t.Fatalf("unexpected ptr: %v", p)
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
