package olehdtv

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// MacCMSDBAdapter reads authorized MacCMS tables (mac_type / mac_vod) via DSN.
// Requires OLEHDTV_MACCMS_DSN. No public web scraping.
type MacCMSDBAdapter struct {
	db *sql.DB
}

func NewMacCMSDBAdapter(dsn string) (*MacCMSDBAdapter, error) {
	if dsn == "" {
		return nil, fmt.Errorf("olehdtv maccms_db: OLEHDTV_MACCMS_DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("olehdtv maccms_db ping: %w", err)
	}
	return &MacCMSDBAdapter{db: db}, nil
}

func (a *MacCMSDBAdapter) Name() string { return "maccms_db" }

func (a *MacCMSDBAdapter) Close() error {
	if a.db == nil {
		return nil
	}
	return a.db.Close()
}

func (a *MacCMSDBAdapter) ListCategories(ctx context.Context) ([]SourceCategory, error) {
	// Standard MacCMS type table. Adjust column names via env later if needed.
	rows, err := a.db.QueryContext(ctx, `
		SELECT type_id, type_name, type_en, type_sort
		FROM mac_type
		WHERE type_status = 1
		ORDER BY type_sort ASC, type_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("mac_type: %w", err)
	}
	defer rows.Close()
	var out []SourceCategory
	for rows.Next() {
		var id int64
		var name, slug string
		var sort int
		if err := rows.Scan(&id, &name, &slug, &sort); err != nil {
			return nil, err
		}
		if slug == "" {
			slug = fmt.Sprintf("type-%d", id)
		}
		out = append(out, SourceCategory{
			SourceID: fmt.Sprintf("%d", id),
			Name:     name,
			Slug:     slug,
			Sort:     sort,
		})
	}
	return out, rows.Err()
}

func (a *MacCMSDBAdapter) ListVideos(ctx context.Context) ([]SourceVideo, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT vod_id, type_id, vod_name, vod_content, vod_year, vod_area,
		       vod_director, vod_actor, vod_score, vod_pic, vod_hits,
		       vod_time, vod_play_from, vod_play_url, vod_status
		FROM mac_vod
		ORDER BY vod_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("mac_vod: %w", err)
	}
	defer rows.Close()
	var out []SourceVideo
	for rows.Next() {
		var (
			id, typeID, hits, status                                   int64
			name, content, year, area, director, actor, pic, playFrom  string
			playURL                                                    string
			score                                                      float64
			vodTime                                                    sql.NullTime
		)
		if err := rows.Scan(&id, &typeID, &name, &content, &year, &area, &director, &actor, &score, &pic, &hits, &vodTime, &playFrom, &playURL, &status); err != nil {
			return nil, err
		}
		updated := time.Time{}
		if vodTime.Valid {
			updated = vodTime.Time
		}
		eps := parseMacPlayURL(playFrom, playURL)
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
			Episodes:         eps,
		})
	}
	return out, rows.Err()
}

// ParseMacPlayURL parses MacCMS vod_play_from / vod_play_url into episodes.
// Format: sources separated by $$$, episodes by #, name$url by $.
func ParseMacPlayURL(playFrom, playURL string) []SourceEpisode {
	return parseMacPlayURL(playFrom, playURL)
}

func parseMacPlayURL(playFrom, playURL string) []SourceEpisode {
	if playURL == "" {
		return nil
	}
	sources := splitMac(playFrom, "$$$")
	blocks := splitMac(playURL, "$$$")
	var out []SourceEpisode
	for si, block := range blocks {
		src := "default"
		if si < len(sources) && sources[si] != "" {
			src = sources[si]
		}
		eps := splitMac(block, "#")
		for ni, ep := range eps {
			if ep == "" {
				continue
			}
			title, url := splitFirst(ep, "$")
			if url == "" {
				url = title
				title = fmt.Sprintf("第%d集", ni+1)
			}
			out = append(out, SourceEpisode{
				SID:            si + 1,
				NID:            ni + 1,
				Title:          title,
				PlaybackSource: src,
				PlaybackURL:    url,
			})
		}
	}
	return out
}

func splitMac(s, sep string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for {
		i := indexOf(s[start:], sep)
		if i < 0 {
			parts = append(parts, s[start:])
			break
		}
		parts = append(parts, s[start:start+i])
		start += i + len(sep)
	}
	return parts
}

func indexOf(s, sep string) int {
	n := len(sep)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == sep {
			return i
		}
	}
	return -1
}

func splitFirst(s, sep string) (string, string) {
	i := indexOf(s, sep)
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+len(sep):]
}
