package olehdtv

import "time"

const SourceSystem = "olehdtv"

type SourceCategory struct {
	SourceID string
	Name     string
	Slug     string
	Sort     int
}

type SourcePlayback struct {
	SID            int
	NID            int
	Title          string
	PlaybackSource string
	PlaybackURL    string
}

type SourceEpisode struct {
	SID            int
	NID            int
	Title          string
	PlaybackSource string
	PlaybackURL    string
	PlayPageURL    string
	PlaybackStatus string // ok | unavailable
}

type SourceVideo struct {
	SourceID         string
	SourceCategoryID string
	Title            string
	Description      string
	Year             string
	Area             string
	Director         string
	Actors           string
	Rating           float64
	PosterURL        string
	ViewCount        int64
	DurationSec      int
	SourceUpdatedAt  time.Time
	IsActive         bool
	Episodes         []SourceEpisode
	DetailURL        string
	StatusText       string
}

type CatalogSnapshot struct {
	Categories []SourceCategory
	Videos     []SourceVideo
}
