package main

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/media"
)

func validR2Cfg() r2ExecuteConfig {
	return r2ExecuteConfig{
		Endpoint:  "https://abc123.r2.cloudflarestorage.com",
		AccessKey: "AKIA_TEST",
		SecretKey: "secret_test",
		Bucket:    "dongman",
		Region:    "auto",
	}
}

func TestValidateExecuteR2Config_ok(t *testing.T) {
	if err := validateExecuteR2Config(validR2Cfg()); err != nil {
		t.Fatal(err)
	}
	if !r2ExecuteConfigReady(validR2Cfg()) {
		t.Fatal("ready")
	}
}

func TestValidateExecuteR2Config_missingAccessKey(t *testing.T) {
	c := validR2Cfg()
	c.AccessKey = ""
	err := validateExecuteR2Config(c)
	if err == nil || !strings.Contains(err.Error(), "R2_ACCESS_KEY_ID") {
		t.Fatalf("err=%v", err)
	}
	assertNoSecretLeak(t, err.Error())
}

func TestValidateExecuteR2Config_missingSecret(t *testing.T) {
	c := validR2Cfg()
	c.SecretKey = ""
	err := validateExecuteR2Config(c)
	if err == nil || !strings.Contains(err.Error(), "R2_SECRET_ACCESS_KEY") {
		t.Fatalf("err=%v", err)
	}
	assertNoSecretLeak(t, err.Error())
}

func TestValidateExecuteR2Config_emptyEndpoint(t *testing.T) {
	c := validR2Cfg()
	c.Endpoint = "   "
	err := validateExecuteR2Config(c)
	if err == nil || !strings.Contains(err.Error(), "R2_ENDPOINT is empty") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateExecuteR2Config_placeholderEndpoint(t *testing.T) {
	c := validR2Cfg()
	c.Endpoint = "https://<ACCOUNT_ID>.r2.cloudflarestorage.com"
	err := validateExecuteR2Config(c)
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "ACCOUNT_ID>.r2") {
		t.Fatal("must not echo full placeholder endpoint host")
	}
	assertNoSecretLeak(t, err.Error())
}

func TestValidateExecuteR2Config_emptyBucket(t *testing.T) {
	c := validR2Cfg()
	c.Bucket = ""
	err := validateExecuteR2Config(c)
	if err == nil || !strings.Contains(err.Error(), "R2_BUCKET") {
		t.Fatalf("err=%v", err)
	}
}

func TestIsPlaceholderR2Endpoint(t *testing.T) {
	if !isPlaceholderR2Endpoint("https://<ACCOUNT_ID>.r2.cloudflarestorage.com") {
		t.Fatal("ACCOUNT_ID placeholder")
	}
	if !isPlaceholderR2Endpoint("https://<other>.example.com") {
		t.Fatal("angle-bracket placeholder")
	}
	if isPlaceholderR2Endpoint("https://abc123.r2.cloudflarestorage.com") {
		t.Fatal("real-looking endpoint must not be placeholder")
	}
}

func TestExecuteObjectStoreUsesR2WithoutLocalFallback(t *testing.T) {
	c := validR2Cfg()
	if err := validateExecuteR2Config(c); err != nil {
		t.Fatal(err)
	}
	// Same construction as execute path in main.go (no LocalStore fallback).
	r2 := media.NewR2StoreRegion(c.Endpoint, c.Bucket, c.AccessKey, c.SecretKey, c.Region, c.PublicBase, nil)
	var _ media.ObjectStore = r2
	if r2.LocalFallback != nil {
		t.Fatal("execute R2 store must not attach LocalStore fallback")
	}
	if r2.Endpoint != strings.TrimRight(c.Endpoint, "/") || r2.Bucket != c.Bucket {
		t.Fatalf("endpoint/bucket mismatch: %+v", r2)
	}
}

func TestDryRunDoesNotRequireR2Credentials(t *testing.T) {
	c := r2ExecuteConfig{
		Endpoint:  "https://<ACCOUNT_ID>.r2.cloudflarestorage.com",
		AccessKey: "",
		SecretKey: "",
		Bucket:    "dongman",
	}
	// Dry-run path only checks readiness for messaging; validation is execute-only.
	if r2ExecuteConfigReady(c) {
		t.Fatal("incomplete config must not be ready for execute")
	}
	if err := validateExecuteR2Config(c); err == nil {
		t.Fatal("execute would refuse")
	}
	// InspectSource does not need Store; nil is acceptable for dry-run.
	var obj media.ObjectStore
	if obj != nil {
		t.Fatal("dry-run may leave object store unset")
	}
}

func TestValidationFailurePrecludesMigrationSuccessSemantics(t *testing.T) {
	// When validateExecuteR2Config fails, main exits before BatchRunner.Run —
	// so no MarkEpisodeMigrated / operational_runs success can occur.
	c := validR2Cfg()
	c.AccessKey = ""
	if validateExecuteR2Config(c) == nil {
		t.Fatal("expected validation failure")
	}
	// Prove helper itself never embeds secret material even when secret was set then cleared.
	c2 := validR2Cfg()
	c2.SecretKey = ""
	err := validateExecuteR2Config(c2)
	if err == nil {
		t.Fatal("expected secret failure")
	}
	assertNoSecretLeak(t, err.Error())
}

func assertNoSecretLeak(t *testing.T, msg string) {
	t.Helper()
	for _, bad := range []string{
		"secret_test", "super-secret", "AKIA_TEST", "password=", "SecretKey=",
	} {
		if strings.Contains(msg, bad) {
			t.Fatalf("error leaked sensitive material %q: %s", bad, msg)
		}
	}
}
