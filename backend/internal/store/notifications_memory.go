package store

import (
	"sort"
	"sync"
	"time"
)

// MemoryNotifications is an in-memory NotificationRepo for tests.
type MemoryNotifications struct {
	mu   sync.Mutex
	seq  uint64
	rows []Notification
}

func NewMemoryNotifications() *MemoryNotifications {
	return &MemoryNotifications{}
}

func (m *MemoryNotifications) CreateNotification(n Notification) (Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	n.ID = m.seq
	n.IsRead = false
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	n.CreatedAt = n.CreatedAt.UTC()
	m.rows = append(m.rows, n)
	return n, nil
}

func (m *MemoryNotifications) ListNotifications(userID uint64, kind string, limit, offset int) ([]Notification, int, error) {
	limit, offset = clampPage(limit, offset, notificationDefaultLimit, notificationMaxLimit)
	m.mu.Lock()
	var rows []Notification
	for _, n := range m.rows {
		if n.UserID == userID && (kind == "" || n.Kind == kind) {
			rows = append(rows, n)
		}
	}
	m.mu.Unlock()
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID > rows[j].ID
	})
	total := len(rows)
	if offset >= len(rows) {
		return []Notification{}, total, nil
	}
	rows = rows[offset:]
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, total, nil
}

func (m *MemoryNotifications) CountUnread(userID uint64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.rows {
		if r.UserID == userID && !r.IsRead {
			n++
		}
	}
	return n, nil
}

func (m *MemoryNotifications) MarkRead(userID uint64, ids []uint64) error {
	want := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rows {
		if m.rows[i].UserID == userID && want[m.rows[i].ID] {
			m.rows[i].IsRead = true
		}
	}
	return nil
}

func (m *MemoryNotifications) MarkAllRead(userID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rows {
		if m.rows[i].UserID == userID {
			m.rows[i].IsRead = true
		}
	}
	return nil
}
