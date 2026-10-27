package store

import (
	"sort"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

type memHistoryRow struct {
	id            uint64
	userID        uint64
	videoID       uint64
	lastWatchedAt time.Time
	progressSec   int
	touch         uint64
}

// MemoryHistory is an in-memory HistoryRepo for tests.
type MemoryHistory struct {
	Videos *MemoryVideoCatalog

	mu    sync.Mutex
	seq   uint64
	touch uint64
	rows  []memHistoryRow
}

// NewMemoryHistory uses catalog for video lookups; nil creates a private catalog.
func NewMemoryHistory(catalog *MemoryVideoCatalog) *MemoryHistory {
	if catalog == nil {
		catalog = NewMemoryVideoCatalog()
	}
	return &MemoryHistory{Videos: catalog}
}

func (m *MemoryHistory) AddVideo(v model.Video) { m.Videos.AddVideo(v) }

func (m *MemoryHistory) HistoryVideoExists(videoID uint64) (bool, error) {
	return m.Videos.Exists(videoID), nil
}

func (m *MemoryHistory) UpsertHistory(userID, videoID uint64, progressSec int, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.touch++
	for i := range m.rows {
		r := &m.rows[i]
		if r.userID == userID && r.videoID == videoID {
			r.lastWatchedAt = now.UTC()
			r.touch = m.touch
			if progressSec >= 0 {
				r.progressSec = progressSec
			}
			return nil
		}
	}
	m.seq++
	if progressSec < 0 {
		progressSec = 0
	}
	m.rows = append(m.rows, memHistoryRow{
		id: m.seq, userID: userID, videoID: videoID, lastWatchedAt: now.UTC(), progressSec: progressSec, touch: m.touch,
	})
	return nil
}

func (m *MemoryHistory) visibleRows(userID uint64) []memHistoryRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	var rows []memHistoryRow
	for _, r := range m.rows {
		if r.userID != userID {
			continue
		}
		if _, ok := m.Videos.Visible(r.videoID); !ok {
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

func (m *MemoryHistory) ListHistory(userID uint64, limit, offset int, includeMembership bool) ([]HistoryVideo, int, error) {
	limit, offset = clampPage(limit, offset, historyDefaultLimit, historyMaxLimit)
	rows := m.visibleRows(userID)
	if !includeMembership {
		filtered := rows[:0]
		for _, r := range rows {
			if v, ok := m.Videos.Visible(r.videoID); ok && !v.MembershipRequired {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	// Most recent first; touch breaks same-second ties the way a later auto-increment id cannot for upserts.
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].lastWatchedAt.Equal(rows[j].lastWatchedAt) {
			return rows[i].lastWatchedAt.After(rows[j].lastWatchedAt)
		}
		return rows[i].touch > rows[j].touch
	})
	total := len(rows)
	if offset >= len(rows) {
		return []HistoryVideo{}, total, nil
	}
	rows = rows[offset:]
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]HistoryVideo, 0, len(rows))
	for _, r := range rows {
		v, _ := m.Videos.Visible(r.videoID)
		out = append(out, HistoryVideo{Video: v, LastWatchedAt: r.lastWatchedAt, ProgressSec: r.progressSec})
	}
	return out, total, nil
}

func (m *MemoryHistory) DeleteHistory(userID, videoID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.rows[:0]
	for _, r := range m.rows {
		if r.userID == userID && r.videoID == videoID {
			continue
		}
		kept = append(kept, r)
	}
	m.rows = kept
	return nil
}

func (m *MemoryHistory) ClearHistory(userID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.rows[:0]
	for _, r := range m.rows {
		if r.userID != userID {
			kept = append(kept, r)
		}
	}
	m.rows = kept
	return nil
}

func (m *MemoryHistory) CountHistory(userID uint64) (int, error) {
	return len(m.visibleRows(userID)), nil
}
