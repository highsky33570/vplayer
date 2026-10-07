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

func splitSQL(sqlText string) []string {
	parts := strings.Split(sqlText, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (m *MySQL) ListCategories() ([]model.Category, error) {
	rows, err := m.DB.Query(`
		SELECT id, name, slug, sort,
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
		var active int
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Sort, &c.SourceSystem, &c.SourceID, &active, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.IsActive = active == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func (m *MySQL) ListReadyVideos(limit, offset int) ([]model.Video, error) {
	return m.ListReadyVideosByCategory(0, limit, offset)
}

func (m *MySQL) ListReadyVideosByCategory(categoryID uint64, limit, offset int) ([]model.Video, error) {
	return m.ListReadyVideosFiltered(categoryID, "", limit, offset)
}

func (m *MySQL) ListReadyVideosFiltered(categoryID uint64, query string, limit, offset int) ([]model.Video, error) {
	if limit <= 0 || limit > 200 {
		limit = 48
	}
	q := `
		SELECT id, COALESCE(source_system,''), COALESCE(source_id,''), category_id, COALESCE(source_category_id,''),
		       title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		       COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		       duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), COALESCE(play_sid,1), COALESCE(play_nid,1),
		       COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		       COALESCE(is_active,1), status, created_at
		FROM videos
		WHERE status IN ('ready','demo') AND COALESCE(is_active,1)=1`
	args := []any{}
	if categoryID > 0 {
		q += ` AND category_id=?`
		args = append(args, categoryID)
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND (title LIKE ? OR COALESCE(description,'') LIKE ? OR COALESCE(actors,'') LIKE ?)`
		args = append(args, like, like, like)
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
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
		       title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		       COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		       duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), COALESCE(play_sid,1), COALESCE(play_nid,1),
		       COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		       COALESCE(is_active,1), status, created_at
		FROM videos WHERE id = ?`, id)
	return scanVideo(row)
}
