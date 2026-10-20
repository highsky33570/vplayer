package model

import "time"

type Category struct {
	ID           uint64     `json:"id"`
	ParentID     *uint64    `json:"parent_id"` // nil => top-level; JSON null when unset
	Name         string     `json:"name"`
	Slug         string     `json:"slug"`
	Sort         int        `json:"sort"`
	SourceSystem string     `json:"source_system,omitempty"`
	SourceID     string     `json:"source_id,omitempty"`
	IsActive     bool       `json:"is_active"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Video struct {
	ID               uint64     `json:"id"`
	SourceSystem     string     `json:"source_system,omitempty"`
	SourceID         string     `json:"source_id,omitempty"`
	// CategoryID is canonical/top-level VPlayer ownership (homepage sections, midnight exclude).
	CategoryID uint64 `json:"category_id"`
	// SourceCategoryID is existing source/provenance (listing type used for ownership repair).
	// Unchanged by child-browse membership.
	SourceCategoryID string `json:"source_category_id,omitempty"`
	// LeafSourceID is optional OLEHDTV child type id (e.g. "101") for child-category browse.
	// Empty means unknown/unset. Never aliases VIP 601↔ movie 101.
	LeafSourceID string `json:"leaf_source_id,omitempty"`
	Title        string `json:"title"`
	Description      string     `json:"description"`
	Year             string     `json:"year,omitempty"`
	Area             string     `json:"area,omitempty"`
	Director         string     `json:"director,omitempty"`
	Actors           string     `json:"actors,omitempty"`
	Rating           float64    `json:"rating,omitempty"`
	CoverKey         string     `json:"-"`
	CoverR2Key       string     `json:"-"`
	StorageProvider  string     `json:"storage_provider,omitempty"`
	PosterSourceURL  string     `json:"poster_source_url,omitempty"`
	CoverURL         string     `json:"cover_url"`
	DurationSec      int        `json:"duration_sec"`
	ViewCount        int64      `json:"view_count"`
	HLSMasterKey     string     `json:"-"`
	PlaybackSource   string     `json:"playback_source,omitempty"`
	PlaySID          int        `json:"play_sid,omitempty"`
	PlayNID          int        `json:"play_nid,omitempty"`
	PlaybackURL      string     `json:"-"`
	SourceUpdatedAt  *time.Time `json:"source_updated_at,omitempty"`
	LastSyncedAt     *time.Time `json:"last_synced_at,omitempty"`
	SyncStatus       string     `json:"sync_status,omitempty"`
	IsActive         bool       `json:"is_active"`
	Status           string     `json:"status"` // draft|ready|disabled|demo
	CreatedAt        time.Time  `json:"created_at"`
}

type Episode struct {
	ID              uint64     `json:"id"`
	VideoID         uint64     `json:"video_id"`
	SID             int        `json:"sid"`
	NID             int        `json:"nid"`
	Title           string     `json:"title"`
	PlaybackSource  string     `json:"playback_source"`
	PlaybackURL     string     `json:"-"`
	HLSObjectKey    string     `json:"-"`
	StorageProvider string     `json:"storage_provider,omitempty"`
	MigrationStatus string     `json:"migration_status,omitempty"`
	MigrationError  string     `json:"migration_error,omitempty"`
	MigratedAt      *time.Time `json:"migrated_at,omitempty"`
	PlaybackStatus  string     `json:"playback_status,omitempty"`
	IsActive        bool       `json:"is_active"`
}

type PlaySession struct {
	Ticket    string `json:"ticket"`
	M3U8URL   string `json:"m3u8_url"`
	ExpiresIn int    `json:"expires_in"`
}

// PlaybackInfo is the normalized play response for the frontend.
type PlaybackInfo struct {
	Type        string `json:"type"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	SID         int    `json:"sid"`
	NID         int    `json:"nid"`
	Ticket      string `json:"ticket,omitempty"`
	FallbackURL string `json:"fallback_url,omitempty"`
}

type SyncSummary struct {
	Scanned      int `json:"scanned"`
	Created      int `json:"created"`
	Updated      int `json:"updated"`
	Unchanged    int `json:"unchanged"`
	Failed       int `json:"failed"`
	Deactivated  int `json:"deactivated"`
	PosterOK     int `json:"poster_ok"`
	PosterFailed int `json:"poster_failed"`
}

type User struct {
	ID        uint64    `json:"id"`
	Email     string    `json:"email"`
	Nickname  string    `json:"nickname"`
	// AvatarHue is 0..359 for a chosen avatar color, or -1 for the automatic default.
	AvatarHue int       `json:"avatar_hue"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
