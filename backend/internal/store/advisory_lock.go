package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrOperationLocked is returned when GET_LOCK indicates another session holds the named lock.
var ErrOperationLocked = errors.New("operation locked")

// AdvisoryHold is a connection-scoped MySQL named lock.
// GET_LOCK and RELEASE_LOCK always use the same *sql.Conn.
type AdvisoryHold struct {
	conn *sql.Conn
	name string
	// onRelease is optional (unit-test fakes without *sql.Conn).
	onRelease func() error
}

// NewTestAdvisoryHold builds a hold that only runs onRelease (no MySQL).
// Production code must use AcquireAdvisoryLock.
func NewTestAdvisoryHold(onRelease func() error) *AdvisoryHold {
	return &AdvisoryHold{onRelease: onRelease}
}

// AcquireAdvisoryLock obtains a non-blocking MySQL named lock on a dedicated pool connection.
// On success the connection is reserved until Release. Catalog queries may use other pool connections.
func (m *MySQL) AcquireAdvisoryLock(ctx context.Context, name string) (*AdvisoryHold, error) {
	if m == nil || m.DB == nil {
		return nil, fmt.Errorf("mysql unavailable")
	}
	name = trimLockName(name)
	if name == "" {
		return nil, fmt.Errorf("advisory lock name required")
	}
	conn, err := m.DB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("advisory lock conn: %w", err)
	}
	var got sql.NullInt64
	err = conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 0)`, name).Scan(&got)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("advisory lock get: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		_ = conn.Close()
		return nil, ErrOperationLocked
	}
	return &AdvisoryHold{conn: conn, name: name}, nil
}

// Release runs RELEASE_LOCK on the same connection, then returns the connection to the pool.
// Uses a short independent context so work-context cancellation cannot skip release.
func (h *AdvisoryHold) Release() error {
	if h == nil {
		return nil
	}
	if h.onRelease != nil {
		fn := h.onRelease
		h.onRelease = nil
		return fn()
	}
	if h.conn == nil {
		return nil
	}
	conn := h.conn
	name := h.name
	h.conn = nil
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var n sql.NullInt64
	err := conn.QueryRowContext(ctx, `SELECT RELEASE_LOCK(?)`, name).Scan(&n)
	closeErr := conn.Close()
	if err != nil {
		return fmt.Errorf("advisory lock release: %w", err)
	}
	return closeErr
}

func trimLockName(name string) string {
	for len(name) > 0 && (name[0] == ' ' || name[0] == '\t') {
		name = name[1:]
	}
	for len(name) > 0 && (name[len(name)-1] == ' ' || name[len(name)-1] == '\t') {
		name = name[:len(name)-1]
	}
	return name
}
