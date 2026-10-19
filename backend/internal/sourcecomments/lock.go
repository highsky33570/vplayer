package sourcecomments

import (
	"context"
	"fmt"

	"github.com/tycdn/vplayer/internal/store"
)

// Locker acquires the shared source-comment advisory lock.
// Release must be called when the run finishes, including on error.
type Locker interface {
	Acquire(ctx context.Context) (release func(), err error)
}

func acquireCommentLock(ctx context.Context, locker Locker) (func(), error) {
	if locker == nil {
		return func() {}, nil
	}
	release, err := locker.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if release == nil {
		release = func() {}
	}
	return release, nil
}

// MySQLCommentLocker uses SourceCommentLockName. Maintenance, historical backfill,
// and one-video sync all take this lock so they cannot run together.
func MySQLCommentLocker(db *store.MySQL) Locker {
	return mysqlCommentLocker{db: db}
}

type mysqlCommentLocker struct {
	db *store.MySQL
}

func (m mysqlCommentLocker) Acquire(ctx context.Context) (func(), error) {
	if m.db == nil {
		return nil, fmt.Errorf("mysql unavailable")
	}
	hold, err := m.db.AcquireAdvisoryLock(ctx, store.SourceCommentLockName)
	if err != nil {
		return nil, err
	}
	return func() { _ = hold.Release() }, nil
}
