package mediamigrate

import (
	"context"
	"fmt"

	"github.com/tycdn/vplayer/internal/media"
)

// HeadObjectStore supports idempotent resume via HEAD + Content-Length.
type HeadObjectStore interface {
	Head(ctx context.Context, key string) (size int64, exists bool, err error)
}

type ensureResult struct {
	key      string
	skipped  bool
	uploaded bool
}

func (m *Migrator) ensureObject(ctx context.Context, episodeID uint64, key string, body []byte, contentType string) (ensureResult, error) {
	expected := int64(len(body))
	if hs, ok := m.Store.(HeadObjectStore); ok {
		size, exists, err := hs.Head(ctx, key)
		if err != nil {
			return ensureResult{key: key}, fmt.Errorf("head_failed: %w", err)
		}
		if exists && size == expected {
			m.logger().Printf("episode=%d object_skip_already_present key=%s size=%d", episodeID, key, size)
			return ensureResult{key: key, skipped: true}, nil
		}
		if exists && size != expected {
			m.logger().Printf("episode=%d object_size_mismatch key=%s head_size=%d expected=%d", episodeID, key, size, expected)
		}
	}
	if err := m.Store.Put(ctx, key, body, contentType); err != nil {
		status := 0
		category := "permanent"
		msg := media.SanitizeS3Message(err.Error())
		if api, ok := media.AsS3APIError(err); ok {
			status = api.StatusCode
			msg = api.Message
			if media.IsRetryableS3Error(err) {
				category = "transient"
			}
		} else if media.IsRetryableS3Error(err) {
			category = "transient"
		}
		m.logger().Printf("episode=%d object_put_failed key=%s http_status=%d category=%s message=%s", episodeID, key, status, category, msg)
		return ensureResult{key: key}, err
	}
	return ensureResult{key: key, uploaded: true}, nil
}

func (m *Migrator) verifyObjectSize(ctx context.Context, episodeID uint64, key string, expected int64) error {
	if hs, ok := m.Store.(HeadObjectStore); ok {
		size, exists, err := hs.Head(ctx, key)
		if err != nil {
			return err
		}
		if !exists || size != expected {
			m.logger().Printf("episode=%d verification_failed key=%s exists=%v head_size=%d expected=%d", episodeID, key, exists, size, expected)
			return fmt.Errorf("size_mismatch")
		}
		return nil
	}
	ok, err := m.Store.Exists(ctx, key)
	if err != nil || !ok {
		return fmt.Errorf("missing")
	}
	return nil
}

func (m *Migrator) wirePutAttemptLogger(episodeID uint64) {
	type putLoggerSetter interface {
		SetPutAttemptLogger(media.PutAttemptLogger)
	}
	if s, ok := m.Store.(putLoggerSetter); ok {
		s.SetPutAttemptLogger(func(key string, attempt int, statusCode int, category, message string) {
			m.logger().Printf("episode=%d r2_put_attempt key=%s attempt=%d http_status=%d category=%s message=%s",
				episodeID, key, attempt, statusCode, category, message)
		})
	}
}
