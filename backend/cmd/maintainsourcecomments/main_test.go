package main

import (
	"strings"
	"testing"
)

func TestMaintenanceCommandValidation(t *testing.T) {
	if _, err := validateOptions("olehdtv", "incremental", "BACKFILL", false, false); err == nil || !strings.Contains(err.Error(), "MAINTENANCE") {
		t.Fatalf("backfill confirm err=%v", err)
	}
	if _, err := validateOptions("olehdtv", "full", "", false, false); err == nil {
		t.Fatal("missing confirm accepted")
	}
	if _, err := validateOptions("olehdtv", "drain", "MAINTENANCE", false, false); err == nil {
		t.Fatal("drain mode accepted")
	}
	opts, err := validateOptions("olehdtv", "incremental", "MAINTENANCE", true, false)
	if err != nil || !opts.RetryFailed || opts.Mode != "incremental" || opts.StatusOnly {
		t.Fatalf("opts=%+v err=%v", opts, err)
	}
	full, err := validateOptions("olehdtv", "full", "MAINTENANCE", false, false)
	if err != nil || full.Mode != "full" {
		t.Fatalf("full=%+v err=%v", full, err)
	}
	status, err := validateOptions("olehdtv", "incremental", "", false, true)
	if err != nil || !status.StatusOnly {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if _, err := validateOptions("olehdtv", "full", "MAINTENANCE", false, true); err == nil {
		t.Fatal("status accepted confirm")
	}
}
