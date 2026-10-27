package store

import (
	"sort"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

// MemoryFavorites is an in-memory FavoriteRepo for tests.
type MemoryFavorites struct {
	Videos *MemoryVideoCatalog

	mu       sync.Mutex
	seq      uint64
	folders  []FavoriteFolder
	favorite []Favorite
}

// NewMemoryFavorites uses catalog for video lookups; nil creates a private catalog.
func NewMemoryFavorites(catalog *MemoryVideoCatalog) *MemoryFavorites {
	if catalog == nil {
		catalog = NewMemoryVideoCatalog()
	}
	return &MemoryFavorites{Videos: catalog}
}

func (m *MemoryFavorites) AddVideo(v model.Video) { m.Videos.AddVideo(v) }

func (m *MemoryFavorites) FavoriteVideoExists(videoID uint64) (bool, error) {
	return m.Videos.Exists(videoID), nil
}

func (m *MemoryFavorites) EnsureDefaultFolder(userID uint64, now time.Time) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.folders {
		if f.UserID == userID && f.IsDefault {
			return f.ID, nil
		}
	}
	m.seq++
	m.folders = append(m.folders, FavoriteFolder{
		ID: m.seq, UserID: userID, Name: DefaultFavoriteFolderName, IsDefault: true, CreatedAt: now.UTC(),
	})
	return m.seq, nil
}

func (m *MemoryFavorites) ListFolders(userID uint64) ([]FavoriteFolder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []FavoriteFolder{}
	for _, f := range m.folders {
		if f.UserID != userID {
			continue
		}
		for _, fav := range m.favorite {
			if fav.UserID == userID && fav.FolderID != nil && *fav.FolderID == f.ID {
				f.Count++
			}
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *MemoryFavorites) AddFavorite(userID, videoID, folderID uint64, now time.Time) (Favorite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.favorite {
		if f.UserID == userID && f.VideoID == videoID {
			return f, nil
		}
	}
	m.seq++
	f := Favorite{ID: m.seq, UserID: userID, VideoID: videoID, CreatedAt: now.UTC()}
	if folderID != 0 {
		id := folderID
		f.FolderID = &id
	}
	m.favorite = append(m.favorite, f)
	return f, nil
}

func (m *MemoryFavorites) RemoveFavorite(userID, videoID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.favorite[:0]
	for _, f := range m.favorite {
		if f.UserID == userID && f.VideoID == videoID {
			continue
		}
		kept = append(kept, f)
	}
	m.favorite = kept
	return nil
}

func (m *MemoryFavorites) IsFavorited(userID, videoID uint64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.favorite {
		if f.UserID == userID && f.VideoID == videoID {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryFavorites) CountFavorites(userID uint64) (int, error) {
	_, total, err := m.ListFavorites(userID, nil, 1, 0, true)
	return total, err
}

func (m *MemoryFavorites) ListFavorites(userID uint64, folderID *uint64, limit, offset int, includeMembership bool) ([]FavoriteVideo, int, error) {
	limit, offset = clampPage(limit, offset, favoriteDefaultLimit, favoriteMaxLimit)
	m.mu.Lock()
	var rows []Favorite
	for _, f := range m.favorite {
		if f.UserID != userID {
			continue
		}
		if folderID != nil && (f.FolderID == nil || *f.FolderID != *folderID) {
			continue
		}
		v, ok := m.Videos.Visible(f.VideoID)
		if !ok {
			continue
		}
		if !includeMembership && v.MembershipRequired {
			continue
		}
		rows = append(rows, f)
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
		return []FavoriteVideo{}, total, nil
	}
	rows = rows[offset:]
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]FavoriteVideo, 0, len(rows))
	for _, f := range rows {
		v, _ := m.Videos.Visible(f.VideoID)
		out = append(out, FavoriteVideo{Video: v, FolderID: f.FolderID, FavoritedAt: f.CreatedAt})
	}
	return out, total, nil
}
