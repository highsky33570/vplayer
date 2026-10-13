package httpapi

import (
	"testing"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
)

func TestWithCoverURL_skipsKnownOleHdTvPlaceholder(t *testing.T) {
	api := &API{Cfg: config.Config{}}
	const placeholder = "https://static.olelive.com/upload/site/20201117-1/105e22b56101eb0268ea9904972357ba.png"
	const real = "https://static.olelive.com/upload/vod/20260712-1/abc.jpg"

	got := api.withCoverURL(model.Video{Title: "t", PosterSourceURL: placeholder})
	if got.CoverURL != "" {
		t.Fatalf("placeholder cover_url=%q want empty", got.CoverURL)
	}

	got = api.withCoverURL(model.Video{Title: "t", PosterSourceURL: real})
	if got.CoverURL != real {
		t.Fatalf("real poster cover_url=%q want %q", got.CoverURL, real)
	}

	// R2/CDN key still wins over a bad PosterSourceURL.
	got = api.withCoverURL(model.Video{
		Title:           "t",
		CoverR2Key:      "https://cdn.example/posters/1.jpg",
		PosterSourceURL: placeholder,
	})
	if got.CoverURL != "https://cdn.example/posters/1.jpg" {
		t.Fatalf("cdn precedence cover_url=%q", got.CoverURL)
	}
}
