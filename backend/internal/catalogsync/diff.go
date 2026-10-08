package catalogsync

import (
	"strings"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

// VideoCatalogChanged reports whether meaningful video catalog fields differ.
func VideoCatalogChanged(existing *model.Video, in store.VideoUpsert) bool {
	if existing == nil {
		return true
	}
	return existing.Title != in.Title ||
		existing.Description != in.Description ||
		existing.CategoryID != in.CategoryID ||
		existing.SourceCategoryID != in.SourceCategoryID ||
		existing.Year != in.Year ||
		existing.Area != in.Area ||
		existing.Director != in.Director ||
		existing.Actors != in.Actors ||
		existing.Rating != in.Rating ||
		existing.PosterSourceURL != in.PosterSourceURL ||
		existing.PlaybackURL != in.PlaybackURL ||
		existing.PlaybackSource != in.PlaybackSource ||
		existing.PlaySID != in.PlaySID ||
		existing.PlayNID != in.PlayNID ||
		existing.ViewCount != in.ViewCount ||
		existing.DurationSec != in.DurationSec ||
		existing.IsActive != in.IsActive ||
		existing.Status != in.Status
}

// EpisodeCatalogChanged reports whether meaningful episode source fields differ.
func EpisodeCatalogChanged(existing *store.EpisodeSyncRow, ep olehdtv.SourceEpisode) bool {
	if existing == nil {
		return true
	}
	status := ep.PlaybackStatus
	if status == "" {
		status = "ok"
	}
	return existing.Title != ep.Title ||
		existing.PlaybackSource != ep.PlaybackSource ||
		existing.PlaybackURL != ep.PlaybackURL ||
		existing.PlayPageURL != ep.PlayPageURL ||
		existing.PlaybackStatus != status ||
		existing.IsActive != (ep.PlaybackURL != "" && status != "unavailable")
}

// EpisodeMediaSourceChanged is true when playback_url changed on an already-migrated episode.
func EpisodeMediaSourceChanged(existing *store.EpisodeSyncRow, ep olehdtv.SourceEpisode) bool {
	if existing == nil {
		return false
	}
	if existing.PlaybackURL == ep.PlaybackURL {
		return false
	}
	st := strings.ToLower(strings.TrimSpace(existing.MigrationStatus))
	return st == "done" || st == "migrated" || existing.HlsObjectKey != ""
}
