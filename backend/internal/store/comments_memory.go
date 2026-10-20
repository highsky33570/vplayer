package store

import (
	"sort"
	"sync"
	"time"
)

// MemoryComments is an in-memory CommentRepo for tests.
type MemoryComments struct {
	mu     sync.Mutex
	seq    uint64
	videos map[uint64]bool
	rows   []Comment
}

func NewMemoryComments() *MemoryComments {
	return &MemoryComments{videos: map[uint64]bool{}}
}

func (m *MemoryComments) AddVideo(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.videos[id] = true
}

// Seed inserts a row without validation so tests can place timestamps and deleted status.
func (m *MemoryComments) Seed(c Comment) Comment {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	if c.ID == 0 {
		c.ID = m.seq
	} else if c.ID > m.seq {
		m.seq = c.ID
	}
	if c.Status == "" {
		c.Status = "visible"
	}
	c.CreatedAt = c.CreatedAt.UTC()
	c.UpdatedAt = c.UpdatedAt.UTC()
	m.rows = append(m.rows, c)
	return c
}

func (m *MemoryComments) VideoExists(id uint64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.videos[id], nil
}

func (m *MemoryComments) ListVisibleComments(videoID uint64, limit, offset int) ([]Comment, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var visible []Comment
	for _, c := range m.rows {
		if c.VideoID == videoID && c.Status == "visible" {
			visible = append(visible, c)
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		if !visible[i].CreatedAt.Equal(visible[j].CreatedAt) {
			return visible[i].CreatedAt.After(visible[j].CreatedAt)
		}
		return visible[i].ID > visible[j].ID
	})
	total := len(visible)
	if offset > len(visible) {
		return []Comment{}, total, nil
	}
	visible = visible[offset:]
	if len(visible) > limit {
		visible = visible[:limit]
	}
	return visible, total, nil
}

func (m *MemoryComments) CountUserCommentsSince(userID uint64, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	since = since.UTC()
	n := 0
	for _, c := range m.rows {
		if c.UserID == userID && !c.CreatedAt.UTC().Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *MemoryComments) CreateComment(videoID, userID uint64, body string, now time.Time, parentID *uint64) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	now = now.UTC()
	c := Comment{
		ID:        m.seq,
		VideoID:   videoID,
		UserID:    userID,
		Body:      body,
		Status:    "visible",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if parentID != nil {
		p := *parentID
		c.ParentID = &p
	}
	m.rows = append(m.rows, c)
	return c, nil
}

func (m *MemoryComments) GetVisibleComment(id uint64) (Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.rows {
		if c.ID == id && c.Status == "visible" {
			return c, nil
		}
	}
	return Comment{}, ErrCommentNotFound
}

func (m *MemoryComments) SoftDeleteComment(commentID, userID uint64, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rows {
		c := &m.rows[i]
		if c.ID == commentID && c.UserID == userID && c.Status == "visible" {
			c.Status = "deleted"
			c.UpdatedAt = now.UTC()
			return nil
		}
	}
	return ErrCommentNotFound
}

// Snapshot returns a copy of stored rows for tests.
func (m *MemoryComments) Snapshot() []Comment {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Comment, len(m.rows))
	copy(out, m.rows)
	return out
}
