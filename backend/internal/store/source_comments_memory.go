package store

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// MemorySourceComments is an in-memory SourceCommentRepo for tests.
type MemorySourceComments struct {
	mu      sync.Mutex
	rows    []sourceRow
	listErr error
}

func NewMemorySourceComments() *MemorySourceComments {
	return &MemorySourceComments{}
}

// SetListErr makes the next list calls fail without touching stored rows.
func (m *MemorySourceComments) SetListErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listErr = err
}

// Snapshot returns stored rows for tests. Provider ids stay inside the store.
func (m *MemorySourceComments) Snapshot() []SourceCommentInput {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SourceCommentInput, len(m.rows))
	for i, row := range m.rows {
		out[i] = SourceCommentInput{
			SourceCommentID:       row.SourceCommentID,
			SourceVideoID:         row.SourceVideoID,
			ParentSourceCommentID: row.Parent,
			AuthorName:            row.Author,
			AvatarPath:            row.Avatar,
			Body:                  row.Body,
			SourceCreatedRaw:      row.Raw,
			DiggUp:                row.DiggUp,
			DiggDown:              row.DiggDown,
		}
	}
	return out
}

func (m *MemorySourceComments) rowByIdentity(system, id string) *sourceRow {
	for i := range m.rows {
		if m.rows[i].SourceSystem == system && m.rows[i].SourceCommentID == id {
			return &m.rows[i]
		}
	}
	return nil
}

// SourceSystem is stored on the memory row so tests can check provider scope.
func (m *MemorySourceComments) Systems() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, row := range m.rows {
		out = append(out, row.SourceSystem+"/"+row.SourceCommentID+"@"+row.SourceVideoID)
	}
	return out
}

func (m *MemorySourceComments) UpsertSourceComments(videoID uint64, sourceSystem string, rows []SourceCommentInput, now time.Time) (int, int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if videoID == 0 {
		return 0, 0, 0, errMissingVideo
	}
	sourceSystem = strings.TrimSpace(sourceSystem)
	if sourceSystem == "" {
		return 0, 0, 0, errMissingSystem
	}
	now = now.UTC()
	inserted, updated, skipped := 0, 0, 0
	seen := map[string]bool{}
	for _, row := range rows {
		row.SourceCommentID = strings.TrimSpace(row.SourceCommentID)
		if row.SourceCommentID == "" || seen[row.SourceCommentID] {
			skipped++
			continue
		}
		seen[row.SourceCommentID] = true
		cur := m.rowByIdentity(sourceSystem, row.SourceCommentID)
		if cur == nil {
			m.rows = append(m.rows, sourceRow{
				SourceCommentID: row.SourceCommentID,
				VideoID:         videoID,
				SourceSystem:    sourceSystem,
				SourceVideoID:   row.SourceVideoID,
				Parent:          strings.TrimSpace(row.ParentSourceCommentID),
				Author:          row.AuthorName,
				Avatar:          row.AvatarPath,
				Body:            row.Body,
				Raw:             row.SourceCreatedRaw,
				DiggUp:          row.DiggUp,
				DiggDown:        row.DiggDown,
				Status:          "visible",
				FetchedAt:       now,
				CreatedAt:       now,
				UpdatedAt:       now,
			})
			inserted++
			continue
		}
		if sameSourceContent(*cur, videoID, row) {
			cur.FetchedAt = now
			cur.UpdatedAt = now
			skipped++
			continue
		}
		cur.VideoID = videoID
		cur.SourceVideoID = row.SourceVideoID
		cur.Parent = strings.TrimSpace(row.ParentSourceCommentID)
		cur.Author = row.AuthorName
		cur.Avatar = row.AvatarPath
		cur.Body = row.Body
		cur.Raw = row.SourceCreatedRaw
		cur.DiggUp = row.DiggUp
		cur.DiggDown = row.DiggDown
		cur.FetchedAt = now
		cur.UpdatedAt = now
		updated++
	}
	return inserted, updated, skipped, nil
}

func (m *MemorySourceComments) ExistingSourceCommentIDs(sourceSystem string, ids []string) (map[string]struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]struct{}{}
	want := map[string]bool{}
	for _, id := range cleanIDs(ids) {
		want[id] = true
	}
	for _, row := range m.rows {
		if row.SourceSystem == sourceSystem && want[row.SourceCommentID] {
			out[row.SourceCommentID] = struct{}{}
		}
	}
	return out, nil
}

func (m *MemorySourceComments) ListSourceCommentThreads(videoID uint64, limit, offset int) ([]SourceThread, int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, 0, 0, m.listErr
	}
	limit, offset = clampSourcePage(limit, offset)
	var parents []sourceRow
	replies := map[string][]sourceRow{}
	rowTotal := 0
	for _, row := range m.rows {
		if row.VideoID != videoID || row.Status != "visible" {
			continue
		}
		rowTotal++
		if row.Parent == "" {
			parents = append(parents, row)
			continue
		}
		replies[row.Parent] = append(replies[row.Parent], row)
	}
	sort.Slice(parents, func(i, j int) bool {
		return sourceIDLess(parents[j].SourceCommentID, parents[i].SourceCommentID)
	})
	parentTotal := len(parents)
	if offset > len(parents) {
		return []SourceThread{}, parentTotal, rowTotal, nil
	}
	parents = parents[offset:]
	if len(parents) > limit {
		parents = parents[:limit]
	}
	threads := make([]SourceThread, 0, len(parents))
	for _, parent := range parents {
		children := append([]sourceRow(nil), replies[parent.SourceCommentID]...)
		sort.Slice(children, func(i, j int) bool {
			return sourceIDLess(children[i].SourceCommentID, children[j].SourceCommentID)
		})
		threads = append(threads, threadFrom(parent, children))
	}
	return threads, parentTotal, rowTotal, nil
}

// CreatedAt returns the insert time for a provider identity. Tests use it to prove reruns do not recreate the row.
func (m *MemorySourceComments) CreatedAt(sourceSystem, sourceCommentID string) (time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.rowByIdentity(sourceSystem, sourceCommentID)
	if row == nil {
		return time.Time{}, false
	}
	return row.CreatedAt, true
}
