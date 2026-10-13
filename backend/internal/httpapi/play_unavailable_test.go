package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/play"
	"github.com/tycdn/vplayer/internal/store"
)

type episodeStateMap struct {
	byKey map[string]store.EpisodePlaybackState
}

func (m episodeStateMap) GetEpisodePlaybackState(videoID uint64, sid, nid int) (store.EpisodePlaybackState, error) {
	st, ok := m.byKey[fmt.Sprintf("%d:%d:%d", videoID, sid, nid)]
	if !ok {
		return store.EpisodePlaybackState{}, nil
	}
	st.Found = true
	return st, nil
}

func TestResolvePlay_playbackUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const authSecret = "test-auth-secret"
	api := &API{
		Cfg: config.Config{AuthSecret: authSecret, CDNSignSecret: "cdn"},
		Resolver: &play.Resolver{
			Cfg: config.Config{CDNSignSecret: "cdn"},
			VideoByID: func(id uint64) (*model.Video, error) {
				return &model.Video{ID: id, Title: "restricted", Status: "ready", IsActive: true, PlaySID: 1, PlayNID: 1}, nil
			},
			EpisodeState: episodeStateMap{byKey: map[string]store.EpisodePlaybackState{
				"9870:1:1": {PlaybackStatus: "unavailable", PlaybackURL: ""},
				"9870:1:4": {PlaybackStatus: "pending_enrichment", PlaybackURL: ""},
			}},
		},
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/videos/:id/play", api.RequireUser, api.ResolvePlay)

	tok, err := auth.IssueToken(authSecret, 1, "u@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/9870/play", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != false || body["error"] != "playback_unavailable" {
		t.Fatalf("%v", body)
	}
	if body["message"] != play.UserMessagePlaybackUnavailable {
		t.Fatalf("message=%v", body["message"])
	}
}

func TestResolvePlay_unknownStillNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const authSecret = "test-auth-secret"
	api := &API{
		Cfg: config.Config{AuthSecret: authSecret, CDNSignSecret: "cdn"},
		Resolver: &play.Resolver{
			Cfg: config.Config{CDNSignSecret: "cdn"},
			VideoByID: func(id uint64) (*model.Video, error) {
				return &model.Video{ID: id, Status: "ready", IsActive: true, PlaySID: 1, PlayNID: 1}, nil
			},
			EpisodeState: episodeStateMap{byKey: map[string]store.EpisodePlaybackState{
				"1:1:1": {PlaybackStatus: "pending_enrichment", PlaybackURL: ""},
			}},
		},
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/videos/:id/play", api.RequireUser, api.ResolvePlay)
	tok, err := auth.IssueToken(authSecret, 1, "u@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] == "playback_unavailable" {
		t.Fatal("pending must not map to playback_unavailable")
	}
	if body["message"] != "no playback url" {
		t.Fatalf("message=%v", body["message"])
	}
}
