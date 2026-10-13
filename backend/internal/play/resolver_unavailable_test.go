package play

import (
	"errors"
	"testing"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

type memAvail struct {
	byID map[uint64]store.VideoPlaybackAvailability
}

func (m memAvail) GetVideoPlaybackAvailability(videoID uint64) (store.VideoPlaybackAvailability, error) {
	return m.byID[videoID], nil
}

func TestResolver_confirmedUnavailable(t *testing.T) {
	r := &Resolver{
		Cfg: config.Config{},
		Availability: memAvail{byID: map[uint64]store.VideoPlaybackAvailability{
			1: {PlayableCount: 0, UnavailableCount: 1, PendingCount: 0},
			2: {PlayableCount: 0, UnavailableCount: 1, PendingCount: 1},
			3: {PlayableCount: 1, UnavailableCount: 1, PendingCount: 0},
		}},
	}
	if !r.confirmedUnavailable(1) {
		t.Fatal("want confirmed unavailable")
	}
	if r.confirmedUnavailable(2) {
		t.Fatal("pending must not confirm unavailable")
	}
	if r.confirmedUnavailable(3) {
		t.Fatal("playable must not confirm unavailable")
	}
}

func TestResolver_Resolve_unavailableVsUnknown(t *testing.T) {
	base := func(av store.VideoPlaybackAvailability) *Resolver {
		return &Resolver{
			Cfg:          config.Config{CDNSignSecret: "s"},
			Availability: memAvail{byID: map[uint64]store.VideoPlaybackAvailability{9: av}},
			VideoByID: func(id uint64) (*model.Video, error) {
				return &model.Video{ID: id, Status: "ready"}, nil
			},
		}
	}
	_, err := base(store.VideoPlaybackAvailability{UnavailableCount: 1}).Resolve(9, 0, 0)
	if !IsPlaybackUnavailable(err) {
		t.Fatalf("err=%v", err)
	}
	_, err = base(store.VideoPlaybackAvailability{PendingCount: 1}).Resolve(9, 0, 0)
	if err == nil || IsPlaybackUnavailable(err) || err.Error() != "no playback url" {
		t.Fatalf("err=%v", err)
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
