package store

import (
	"sync"

	"github.com/tycdn/vplayer/internal/model"
)

// libraryVideoColumns mirrors the column order scanVideo expects, prefixed with the videos alias "v".
// Favorites and history join videos with another table that also has id/created_at, so every
// column must be qualified.
const libraryVideoColumns = `
		v.id, COALESCE(v.source_system,''), COALESCE(v.source_id,''), v.category_id, COALESCE(v.source_category_id,''),
		COALESCE(v.leaf_source_id,''),
		v.title, COALESCE(v.description,''), COALESCE(v.year,''), COALESCE(v.area,''), COALESCE(v.director,''),
		COALESCE(v.actors,''), COALESCE(v.rating,0), v.cover_key, COALESCE(v.cover_r2_key,''), COALESCE(v.poster_source_url,''),
		v.duration_sec, v.view_count, v.hls_master_key, COALESCE(v.playback_source,''), COALESCE(v.play_sid,1), COALESCE(v.play_nid,1),
		COALESCE(v.playback_url,''), v.source_updated_at, v.last_synced_at, COALESCE(v.sync_status,'ok'),
		COALESCE(v.is_active,1), v.status, v.created_at`

// libraryVideoVisible limits library lists to videos the public site can show.
const libraryVideoVisible = `v.status IN ('ready','demo') AND COALESCE(v.is_active,1)=1`

// scanVideoExtra scans a videos row (libraryVideoColumns) followed by extra destinations.
type extraScan struct {
	row   scannable
	extra []any
}

func (e extraScan) Scan(dest ...any) error {
	return e.row.Scan(append(dest, e.extra...)...)
}

func scanVideoWithExtra(row scannable, extra ...any) (*model.Video, error) {
	return scanVideo(extraScan{row: row, extra: extra})
}

// MemoryVideoCatalog is the in-memory video set shared by the library memory repos.
type MemoryVideoCatalog struct {
	mu     sync.Mutex
	videos map[uint64]model.Video
}

func NewMemoryVideoCatalog() *MemoryVideoCatalog {
	return &MemoryVideoCatalog{videos: map[uint64]model.Video{}}
}

// AddVideo registers a video. Status defaults to "ready" and IsActive to true when Status is empty.
func (c *MemoryVideoCatalog) AddVideo(v model.Video) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v.Status == "" {
		v.Status = "ready"
		v.IsActive = true
	}
	c.videos[v.ID] = v
}

func (c *MemoryVideoCatalog) Get(id uint64) (model.Video, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.videos[id]
	return v, ok
}

// Exists mirrors MySQL VideoExists: any row counts regardless of status.
func (c *MemoryVideoCatalog) Exists(id uint64) bool {
	_, ok := c.Get(id)
	return ok
}

// Visible mirrors libraryVideoVisible.
func (c *MemoryVideoCatalog) Visible(id uint64) (model.Video, bool) {
	v, ok := c.Get(id)
	if !ok || !v.IsActive || (v.Status != "ready" && v.Status != "demo") {
		return model.Video{}, false
	}
	return v, true
}

func clampPage(limit, offset, def, max int) (int, int) {
	if limit <= 0 {
		limit = def
	}
	if limit > max {
		limit = max
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
