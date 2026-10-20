package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

// DefaultFavoriteFolderName is the folder created on a user's first favorite.
const DefaultFavoriteFolderName = "默认收藏"

// FavoriteFolder is a user's favorites folder. Count is the number of favorites inside it.
type FavoriteFolder struct {
	ID        uint64
	UserID    uint64
	Name      string
	IsDefault bool
	CreatedAt time.Time
	Count     int
}

// Favorite is a user_favorites row.
type Favorite struct {
	ID        uint64
	UserID    uint64
	VideoID   uint64
	FolderID  *uint64
	CreatedAt time.Time
}

// FavoriteVideo is a favorite joined with its video.
type FavoriteVideo struct {
	Video       model.Video
	FolderID    *uint64
	FavoritedAt time.Time
}

// FavoriteRepo is the favorites surface used by the /me/favorites APIs.
type FavoriteRepo interface {
	// FavoriteVideoExists reports whether the video row exists (any status).
	FavoriteVideoExists(videoID uint64) (bool, error)
	EnsureDefaultFolder(userID uint64, now time.Time) (uint64, error)
	ListFolders(userID uint64) ([]FavoriteFolder, error)
	// AddFavorite is idempotent: an existing favorite is returned unchanged.
	AddFavorite(userID, videoID, folderID uint64, now time.Time) (Favorite, error)
	// RemoveFavorite is idempotent: removing a missing favorite is not an error.
	RemoveFavorite(userID, videoID uint64) error
	IsFavorited(userID, videoID uint64) (bool, error)
	// ListFavorites lists newest first. A nil folderID lists all folders.
	ListFavorites(userID uint64, folderID *uint64, limit, offset int) ([]FavoriteVideo, int, error)
	CountFavorites(userID uint64) (int, error)
}

const (
	favoriteDefaultLimit = 24
	favoriteMaxLimit     = 100
)

func (m *MySQL) FavoriteVideoExists(videoID uint64) (bool, error) {
	return m.VideoExists(videoID)
}

func (m *MySQL) EnsureDefaultFolder(userID uint64, now time.Time) (uint64, error) {
	const selectSQL = `SELECT id FROM user_favorite_folders WHERE user_id=? AND is_default=1 ORDER BY id ASC LIMIT 1`
	var id uint64
	err := m.DB.QueryRow(selectSQL, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := m.DB.Exec(
		`INSERT INTO user_favorite_folders (user_id, name, is_default, created_at) VALUES (?, ?, 1, ?)`,
		userID, DefaultFavoriteFolderName, formatUTCDateTime(now.UTC()),
	)
	if err != nil {
		return 0, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	// There is no unique key on (user_id, is_default). If two requests raced, keep the lowest id.
	var first uint64
	if err := m.DB.QueryRow(selectSQL, userID).Scan(&first); err != nil {
		return 0, err
	}
	if first != uint64(newID) {
		_, _ = m.DB.Exec(`DELETE FROM user_favorite_folders WHERE id=? AND user_id=?`, newID, userID)
	}
	return first, nil
}

func (m *MySQL) ListFolders(userID uint64) ([]FavoriteFolder, error) {
	rows, err := m.DB.Query(`
		SELECT f.id, f.user_id, f.name, f.is_default,
		       DATE_FORMAT(f.created_at, '%Y-%m-%d %H:%i:%s'),
		       (SELECT COUNT(*) FROM user_favorites uf WHERE uf.user_id=f.user_id AND uf.folder_id=f.id)
		FROM user_favorite_folders f
		WHERE f.user_id=?
		ORDER BY f.is_default DESC, f.id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FavoriteFolder{}
	for rows.Next() {
		var f FavoriteFolder
		var def int
		var createdRaw string
		if err := rows.Scan(&f.ID, &f.UserID, &f.Name, &def, &createdRaw, &f.Count); err != nil {
			return nil, err
		}
		f.IsDefault = def == 1
		if f.CreatedAt, err = parseUTCDateTime(createdRaw); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (m *MySQL) AddFavorite(userID, videoID, folderID uint64, now time.Time) (Favorite, error) {
	var folder any
	if folderID != 0 {
		folder = folderID
	}
	// No-op update keeps the first favorite (and its folder/time) on repeats.
	if _, err := m.DB.Exec(`
		INSERT INTO user_favorites (user_id, video_id, folder_id, created_at) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE id=id`,
		userID, videoID, folder, formatUTCDateTime(now.UTC()),
	); err != nil {
		return Favorite{}, err
	}
	var f Favorite
	var folderNull sql.NullInt64
	var createdRaw string
	err := m.DB.QueryRow(`
		SELECT id, user_id, video_id, folder_id, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s')
		FROM user_favorites WHERE user_id=? AND video_id=? LIMIT 1`, userID, videoID,
	).Scan(&f.ID, &f.UserID, &f.VideoID, &folderNull, &createdRaw)
	if err != nil {
		return Favorite{}, err
	}
	f.FolderID = nullUint64Ptr(folderNull)
	if f.CreatedAt, err = parseUTCDateTime(createdRaw); err != nil {
		return Favorite{}, err
	}
	return f, nil
}

func (m *MySQL) RemoveFavorite(userID, videoID uint64) error {
	_, err := m.DB.Exec(`DELETE FROM user_favorites WHERE user_id=? AND video_id=?`, userID, videoID)
	return err
}

func (m *MySQL) IsFavorited(userID, videoID uint64) (bool, error) {
	var one int
	err := m.DB.QueryRow(`SELECT 1 FROM user_favorites WHERE user_id=? AND video_id=? LIMIT 1`, userID, videoID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *MySQL) CountFavorites(userID uint64) (int, error) {
	var n int
	err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM user_favorites f
		JOIN videos v ON v.id = f.video_id
		WHERE f.user_id=? AND `+libraryVideoVisible, userID).Scan(&n)
	return n, err
}

func (m *MySQL) ListFavorites(userID uint64, folderID *uint64, limit, offset int) ([]FavoriteVideo, int, error) {
	limit, offset = clampPage(limit, offset, favoriteDefaultLimit, favoriteMaxLimit)
	where := `f.user_id=? AND ` + libraryVideoVisible
	args := []any{userID}
	if folderID != nil {
		where += ` AND f.folder_id=?`
		args = append(args, *folderID)
	}
	var total int
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM user_favorites f JOIN videos v ON v.id = f.video_id WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT ` + libraryVideoColumns + `,
		       f.folder_id, DATE_FORMAT(f.created_at, '%Y-%m-%d %H:%i:%s')
		FROM user_favorites f
		JOIN videos v ON v.id = f.video_id
		WHERE ` + where + `
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT ? OFFSET ?`
	rows, err := m.DB.Query(q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []FavoriteVideo{}
	for rows.Next() {
		var folder sql.NullInt64
		var createdRaw string
		v, err := scanVideoWithExtra(rows, &folder, &createdRaw)
		if err != nil {
			return nil, 0, err
		}
		at, err := parseUTCDateTime(createdRaw)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, FavoriteVideo{Video: *v, FolderID: nullUint64Ptr(folder), FavoritedAt: at})
	}
	return out, total, rows.Err()
}
