package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/tycdn/vplayer/internal/model"
)

type MySQL struct {
	DB *sql.DB
}

func OpenMySQL(dsn string) (*MySQL, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("mysql ping: %w", err)
	}
	return &MySQL{DB: db}, nil
}

func (m *MySQL) Migrate(sqlText string) error {
	for _, stmt := range splitSQL(sqlText) {
		if _, err := m.DB.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// splitSQL splits a migration script on semicolons that terminate statements.
// Semicolons inside -- line comments, # line comments, /* block comments */,
// and quoted literals stay inside the statement. Comment text is preserved so
// MySQL still receives it; only the split points change.
func splitSQL(sqlText string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		b.Reset()
		if s != "" {
			out = append(out, s)
		}
	}

	const (
		stateNormal = iota
		stateLineComment
		stateBlockComment
		stateSingle
		stateDouble
		stateBacktick
	)
	state := stateNormal
	for i := 0; i < len(sqlText); {
		c := sqlText[i]
		switch state {
		case stateLineComment:
			b.WriteByte(c)
			i++
			if c == '\n' {
				state = stateNormal
			}
		case stateBlockComment:
			if c == '*' && i+1 < len(sqlText) && sqlText[i+1] == '/' {
				b.WriteString("*/")
				i += 2
				state = stateNormal
				continue
			}
			b.WriteByte(c)
			i++
		case stateSingle, stateDouble, stateBacktick:
			quote := byte('\'')
			if state == stateDouble {
				quote = '"'
			} else if state == stateBacktick {
				quote = '`'
			}
			b.WriteByte(c)
			i++
			// MySQL default escaping. Backticks escape by doubling only.
			if c == '\\' && state != stateBacktick && i < len(sqlText) {
				b.WriteByte(sqlText[i])
				i++
				continue
			}
			if c == quote {
				if i < len(sqlText) && sqlText[i] == quote {
					b.WriteByte(quote)
					i++
					continue
				}
				state = stateNormal
			}
		default:
			if c == '-' && i+1 < len(sqlText) && sqlText[i+1] == '-' && (i+2 >= len(sqlText) || isSQLCommentSpace(sqlText[i+2])) {
				b.WriteByte(c)
				i++
				state = stateLineComment
				continue
			}
			if c == '#' {
				b.WriteByte(c)
				i++
				state = stateLineComment
				continue
			}
			if c == '/' && i+1 < len(sqlText) && sqlText[i+1] == '*' {
				b.WriteString("/*")
				i += 2
				state = stateBlockComment
				continue
			}
			if c == ';' {
				flush()
				i++
				continue
			}
			b.WriteByte(c)
			i++
			switch c {
			case '\'':
				state = stateSingle
			case '"':
				state = stateDouble
			case '`':
				state = stateBacktick
			}
		}
	}
	flush()
	return out
}

func isSQLCommentSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

func (m *MySQL) ListCategories() ([]model.Category, error) {
	rows, err := m.DB.Query(`
		SELECT id, parent_id, name, slug, sort,
		       COALESCE(source_system,''), COALESCE(source_id,''),
		       COALESCE(is_active,1), created_at
		FROM categories
		WHERE COALESCE(is_active,1)=1
		ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Category
	for rows.Next() {
		var c model.Category
		var parentID sql.NullInt64
		var active int
		if err := rows.Scan(&c.ID, &parentID, &c.Name, &c.Slug, &c.Sort, &c.SourceSystem, &c.SourceID, &active, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.ParentID = nullUint64Ptr(parentID)
		c.IsActive = active == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// nullUint64Ptr maps a nullable BIGINT to *uint64 (nil when SQL NULL).
func nullUint64Ptr(n sql.NullInt64) *uint64 {
	if !n.Valid {
		return nil
	}
	v := uint64(n.Int64)
	return &v
}

func (m *MySQL) ListReadyVideos(limit, offset int) ([]model.Video, error) {
	return m.ListReadyVideosByCategory(0, limit, offset)
}

func (m *MySQL) ListReadyVideosByCategory(categoryID uint64, limit, offset int) ([]model.Video, error) {
	return m.ListReadyVideosFiltered(categoryID, "", "", "", limit, offset)
}

func (m *MySQL) ListReadyVideosFiltered(categoryID uint64, query, excludeCategorySlug, sort string, limit, offset int) ([]model.Video, error) {
	if limit <= 0 || limit > 200 {
		limit = 48
	}
	browse, err := m.resolveCategoryBrowse(categoryID)
	if err != nil {
		return nil, err
	}
	q := `
		SELECT id, COALESCE(source_system,''), COALESCE(source_id,''), category_id, COALESCE(source_category_id,''),
		       COALESCE(leaf_source_id,''),
		       title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		       COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		       duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), COALESCE(play_sid,1), COALESCE(play_nid,1),
		       COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		       COALESCE(is_active,1), status, created_at
		FROM videos
		WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	args := []any{}
	q, args = AppendReadyVideosWhere(q, args, browse, query, excludeCategorySlug)
	// Exclusion (if any) is already in WHERE above — applied before ORDER BY/LIMIT/OFFSET.
	q += ReadyVideoOrderSQL(sort) + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Video
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (m *MySQL) GetVideo(id uint64) (*model.Video, error) {
	row := m.DB.QueryRow(`
		SELECT id, COALESCE(source_system,''), COALESCE(source_id,''), category_id, COALESCE(source_category_id,''),
		       COALESCE(leaf_source_id,''),
		       title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		       COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		       duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), COALESCE(play_sid,1), COALESCE(play_nid,1),
		       COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		       COALESCE(is_active,1), status, created_at
		FROM videos WHERE id = ?`, id)
	return scanVideo(row)
}

func (m *MySQL) resolveCategoryBrowse(categoryID uint64) (CategoryBrowseFilter, error) {
	if categoryID == 0 {
		return CategoryBrowseFilter{}, nil
	}
	cat, err := m.GetCategoryByID(categoryID)
	if err != nil {
		return CategoryBrowseFilter{}, err
	}
	return CategoryBrowseFromRow(categoryID, cat), nil
}
