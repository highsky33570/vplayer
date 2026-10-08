package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryS3Op_transientThenSuccess(t *testing.T) {
	var attempts int
	var logged []int
	err := retryS3Op(context.Background(), "videos/1/1/seg.ts", 5, func(key string, attempt int, statusCode int, category, message string) {
		logged = append(logged, attempt)
		if key != "videos/1/1/seg.ts" || statusCode != 503 || category != "transient" {
			t.Fatalf("log key=%s attempt=%d status=%d cat=%s msg=%s", key, attempt, statusCode, category, message)
		}
	}, func() error {
		attempts++
		if attempts < 3 {
			return &S3APIError{Op: "put", Key: "videos/1/1/seg.ts", StatusCode: 503, Message: "busy"}
		}
		return nil
	}, time.Millisecond)
	if err != nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
	if len(logged) != 2 {
		t.Fatalf("logged attempts %#v", logged)
	}
}

func TestRetryS3Op_exhaustion(t *testing.T) {
	var attempts int
	err := retryS3Op(context.Background(), "k.ts", 5, nil, func() error {
		attempts++
		return &S3APIError{Op: "put", Key: "k.ts", StatusCode: 503, Message: "unavailable"}
	}, time.Millisecond)
	if err == nil || attempts != 5 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
	api, ok := AsS3APIError(err)
	if !ok || api.StatusCode != 503 {
		t.Fatalf("err=%v", err)
	}
}

func TestRetryS3Op_noRetryOn403(t *testing.T) {
	var attempts int
	err := retryS3Op(context.Background(), "k.ts", 5, nil, func() error {
		attempts++
		return &S3APIError{Op: "put", Key: "k.ts", StatusCode: 403, Message: "AccessDenied"}
	}, time.Millisecond)
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
	if !errors.As(err, new(*S3APIError)) {
		t.Fatalf("type %T", err)
	}
}

func TestRetryS3Op_retries429(t *testing.T) {
	var attempts int
	err := retryS3Op(context.Background(), "k", 3, nil, func() error {
		attempts++
		return &S3APIError{Op: "put", Key: "k", StatusCode: 429, Message: "rate"}
	}, time.Millisecond)
	if err == nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}
