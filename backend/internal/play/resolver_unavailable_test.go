package play

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

type memEpisodeState struct {
	// key: "videoID:sid:nid"
	byKey map[string]store.EpisodePlaybackState
}

func (m memEpisodeState) GetEpisodePlaybackState(videoID uint64, sid, nid int) (store.EpisodePlaybackState, error) {
	st, ok := m.byKey[fmt.Sprintf("%d:%d:%d", videoID, sid, nid)]
	if !ok {
		return store.EpisodePlaybackState{}, nil
	}
	st.Found = true
	return st, nil
}

func TestResolver_targetEpisodeUnavailable_ignoresSiblingPending(t *testing.T) {
	r := &Resolver{
		Cfg: config.Config{},
		EpisodeState: memEpisodeState{byKey: map[string]store.EpisodePlaybackState{
			"9870:1:1": {PlaybackStatus: "unavailable", PlaybackURL: ""},
			"9870:1:4": {PlaybackStatus: "pending_enrichment", PlaybackURL: ""},
		}},
	}
	if !r.targetEpisodeUnavailable(9870, 1, 1) {
		t.Fatal("primary unavailable must confirm even when siblings are pending")
	}
	if r.targetEpisodeUnavailable(9870, 1, 4) {
		t.Fatal("pending episode must not be treated as unavailable")
	}
}

func TestResolver_Resolve_defaultUnavailableWithPendingSiblings(t *testing.T) {
	r := &Resolver{
		Cfg: config.Config{CDNSignSecret: "s"},
		VideoByID: func(id uint64) (*model.Video, error) {
			return &model.Video{ID: id, Status: "ready", PlaySID: 1, PlayNID: 1}, nil
		},
		EpisodeState: memEpisodeState{byKey: map[string]store.EpisodePlaybackState{
			"9870:1:1": {PlaybackStatus: "unavailable", PlaybackURL: ""},
			"9870:1:4": {PlaybackStatus: "pending_enrichment", PlaybackURL: ""},
		}},
	}
	_, err := r.Resolve(9870, 0, 0)
	if !IsPlaybackUnavailable(err) {
		t.Fatalf("err=%v want playback_unavailable", err)
	}
}

func TestResolver_Resolve_defaultPendingRemainsUnknown(t *testing.T) {
	r := &Resolver{
		Cfg: config.Config{CDNSignSecret: "s"},
		VideoByID: func(id uint64) (*model.Video, error) {
			return &model.Video{ID: id, Status: "ready", PlaySID: 1, PlayNID: 1}, nil
		},
		EpisodeState: memEpisodeState{byKey: map[string]store.EpisodePlaybackState{
			"9:1:1": {PlaybackStatus: "pending_enrichment", PlaybackURL: ""},
		}},
	}
	_, err := r.Resolve(9, 0, 0)
	if err == nil || IsPlaybackUnavailable(err) || err.Error() != "no playback url" {
		t.Fatalf("err=%v", err)
	}
}

func TestResolver_Resolve_playableStillWorks(t *testing.T) {
	r := &Resolver{
		Cfg: config.Config{CDNSignSecret: "s"},
		VideoByID: func(id uint64) (*model.Video, error) {
			return &model.Video{
				ID: id, Status: "ready", PlaySID: 1, PlayNID: 1,
				PlaybackURL: "https://cdn.example/ok.m3u8", PlaybackSource: "plyr",
			}, nil
		},
		EpisodeState: memEpisodeState{byKey: map[string]store.EpisodePlaybackState{
			"1:1:1": {PlaybackStatus: "unavailable", PlaybackURL: ""},
		}},
	}
	info, err := r.Resolve(1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.URL != "https://cdn.example/ok.m3u8" {
		t.Fatalf("%+v", info)
	}
}

func TestIsPlaybackUnavailable(t *testing.T) {
	if !IsPlaybackUnavailable(ErrPlaybackUnavailable) {
		t.Fatal()
	}
	if IsPlaybackUnavailable(errors.New("no playback url")) {
		t.Fatal()
	}
}

func TestUserMessagePlaybackUnavailable(t *testing.T) {
	if UserMessagePlaybackUnavailable != "This video is currently unavailable for playback." {
		t.Fatalf("%q", UserMessagePlaybackUnavailable)
	}
}
