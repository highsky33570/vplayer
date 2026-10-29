package store

import (
	"fmt"
	"strings"

	"github.com/tycdn/vplayer/internal/olehdtv"
)

// oleHdTvVIPSourceCategorySQL returns "?,?,…" placeholders and args for VIP source ids.
func oleHdTvVIPSourceCategorySQL() (placeholders string, args []any) {
	ids := olehdtv.VIPSourceCategoryIDs()
	ph := make([]string, 0, len(ids))
	args = make([]any, 0, len(ids))
	for _, id := range ids {
		ph = append(ph, "?")
		args = append(args, id)
	}
	return strings.Join(ph, ","), args
}

// AdminExcludeOleHdTvVIPSQL hides OLEHDTV VIP蓝光影院 titles from admin video lists.
// Alias is the videos table alias ("" or "v").
func AdminExcludeOleHdTvVIPSQL(alias string) (string, []any) {
	col := "source_system"
	srcCat := "source_category_id"
	if alias != "" {
		col = alias + ".source_system"
		srcCat = alias + ".source_category_id"
	}
	ph, args := oleHdTvVIPSourceCategorySQL()
	sql := fmt.Sprintf(
		` AND NOT (COALESCE(%s,'') = 'olehdtv' AND COALESCE(%s,'') IN (%s))`,
		col, srcCat, ph,
	)
	return sql, args
}

// PurgeOleHdTvVIPVideos hard-deletes OLEHDTV VIP蓝光影院 titles and related rows.
// Returns the number of videos removed. Safe to run repeatedly.
func (m *MySQL) PurgeOleHdTvVIPVideos() (int64, error) {
	ph, vipArgs := oleHdTvVIPSourceCategorySQL()
	where := `COALESCE(source_system,'') = 'olehdtv' AND COALESCE(source_category_id,'') IN (` + ph + `)`

	type step struct {
		sql  string
		args []any
	}
	steps := []step{
		{`DELETE FROM video_episodes WHERE video_id IN (SELECT id FROM videos WHERE ` + where + `)`, vipArgs},
		{`DELETE FROM video_comments WHERE video_id IN (SELECT id FROM videos WHERE ` + where + `)`, vipArgs},
		{`DELETE FROM video_source_comments WHERE video_id IN (SELECT id FROM videos WHERE ` + where + `)`, vipArgs},
		{`DELETE FROM user_favorites WHERE video_id IN (SELECT id FROM videos WHERE ` + where + `)`, vipArgs},
		{`DELETE FROM user_history WHERE video_id IN (SELECT id FROM videos WHERE ` + where + `)`, vipArgs},
		{`DELETE FROM user_notifications WHERE video_id IN (SELECT id FROM videos WHERE ` + where + `)`, vipArgs},
	}
	for _, s := range steps {
		if _, err := m.DB.Exec(s.sql, s.args...); err != nil {
			// Some optional tables may be missing on older DBs — ignore unknown-table.
			if !strings.Contains(strings.ToLower(err.Error()), "doesn't exist") &&
				!strings.Contains(strings.ToLower(err.Error()), "unknown table") {
				return 0, err
			}
		}
	}
	res, err := m.DB.Exec(`DELETE FROM videos WHERE `+where, vipArgs...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
