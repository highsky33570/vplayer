package store

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// SourceCommentInput is one parsed source comment ready to store.
// SourceCreatedRaw is copied as text and is not parsed as a timestamp.
type SourceCommentInput struct {
	SourceCommentID       string
	SourceVideoID         string
	ParentSourceCommentID string
	AuthorName            string
	AvatarPath            string
	Body                  string
	SourceCreatedRaw      string
	DiggUp                int
	DiggDown              int
}

// SourceThread is a parent comment and the replies stored under it.
// It deliberately omits provider ids and the local row id.
type SourceThread struct {
	AuthorName       string
	Body             string
	SourceCreatedRaw string
	DiggUp           int
	DiggDown         int
	Replies          []SourceThread
}

// SourceCommentRepo stores imported comments apart from video_comments.
type SourceCommentRepo interface {
	UpsertSourceComments(videoID uint64, sourceSystem string, rows []SourceCommentInput, now time.Time) (inserted, updated, skipped int, err error)
	ExistingSourceCommentIDs(sourceSystem string, ids []string) (map[string]struct{}, error)
	ListSourceCommentThreads(videoID uint64, limit, offset int) (threads []SourceThread, parentTotal, rowTotal int, err error)
}

const (
	selectSourceCommentSQL = `
		SELECT source_comment_id, video_id, source_video_id, COALESCE(parent_source_comment_id,''),
		       author_name, COALESCE(avatar_path,''), body, COALESCE(source_created_raw,''),
		       digg_up, digg_down
		FROM video_source_comments
		WHERE source_system=? AND source_comment_id=?`
	insertSourceCommentSQL = `
		INSERT INTO video_source_comments (
			video_id, source_system, source_video_id, source_comment_id, parent_source_comment_id,
			author_name, avatar_path, body, source_created_raw, digg_up, digg_down, status,
			fetched_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'visible', ?, ?, ?)`
	updateSourceCommentSQL = `
		UPDATE video_source_comments
		SET video_id=?, source_video_id=?, parent_source_comment_id=?, author_name=?, avatar_path=?,
		    body=?, source_created_raw=?, digg_up=?, digg_down=?, fetched_at=?, updated_at=?
		WHERE source_system=? AND source_comment_id=?`
	touchSourceCommentSQL = `
		UPDATE video_source_comments
		SET fetched_at=?, updated_at=?
		WHERE source_system=? AND source_comment_id=?`
)

func (m *MySQL) UpsertSourceComments(videoID uint64, sourceSystem string, rows []SourceCommentInput, now time.Time) (int, int, int, error) {
	if videoID == 0 {
		return 0, 0, 0, errMissingVideo
	}
	sourceSystem = strings.TrimSpace(sourceSystem)
	if sourceSystem == "" {
		return 0, 0, 0, errMissingSystem
	}
	stamp := formatUTCDateTime(now.UTC())
	inserted, updated, skipped := 0, 0, 0
	seen := map[string]bool{}
	for _, row := range rows {
		row.SourceCommentID = strings.TrimSpace(row.SourceCommentID)
		if row.SourceCommentID == "" || seen[row.SourceCommentID] {
			skipped++
			continue
		}
		seen[row.SourceCommentID] = true
		var cur sourceRow
		err := m.DB.QueryRow(selectSourceCommentSQL, sourceSystem, row.SourceCommentID).Scan(
			&cur.SourceCommentID, &cur.VideoID, &cur.SourceVideoID, &cur.Parent,
			&cur.Author, &cur.Avatar, &cur.Body, &cur.Raw, &cur.DiggUp, &cur.DiggDown,
		)
		if errors.Is(err, sql.ErrNoRows) {
			parent := nullIfEmpty(row.ParentSourceCommentID)
			avatar := nullIfEmpty(row.AvatarPath)
			raw := nullIfEmpty(row.SourceCreatedRaw)
			if _, err := m.DB.Exec(insertSourceCommentSQL,
				videoID, sourceSystem, row.SourceVideoID, row.SourceCommentID, parent,
				row.AuthorName, avatar, row.Body, raw, row.DiggUp, row.DiggDown,
				stamp, stamp, stamp,
			); err != nil {
				return inserted, updated, skipped, err
			}
			inserted++
			continue
		}
		if err != nil {
			return inserted, updated, skipped, err
		}
		if sameSourceContent(cur, videoID, row) {
			if _, err := m.DB.Exec(touchSourceCommentSQL, stamp, stamp, sourceSystem, row.SourceCommentID); err != nil {
				return inserted, updated, skipped, err
			}
			skipped++
			continue
		}
		if _, err := m.DB.Exec(updateSourceCommentSQL,
			videoID, row.SourceVideoID, nullIfEmpty(row.ParentSourceCommentID), row.AuthorName, nullIfEmpty(row.AvatarPath),
			row.Body, nullIfEmpty(row.SourceCreatedRaw), row.DiggUp, row.DiggDown, stamp, stamp,
			sourceSystem, row.SourceCommentID,
		); err != nil {
			return inserted, updated, skipped, err
		}
		updated++
	}
	return inserted, updated, skipped, nil
}

func (m *MySQL) ExistingSourceCommentIDs(sourceSystem string, ids []string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	ids = cleanIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, sourceSystem)
	marks := make([]string, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	q := `SELECT source_comment_id FROM video_source_comments WHERE source_system=? AND source_comment_id IN (` + strings.Join(marks, ",") + `)`
	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (m *MySQL) ListSourceCommentThreads(videoID uint64, limit, offset int) ([]SourceThread, int, int, error) {
	limit, offset = clampSourcePage(limit, offset)
	var parentTotal, rowTotal int
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_source_comments
		WHERE video_id=? AND status='visible' AND (parent_source_comment_id IS NULL OR parent_source_comment_id='')`,
		videoID).Scan(&parentTotal); err != nil {
		return nil, 0, 0, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM video_source_comments WHERE video_id=? AND status='visible'`, videoID).Scan(&rowTotal); err != nil {
		return nil, 0, 0, err
	}
	parentRows, err := m.DB.Query(`
		SELECT source_comment_id, author_name, body, COALESCE(source_created_raw,''), digg_up, digg_down
		FROM video_source_comments
		WHERE video_id=? AND status='visible' AND (parent_source_comment_id IS NULL OR parent_source_comment_id='')
		ORDER BY CAST(source_comment_id AS UNSIGNED) DESC, source_comment_id DESC
		LIMIT ? OFFSET ?`, videoID, limit, offset)
	if err != nil {
		return nil, 0, 0, err
	}
	defer parentRows.Close()
	var parents []sourceRow
	for parentRows.Next() {
		var row sourceRow
		if err := parentRows.Scan(&row.SourceCommentID, &row.Author, &row.Body, &row.Raw, &row.DiggUp, &row.DiggDown); err != nil {
			return nil, 0, 0, err
		}
		parents = append(parents, row)
	}
	if err := parentRows.Err(); err != nil {
		return nil, 0, 0, err
	}
	replies := map[string][]sourceRow{}
	if len(parents) > 0 {
		args := make([]any, 0, len(parents)+1)
		args = append(args, videoID)
		marks := make([]string, len(parents))
		for i, p := range parents {
			marks[i] = "?"
			args = append(args, p.SourceCommentID)
		}
		q := `
			SELECT parent_source_comment_id, author_name, body, COALESCE(source_created_raw,''), digg_up, digg_down, source_comment_id
			FROM video_source_comments
			WHERE video_id=? AND status='visible' AND parent_source_comment_id IN (` + strings.Join(marks, ",") + `)
			ORDER BY CAST(source_comment_id AS UNSIGNED) ASC, source_comment_id ASC`
		replyRows, err := m.DB.Query(q, args...)
		if err != nil {
			return nil, 0, 0, err
		}
		defer replyRows.Close()
		for replyRows.Next() {
			var row sourceRow
			if err := replyRows.Scan(&row.Parent, &row.Author, &row.Body, &row.Raw, &row.DiggUp, &row.DiggDown, &row.SourceCommentID); err != nil {
				return nil, 0, 0, err
			}
			replies[row.Parent] = append(replies[row.Parent], row)
		}
		if err := replyRows.Err(); err != nil {
			return nil, 0, 0, err
		}
	}
	threads := make([]SourceThread, 0, len(parents))
	for _, p := range parents {
		threads = append(threads, threadFrom(p, replies[p.SourceCommentID]))
	}
	return threads, parentTotal, rowTotal, nil
}

var (
	errMissingVideo  = errors.New("missing video id")
	errMissingSystem = errors.New("missing source system")
)

type sourceRow struct {
	SourceCommentID string
	SourceSystem    string
	VideoID         uint64
	SourceVideoID   string
	Parent          string
	Author          string
	Avatar          string
	Body            string
	Raw             string
	DiggUp          int
	DiggDown        int
	Status          string
	FetchedAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func sameSourceContent(cur sourceRow, videoID uint64, row SourceCommentInput) bool {
	return cur.VideoID == videoID &&
		cur.SourceVideoID == row.SourceVideoID &&
		cur.Parent == strings.TrimSpace(row.ParentSourceCommentID) &&
		cur.Author == row.AuthorName &&
		cur.Avatar == row.AvatarPath &&
		cur.Body == row.Body &&
		cur.Raw == row.SourceCreatedRaw &&
		cur.DiggUp == row.DiggUp &&
		cur.DiggDown == row.DiggDown
}

func threadFrom(parent sourceRow, replies []sourceRow) SourceThread {
	out := SourceThread{
		AuthorName:       parent.Author,
		Body:             parent.Body,
		SourceCreatedRaw: parent.Raw,
		DiggUp:           parent.DiggUp,
		DiggDown:         parent.DiggDown,
	}
	for _, reply := range replies {
		out.Replies = append(out.Replies, SourceThread{
			AuthorName:       reply.Author,
			Body:             reply.Body,
			SourceCreatedRaw: reply.Raw,
			DiggUp:           reply.DiggUp,
			DiggDown:         reply.DiggDown,
		})
	}
	return out
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func cleanIDs(ids []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func clampSourcePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func sourceIDLess(a, b string) bool {
	an, aErr := strconv.ParseUint(a, 10, 64)
	bn, bErr := strconv.ParseUint(b, 10, 64)
	if aErr == nil && bErr == nil && an != bn {
		return an < bn
	}
	return a < b
}
