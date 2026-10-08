package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultS3PutMaxAttempts = 5

// Minimal SigV4 PutObject for Cloudflare R2 / S3-compatible endpoints.
func putS3Compatible(ctx context.Context, endpoint, bucket, accessKey, secretKey, key string, body []byte, contentType string) error {
	return putS3CompatibleRegionWithRetry(ctx, endpoint, bucket, accessKey, secretKey, "auto", key, body, contentType, defaultS3PutMaxAttempts, nil)
}

func putS3CompatibleRegion(ctx context.Context, endpoint, bucket, accessKey, secretKey, region, key string, body []byte, contentType string) error {
	return putS3CompatibleRegionWithRetry(ctx, endpoint, bucket, accessKey, secretKey, region, key, body, contentType, defaultS3PutMaxAttempts, nil)
}

// PutAttemptLogger receives sanitized per-attempt Put failures (optional).
type PutAttemptLogger func(key string, attempt int, statusCode int, category, message string)

func putS3CompatibleRegionWithRetry(ctx context.Context, endpoint, bucket, accessKey, secretKey, region, key string, body []byte, contentType string, maxAttempts int, log PutAttemptLogger) error {
	return retryS3Op(ctx, key, maxAttempts, log, func() error {
		return putS3CompatibleRegionOnce(ctx, endpoint, bucket, accessKey, secretKey, region, key, body, contentType)
	}, 0)
}

func retryS3Op(ctx context.Context, key string, maxAttempts int, log PutAttemptLogger, op func() error, baseBackoff time.Duration) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if baseBackoff <= 0 {
		baseBackoff = 500 * time.Millisecond
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = op()
		if lastErr == nil {
			return nil
		}
		status := 0
		msg := SanitizeS3Message(lastErr.Error())
		if api, ok := AsS3APIError(lastErr); ok {
			status = api.StatusCode
			msg = api.Message
		} else {
			status = parseStatusFromLegacyPutErr(lastErr.Error())
		}
		category := "permanent"
		if IsRetryableS3Error(lastErr) {
			category = "transient"
		}
		if log != nil {
			log(key, attempt, status, category, msg)
		}
		if !IsRetryableS3Error(lastErr) || attempt == maxAttempts {
			return lastErr
		}
		backoff := baseBackoff * time.Duration(1<<(attempt-1))
		if backoff > 8*time.Second {
			backoff = 8 * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return lastErr
}

func putS3CompatibleRegionOnce(ctx context.Context, endpoint, bucket, accessKey, secretKey, region, key string, body []byte, contentType string) error {
	key = strings.TrimLeft(key, "/")
	if region == "" {
		region = "auto"
	}
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	url := fmt.Sprintf("%s/%s/%s", strings.TrimRight(endpoint, "/"), bucket, key)
	payloadHash := sha256Hex(body)
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	service := "s3"

	canonicalURI := "/" + bucket + "/" + key
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		"PUT",
		canonicalURI,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := aws4SigningKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.Host = host
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("Authorization", auth)

	resp, err := s3HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 300 {
		return &S3APIError{
			Op:         "put",
			Key:        key,
			StatusCode: resp.StatusCode,
			Message:    SanitizeS3Message(string(b)),
		}
	}
	return nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func s3HTTPClient() *http.Client {
	return &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
}

func aws4SigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}
