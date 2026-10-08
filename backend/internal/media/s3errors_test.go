package media

import (
	"strings"
	"testing"
)

func TestSanitizeS3Message_redactsCredentials(t *testing.T) {
	raw := "Error Credential=AKIA123/20260101/auto/s3/aws4_request Signature=abc123 https://secret.example/path?X-Amz-Signature=deadbeef"
	out := SanitizeS3Message(raw)
	if strings.Contains(out, "AKIA123") || strings.Contains(out, "deadbeef") {
		t.Fatalf("not sanitized: %q", out)
	}
	if !strings.Contains(out, "Credential=<redacted>") {
		t.Fatalf("missing redaction: %q", out)
	}
}

func TestIsRetryableS3Error(t *testing.T) {
	if !IsRetryableS3Error(&S3APIError{StatusCode: 429, Message: "rate"}) {
		t.Fatal("429 retryable")
	}
	if !IsRetryableS3Error(&S3APIError{StatusCode: 503, Message: "unavail"}) {
		t.Fatal("503 retryable")
	}
	if IsRetryableS3Error(&S3APIError{StatusCode: 403, Message: "denied"}) {
		t.Fatal("403 not retryable")
	}
}
