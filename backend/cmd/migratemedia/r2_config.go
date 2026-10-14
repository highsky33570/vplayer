package main

import (
	"fmt"
	"strings"
)

// r2ExecuteConfig holds the credentials required for authoritative R2 migration.
// Never log AccessKey or SecretKey values.
type r2ExecuteConfig struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	Bucket     string
	Region     string
	PublicBase string
}

// validateExecuteR2Config fail-closes execute mode when remote R2 is not usable.
// Error text names the field only — never credential values.
func validateExecuteR2Config(c r2ExecuteConfig) error {
	if strings.TrimSpace(c.Bucket) == "" {
		return fmt.Errorf("R2_BUCKET is empty")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("R2_ENDPOINT is empty")
	}
	if isPlaceholderR2Endpoint(c.Endpoint) {
		return fmt.Errorf("R2_ENDPOINT is a placeholder (contains <ACCOUNT_ID> or similar); set a real Cloudflare R2 endpoint")
	}
	if strings.TrimSpace(c.AccessKey) == "" {
		return fmt.Errorf("R2_ACCESS_KEY_ID is empty")
	}
	if strings.TrimSpace(c.SecretKey) == "" {
		return fmt.Errorf("R2_SECRET_ACCESS_KEY is empty")
	}
	return nil
}

// isPlaceholderR2Endpoint detects unfinished template endpoints from .env.example.
func isPlaceholderR2Endpoint(endpoint string) bool {
	e := strings.TrimSpace(endpoint)
	if e == "" {
		return false
	}
	lower := strings.ToLower(e)
	if strings.Contains(e, "<ACCOUNT_ID>") || strings.Contains(lower, "<account_id>") {
		return true
	}
	if strings.Contains(e, "<") && strings.Contains(e, ">") {
		return true
	}
	return false
}

// r2ExecuteConfigReady reports whether execute-mode R2 validation would pass
// (dry-run messaging only; does not expose secrets).
func r2ExecuteConfigReady(c r2ExecuteConfig) bool {
	return validateExecuteR2Config(c) == nil
}
