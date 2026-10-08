package mediamigrate

import (
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type PlaylistKind string

const (
	PlaylistMedia  PlaylistKind = "media"
	PlaylistMaster PlaylistKind = "master"
	PlaylistUnknown PlaylistKind = "unknown"
)

// SegmentRef is one media URI from a media playlist.
type SegmentRef struct {
	SourceURL string // absolute URL used to download
	LocalName string // relative name written into our playlist
	LineIndex int
}

// ParsedPlaylist is a parsed HLS playlist (master or media).
type ParsedPlaylist struct {
	Kind     PlaylistKind
	Raw      string
	Segments []SegmentRef
	// MasterRefs are child playlist URIs when Kind == master.
	MasterRefs []string
}

// DetectPlaylistKind returns master if #EXT-X-STREAM-INF / #EXT-X-I-FRAME-STREAM-INF present.
func DetectPlaylistKind(body string) PlaylistKind {
	for _, line := range splitLines(body) {
		u := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(u, "#EXT-X-STREAM-INF") || strings.HasPrefix(u, "#EXT-X-I-FRAME-STREAM-INF") {
			return PlaylistMaster
		}
	}
	if strings.Contains(strings.ToUpper(body), "#EXTINF") {
		return PlaylistMedia
	}
	return PlaylistUnknown
}

// ResolvePlaylistURL joins a playlist-relative or absolute URI against the playlist URL.
func ResolvePlaylistURL(playlistURL, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty playlist URI")
	}
	base, err := url.Parse(playlistURL)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(u).String(), nil
}

// ParseMediaPlaylist extracts segment URIs from a media playlist and resolves them.
// Master playlists return Kind=master with MasterRefs filled and an error from callers expected.
func ParseMediaPlaylist(playlistURL, body string) (*ParsedPlaylist, error) {
	kind := DetectPlaylistKind(body)
	out := &ParsedPlaylist{Kind: kind, Raw: body}
	lines := splitLines(body)

	if kind == PlaylistMaster {
		for i := 0; i < len(lines); i++ {
			u := strings.ToUpper(strings.TrimSpace(lines[i]))
			if strings.HasPrefix(u, "#EXT-X-STREAM-INF") || strings.HasPrefix(u, "#EXT-X-I-FRAME-STREAM-INF") {
				if i+1 < len(lines) {
					next := strings.TrimSpace(lines[i+1])
					if next != "" && !strings.HasPrefix(next, "#") {
						abs, err := ResolvePlaylistURL(playlistURL, next)
						if err != nil {
							return nil, err
						}
						out.MasterRefs = append(out.MasterRefs, abs)
					}
				}
			}
		}
		return out, nil
	}

	segIdx := 0
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		abs, err := ResolvePlaylistURL(playlistURL, trim)
		if err != nil {
			return nil, fmt.Errorf("segment %d: %w", segIdx, err)
		}
		ext := path.Ext(stripQuery(abs))
		if ext == "" {
			ext = ".ts"
		}
		name := fmt.Sprintf("segment%03d%s", segIdx, ext)
		out.Segments = append(out.Segments, SegmentRef{
			SourceURL: abs,
			LocalName: name,
			LineIndex: i,
		})
		segIdx++
	}
	if kind == PlaylistMedia && len(out.Segments) == 0 {
		return nil, fmt.Errorf("media playlist has no segments")
	}
	return out, nil
}

// RewriteMediaPlaylist replaces media URI lines with LocalName values in order.
func RewriteMediaPlaylist(body string, segments []SegmentRef) (string, error) {
	lines := splitLines(body)
	byLine := map[int]string{}
	for _, s := range segments {
		byLine[s.LineIndex] = s.LocalName
	}
	var b strings.Builder
	uriOrdinal := 0
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim != "" && !strings.HasPrefix(trim, "#") {
			if name, ok := byLine[i]; ok {
				b.WriteString(name)
			} else if uriOrdinal < len(segments) {
				// Fallback by ordinal if LineIndex drifted.
				b.WriteString(segments[uriOrdinal].LocalName)
			} else {
				return "", fmt.Errorf("rewrite: unexpected media URI at line %d", i+1)
			}
			uriOrdinal++
		} else {
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	if uriOrdinal != len(segments) {
		return "", fmt.Errorf("rewrite: replaced %d URIs, expected %d", uriOrdinal, len(segments))
	}
	return b.String(), nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func stripQuery(u string) string {
	if i := strings.Index(u, "?"); i >= 0 {
		return u[:i]
	}
	return u
}

// RedactURL returns host + path without query/fragment for safe logs.
func RedactURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "(url)"
	}
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	return u.Scheme + "://" + u.Host + p
}

// EpisodePrefix is videos/<videoID>/<episodeID>/
func EpisodePrefix(videoID, episodeID uint64) string {
	return "videos/" + strconv.FormatUint(videoID, 10) + "/" + strconv.FormatUint(episodeID, 10) + "/"
}
