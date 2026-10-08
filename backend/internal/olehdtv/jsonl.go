package olehdtv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// JSONL file names under an import root (categories.json + videos/episodes JSONL).
const (
	JSONLCategoriesFile = "categories.json"
	JSONLVideosFile     = "videos.jsonl"
	JSONLEpisodesFile   = "episodes.jsonl"
)

type jsonlCategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type jsonlVideoMeta struct {
	Year       string   `json:"year"`
	Area       string   `json:"area"`
	Type       string   `json:"type"`
	Status     string   `json:"status"`
	Permission string   `json:"permission"`
	Rating     string   `json:"rating"`
	Director   []string `json:"director"`
	Actors     []string `json:"actors"`
}

type jsonlVideo struct {
	VodID        string         `json:"vod_id"`
	CategoryID   int            `json:"category_id"`
	CategoryName string         `json:"category_name"`
	Title        string         `json:"title"`
	PosterURL    string         `json:"poster_url"`
	Description  string         `json:"description"`
	Metadata     jsonlVideoMeta `json:"metadata"`
	PlayURLs     []string       `json:"play_urls"`
	SourceURL    string         `json:"source_url"`
	CollectedAt  string         `json:"collected_at"`
}

type jsonlEpisode struct {
	VodID       string          `json:"vod_id"`
	SID         int             `json:"sid"`
	NID         int             `json:"nid"`
	Name        string          `json:"name"`
	PlayURL     string          `json:"play_url"`
	StreamURL   *string         `json:"stream_url"`
	Player      *string         `json:"player"`
	Server      *string         `json:"server"`
	PlayerData  json.RawMessage `json:"player_data"`
	HTMLFile    string          `json:"html_file"`
	CollectedAt string          `json:"collected_at"`
}

// JSONLValidation holds dry-run / preflight results (no DB writes).
type JSONLValidation struct {
	Root               string         `json:"root"`
	CategoryCount      int            `json:"category_count"`
	VideoCount         int            `json:"video_count"`
	EpisodeCount       int            `json:"episode_count"`
	VideosWithStream   int            `json:"videos_with_any_stream"`
	VideosWithoutStream int           `json:"videos_without_stream"`
	EpisodesWithStream int            `json:"episodes_with_stream"`
	EpisodesNoStream   int            `json:"episodes_without_stream"`
	DuplicateVodIDs    []string       `json:"duplicate_vod_ids"`
	DuplicateEpisodes  []string       `json:"duplicate_episode_keys"`
	OrphanEpisodes     int            `json:"orphan_episodes"`
	OrphanEpisodeSample []string      `json:"orphan_episode_sample,omitempty"`
	MissingCategory    int            `json:"videos_missing_category_id"`
	MissingTitle       int            `json:"videos_missing_title"`
	MissingPoster      int            `json:"videos_missing_poster"`
	MissingDescription int            `json:"videos_missing_description"`
	MissingPlayURLs    int            `json:"videos_missing_play_urls"`
	CategoryIDsInVideos map[string]int `json:"category_ids_in_videos"`
	ParseErrors        []string       `json:"parse_errors,omitempty"`
	OK                 bool           `json:"ok"`
}

// CategorySlugForName maps OLEHDTV Chinese names onto VPlayer seed/nav slugs.
func CategorySlugForName(name string, sourceID string) string {
	switch strings.TrimSpace(name) {
	case "电影":
		return "movie"
	case "连续剧", "剧集":
		return "tv"
	case "综艺":
		return "variety"
	case "动漫":
		return "anime"
	case "午夜影院":
		return "midnight"
	case "VIP蓝光影院":
		return "vip-bluray"
	case "体育直播":
		return "sports"
	case "短剧":
		return "short-drama"
	default:
		if sourceID != "" {
			return "type-" + sourceID
		}
		return "cat"
	}
}

func CategorySortForSourceID(id int) int {
	switch id {
	case 1:
		return 10
	case 2:
		return 20
	case 3:
		return 30
	case 4:
		return 40
	case 5:
		return 50
	case 6:
		return 60
	case 13:
		return 70
	case 14:
		return 80
	default:
		if id > 0 {
			return id * 10
		}
		return 100
	}
}

// ResolveImportRoot picks the directory containing the three import files.
// Prefers olehdtv-import, then olehdtv-html, under the given candidates.
func ResolveImportRoot(explicit string) (string, error) {
	if explicit != "" {
		if err := assertImportRoot(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	candidates := []string{
		filepath.Join("..", "data", "olehdtv-import"),
		filepath.Join("..", "data", "olehdtv-html"),
		filepath.Join("data", "olehdtv-import"),
		filepath.Join("data", "olehdtv-html"),
		filepath.Join("F:", "VPlayer", "data", "olehdtv-import"),
		filepath.Join("F:", "VPlayer", "data", "olehdtv-html"),
	}
	var tried []string
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			tried = append(tried, c)
			continue
		}
		if err := assertImportRoot(abs); err == nil {
			return abs, nil
		}
		tried = append(tried, abs)
	}
	return "", fmt.Errorf("import root not found (need categories.json, videos.jsonl, episodes.jsonl); tried: %s", strings.Join(tried, ", "))
}

func assertImportRoot(root string) error {
	for _, name := range []string{JSONLCategoriesFile, JSONLVideosFile, JSONLEpisodesFile} {
		st, err := os.Stat(filepath.Join(root, name))
		if err != nil || st.IsDir() {
			return fmt.Errorf("%s missing under %s", name, root)
		}
	}
	return nil
}

// LoadJSONLCategories reads categories.json into SourceCategory rows.
func LoadJSONLCategories(root string) ([]SourceCategory, error) {
	b, err := os.ReadFile(filepath.Join(root, JSONLCategoriesFile))
	if err != nil {
		return nil, err
	}
	var raw []jsonlCategory
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("categories.json: %w", err)
	}
	out := make([]SourceCategory, 0, len(raw))
	for _, c := range raw {
		id := strconv.Itoa(c.ID)
		out = append(out, SourceCategory{
			SourceID: id,
			Name:     c.Name,
			Slug:     CategorySlugForName(c.Name, id),
			Sort:     CategorySortForSourceID(c.ID),
		})
	}
	return out, nil
}

func parseRating(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func joinNames(parts []string) string {
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, ",")
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func streamURLValue(p *string) string {
	return derefStr(p)
}

func parseCollectedAt(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t
	}
	return time.Time{}
}

func playbackSourceFromEpisode(e jsonlEpisode) string {
	if v := derefStr(e.Player); v != "" {
		return v
	}
	if len(e.PlayerData) == 0 || string(e.PlayerData) == "null" {
		return ""
	}
	var pd struct {
		From string `json:"from"`
	}
	if err := json.Unmarshal(e.PlayerData, &pd); err == nil && pd.From != "" {
		return pd.From
	}
	return ""
}

// LoadEpisodesByVod streams episodes.jsonl into a map keyed by vod_id.
func LoadEpisodesByVod(root string) (map[string][]SourceEpisode, *JSONLValidation, error) {
	path := filepath.Join(root, JSONLEpisodesFile)
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	val := &JSONLValidation{Root: root, CategoryIDsInVideos: map[string]int{}}
	byVod := make(map[string][]SourceEpisode, 10000)
	seen := make(map[string]struct{}, 40000)
	sc := bufio.NewScanner(f)
	// episodes lines can be large (player_data)
	buf := make([]byte, 0, 1024*1024)
	sc.Buffer(buf, 8*1024*1024)

	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e jsonlEpisode
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			val.ParseErrors = append(val.ParseErrors, fmt.Sprintf("episodes.jsonl:%d: %v", lineNo, err))
			continue
		}
		vodID := strings.TrimSpace(e.VodID)
		if vodID == "" || e.SID <= 0 || e.NID <= 0 {
			val.ParseErrors = append(val.ParseErrors, fmt.Sprintf("episodes.jsonl:%d: invalid identity vod_id=%q sid=%d nid=%d", lineNo, e.VodID, e.SID, e.NID))
			continue
		}
		key := fmt.Sprintf("%s/%d/%d", vodID, e.SID, e.NID)
		if _, ok := seen[key]; ok {
			val.DuplicateEpisodes = append(val.DuplicateEpisodes, key)
			continue
		}
		seen[key] = struct{}{}
		val.EpisodeCount++

		stream := streamURLValue(e.StreamURL)
		status := "ok"
		if stream == "" {
			status = "unavailable"
			val.EpisodesNoStream++
		} else {
			val.EpisodesWithStream++
		}
		byVod[vodID] = append(byVod[vodID], SourceEpisode{
			SID:            e.SID,
			NID:            e.NID,
			Title:          strings.TrimSpace(e.Name),
			PlaybackSource: playbackSourceFromEpisode(e),
			PlaybackURL:    stream,
			PlayPageURL:    strings.TrimSpace(e.PlayURL),
			PlaybackStatus: status,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, val, err
	}
	return byVod, val, nil
}

// ValidateJSONLImport scans all three files without writing to DB/R2.
func ValidateJSONLImport(root string) (*JSONLValidation, error) {
	if err := assertImportRoot(root); err != nil {
		return nil, err
	}
	cats, err := LoadJSONLCategories(root)
	if err != nil {
		return nil, err
	}
	epsByVod, val, err := LoadEpisodesByVod(root)
	if err != nil {
		return nil, err
	}
	val.CategoryCount = len(cats)
	catSet := map[string]struct{}{}
	for _, c := range cats {
		catSet[c.SourceID] = struct{}{}
	}

	path := filepath.Join(root, JSONLVideosFile)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seenVod := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	sc.Buffer(buf, 4*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v jsonlVideo
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			val.ParseErrors = append(val.ParseErrors, fmt.Sprintf("videos.jsonl:%d: %v", lineNo, err))
			continue
		}
		vodID := strings.TrimSpace(v.VodID)
		if vodID == "" {
			val.ParseErrors = append(val.ParseErrors, fmt.Sprintf("videos.jsonl:%d: missing vod_id", lineNo))
			continue
		}
		if _, ok := seenVod[vodID]; ok {
			val.DuplicateVodIDs = append(val.DuplicateVodIDs, vodID)
			continue
		}
		seenVod[vodID] = struct{}{}
		val.VideoCount++

		catKey := strconv.Itoa(v.CategoryID)
		val.CategoryIDsInVideos[catKey]++
		if v.CategoryID == 0 {
			val.MissingCategory++
		} else if _, ok := catSet[catKey]; !ok {
			val.MissingCategory++
		}
		if strings.TrimSpace(v.Title) == "" {
			val.MissingTitle++
		}
		if strings.TrimSpace(v.PosterURL) == "" {
			val.MissingPoster++
		}
		if strings.TrimSpace(v.Description) == "" {
			val.MissingDescription++
		}
		if len(v.PlayURLs) == 0 {
			val.MissingPlayURLs++
		}

		eps := epsByVod[vodID]
		hasStream := false
		for _, ep := range eps {
			if ep.PlaybackURL != "" {
				hasStream = true
				break
			}
		}
		if hasStream {
			val.VideosWithStream++
		} else {
			val.VideosWithoutStream++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	for vodID, eps := range epsByVod {
		if _, ok := seenVod[vodID]; ok {
			continue
		}
		val.OrphanEpisodes += len(eps)
		if len(val.OrphanEpisodeSample) < 10 {
			val.OrphanEpisodeSample = append(val.OrphanEpisodeSample, vodID)
		}
	}

	val.OK = len(val.DuplicateVodIDs) == 0 &&
		len(val.DuplicateEpisodes) == 0 &&
		len(val.ParseErrors) == 0 &&
		val.MissingCategory == 0 &&
		val.MissingTitle == 0 &&
		val.VideoCount > 0
	return val, nil
}

// VideoFromJSONL maps one videos.jsonl record + its episodes into SourceVideo.
func VideoFromJSONL(v jsonlVideo, eps []SourceEpisode) SourceVideo {
	return SourceVideo{
		SourceID:         strings.TrimSpace(v.VodID),
		SourceCategoryID: strconv.Itoa(v.CategoryID),
		Title:            strings.TrimSpace(v.Title),
		Description:      strings.TrimSpace(v.Description),
		Year:             strings.TrimSpace(v.Metadata.Year),
		Area:             strings.TrimSpace(v.Metadata.Area),
		Director:         joinNames(v.Metadata.Director),
		Actors:           joinNames(v.Metadata.Actors),
		Rating:           parseRating(v.Metadata.Rating),
		PosterURL:        strings.TrimSpace(v.PosterURL),
		SourceUpdatedAt:  parseCollectedAt(v.CollectedAt),
		IsActive:         true,
		Episodes:         eps,
		DetailURL:        strings.TrimSpace(v.SourceURL),
		StatusText:       strings.TrimSpace(v.Metadata.Status),
	}
}

// IterVideosJSONL streams videos.jsonl and yields SourceVideo with episodes attached.
func IterVideosJSONL(root string, epsByVod map[string][]SourceEpisode, fn func(SourceVideo, int) error) error {
	path := filepath.Join(root, JSONLVideosFile)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	sc.Buffer(buf, 4*1024*1024)
	lineNo := 0
	idx := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v jsonlVideo
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return fmt.Errorf("videos.jsonl:%d: %w", lineNo, err)
		}
		idx++
		eps := epsByVod[strings.TrimSpace(v.VodID)]
		if err := fn(VideoFromJSONL(v, eps), idx); err != nil {
			return err
		}
	}
	return sc.Err()
}
