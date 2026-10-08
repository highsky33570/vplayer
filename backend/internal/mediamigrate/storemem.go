package mediamigrate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/media"
)

const memoryPutMaxAttempts = 5

// MemoryObjectStore is an in-memory ObjectStore for tests.
type MemoryObjectStore struct {
	mu      sync.Mutex
	Objects map[string][]byte
	FailPut string // substring of key that should fail Put (permanent)

	// PutTransientFails: per-key count of remaining retryable failures.
	PutTransientFails map[string]int
	putAttempts       map[string]int
	PutCalls          int
}

func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{Objects: map[string][]byte{}}
}

func (m *MemoryObjectStore) Exists(ctx context.Context, key string) (bool, error) {
	_, ok, err := m.Head(ctx, key)
	return ok, err
}

func (m *MemoryObjectStore) Head(ctx context.Context, key string) (int64, bool, error) {
	_ = ctx
	key = strings.TrimLeft(key, "/")
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.Objects[key]
	if !ok {
		return 0, false, nil
	}
	return int64(len(b)), true, nil
}

func (m *MemoryObjectStore) Put(ctx context.Context, key string, body []byte, contentType string) error {
	_ = contentType
	key = strings.TrimLeft(key, "/")
	m.mu.Lock()
	m.PutCalls++
	m.mu.Unlock()

	if m.FailPut != "" && strings.Contains(key, m.FailPut) {
		return fmt.Errorf("simulated put failure for %s", key)
	}

	var lastErr error
	for attempt := 1; attempt <= memoryPutMaxAttempts; attempt++ {
		m.mu.Lock()
		if m.putAttempts == nil {
			m.putAttempts = map[string]int{}
		}
		m.putAttempts[key]++
		remaining := 0
		if m.PutTransientFails != nil {
			remaining = m.PutTransientFails[key]
		}
		m.mu.Unlock()

		if remaining > 0 {
			m.mu.Lock()
			m.PutTransientFails[key] = remaining - 1
			m.mu.Unlock()
			lastErr = &media.S3APIError{Op: "put", Key: key, StatusCode: 503, Message: "service_unavailable"}
			if attempt == memoryPutMaxAttempts {
				return lastErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
			continue
		}

		m.mu.Lock()
		cp := make([]byte, len(body))
		copy(cp, body)
		m.Objects[key] = cp
		m.mu.Unlock()
		return nil
	}
	return lastErr
}

func (m *MemoryObjectStore) PublicURL(key string) string {
	return "/" + strings.TrimLeft(key, "/")
}

func (m *MemoryObjectStore) PutAttemptCount(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.putAttempts == nil {
		return 0
	}
	return m.putAttempts[strings.TrimLeft(key, "/")]
}
