package play

import "errors"

// ErrPlaybackUnavailable means the video is catalog-visible but has no resolvable
// stream after confirmed source unavailability (e.g. OLEHDTV membership restriction
// persisted as playback_status=unavailable). Safe for user-facing copy.
var ErrPlaybackUnavailable = errors.New("playback_unavailable")

// UserMessagePlaybackUnavailable is the exact watch-page copy for confirmed unavailability.
const UserMessagePlaybackUnavailable = "This video is currently unavailable for playback."
