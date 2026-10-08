package olehdtv

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// RecentMeta describes watermark quality and optional HTTP budget from discovery.
type RecentMeta struct {
	ReliableWatermark bool       `json:"reliable_watermark"`
	WatermarkField    string     `json:"watermark_field,omitempty"`
	MaxSourceUpdated  *time.Time `json:"max_source_updated_at,omitempty"`
	MaxSourceID       string     `json:"max_source_id,omitempty"`
	Note              string     `json:"note,omitempty"`

	// HTTP budget (populated by maccms_recent)
	ListingRequests   int  `json:"listing_requests,omitempty"`
	DetailRequests    int  `json:"detail_requests,omitempty"`
	PlayRequests      int  `json:"play_requests,omitempty"`
	TotalHTTPRequests int  `json:"total_http_requests,omitempty"`
	EnrichmentCapped  bool `json:"enrichment_capped,omitempty"`
	PlaySkipped       int  `json:"play_skipped,omitempty"`
}

// RecentSource discovers a bounded set of recent source videos (never a full catalog crawl).
type RecentSource interface {
	Name() string
	ListCategories(ctx context.Context) ([]SourceCategory, error)
	ListRecentVideos(ctx context.Context, limit int) ([]SourceVideo, RecentMeta, error)
}

// RecentSourceOpts configures NewRecentSource.
type RecentSourceOpts struct {
	Mode              string
	DSN               string
	ExportPath        string
	FixturePath       string
	HTMLDumpRoot      string
	BaseURL           string
	RequestDelayMs    int
	RequestTimeoutSec int
	MaxConcurrency    int
	MaxRetries        int
}

// NewRecentSource builds a bounded recent-discovery source from OLEHDTV_SOURCE_MODE.
// html_dump is rejected (it performs a full crawl).
func NewRecentSource(mode, dsn, exportPath, fixturePath, htmlDumpRoot, baseURL string) (RecentSource, error) {
	return NewRecentSourceOpts(RecentSourceOpts{
		Mode: mode, DSN: dsn, ExportPath: exportPath, FixturePath: fixturePath,
		HTMLDumpRoot: htmlDumpRoot, BaseURL: baseURL,
	})
}

// NewRecentSourceOpts builds a RecentSource with HTTP tuning for maccms_recent.
func NewRecentSourceOpts(opts RecentSourceOpts) (RecentSource, error) {
	_ = opts.HTMLDumpRoot
	switch opts.Mode {
	case "", "fixture":
		ad, err := NewFixtureAdapter(opts.FixturePath)
		if err != nil {
			return nil, err
		}
		return &fixtureRecent{FixtureAdapter: ad}, nil
	case "export_file", "export":
		ad, err := NewExportFileAdapter(opts.ExportPath)
		if err != nil {
			return nil, err
		}
		return &fixtureRecent{FixtureAdapter: ad}, nil
	case "maccms_db", "db":
		ad, err := NewMacCMSDBAdapter(opts.DSN)
		if err != nil {
			return nil, err
		}
		return &maccmsRecent{MacCMSDBAdapter: ad}, nil
	case "maccms_recent":
		return NewMacCMSHTTPRecent(opts.BaseURL, opts.RequestDelayMs, opts.RequestTimeoutSec, opts.MaxConcurrency, opts.MaxRetries)
	case "html_dump", "maccms_html", "dump":
		return nil, fmt.Errorf("OLEHDTV_SOURCE_MODE=%q is not supported for incremental sync (full crawl). Use maccms_recent, maccms_db, export_file, or fixture", opts.Mode)
	default:
		return nil, fmt.Errorf("unknown OLEHDTV_SOURCE_MODE %q for incremental sync (supported: fixture, export_file, maccms_db, maccms_recent)", opts.Mode)
	}
}

type fixtureRecent struct {
	*FixtureAdapter
}

func (a *fixtureRecent) ListRecentVideos(ctx context.Context, limit int) ([]SourceVideo, RecentMeta, error) {
	_ = ctx
	if limit <= 0 {
		return nil, RecentMeta{}, fmt.Errorf("limit must be > 0")
	}
	all := append([]SourceVideo(nil), a.data.Videos...)
	sort.SliceStable(all, func(i, j int) bool {
		ti, tj := all[i].SourceUpdatedAt, all[j].SourceUpdatedAt
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return sourceIDGreater(all[i].SourceID, all[j].SourceID)
	})
	if limit > len(all) {
		limit = len(all)
	}
	out := all[:limit]
	meta := recentMetaFromVideos(out, "fixture/export: sorted by source_updated_at then source_id; not a live feed watermark")
	return out, meta, nil
}

type maccmsRecent struct {
	*MacCMSDBAdapter
}

func (a *maccmsRecent) ListRecentVideos(ctx context.Context, limit int) ([]SourceVideo, RecentMeta, error) {
	if limit <= 0 {
		return nil, RecentMeta{}, fmt.Errorf("limit must be > 0")
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT vod_id, type_id, vod_name, vod_content, vod_year, vod_area,
		       vod_director, vod_actor, vod_score, vod_pic, vod_hits,
		       vod_time, vod_play_from, vod_play_url, vod_status
		FROM mac_vod
		ORDER BY vod_time DESC, vod_id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, RecentMeta{}, fmt.Errorf("mac_vod recent: %w", err)
	}
	defer rows.Close()
	var out []SourceVideo
	for rows.Next() {
		var (
			id, typeID, hits, status                                  int64
			name, content, year, area, director, actor, pic, playFrom string
			playURL                                                   string
			score                                                     float64
			vodTime                                                   sql.NullTime
		)
		if err := rows.Scan(&id, &typeID, &name, &content, &year, &area, &director, &actor, &score, &pic, &hits, &vodTime, &playFrom, &playURL, &status); err != nil {
			return nil, RecentMeta{}, err
		}
		updated := time.Time{}
		if vodTime.Valid {
			updated = vodTime.Time
		}
		out = append(out, SourceVideo{
			SourceID:         fmt.Sprintf("%d", id),
			SourceCategoryID: fmt.Sprintf("%d", typeID),
			Title:            name,
			Description:      content,
			Year:             year,
			Area:             area,
			Director:         director,
			Actors:           actor,
			Rating:           score,
			PosterURL:        pic,
			ViewCount:        hits,
			SourceUpdatedAt:  updated,
			IsActive:         status == 1,
			Episodes:         parseMacPlayURL(playFrom, playURL),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, RecentMeta{}, err
	}
	meta := recentMetaFromVideos(out, "maccms_db: ORDER BY vod_time DESC, vod_id DESC")
	if meta.ReliableWatermark {
		meta.WatermarkField = "source_updated_at"
		meta.Note = "vod_time available; watermark may be recorded in sync_runs.summary_json after successful real runs"
	}
	return out, meta, nil
}

func recentMetaFromVideos(videos []SourceVideo, note string) RecentMeta {
	meta := RecentMeta{Note: note}
	var maxT time.Time
	var maxID string
	allHaveTime := len(videos) > 0
	for _, v := range videos {
		if v.SourceUpdatedAt.IsZero() {
			allHaveTime = false
		} else if v.SourceUpdatedAt.After(maxT) {
			maxT = v.SourceUpdatedAt
		}
		if maxID == "" || sourceIDGreater(v.SourceID, maxID) {
			maxID = v.SourceID
		}
	}
	meta.MaxSourceID = maxID
	if !maxT.IsZero() {
		t := maxT
		meta.MaxSourceUpdated = &t
	}
	if allHaveTime && !maxT.IsZero() {
		meta.ReliableWatermark = true
		meta.WatermarkField = "source_updated_at"
	} else {
		meta.ReliableWatermark = false
		if note == "" {
			meta.Note = "no reliable monotonic source timestamp; using bounded recent discovery only"
		}
	}
	return meta
}

func sourceIDGreater(a, b string) bool {
	ai, ea := strconv.ParseUint(a, 10, 64)
	bi, eb := strconv.ParseUint(b, 10, 64)
	if ea == nil && eb == nil {
		return ai > bi
	}
	return a > b
}
