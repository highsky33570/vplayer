package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

type libraryFixture struct {
	r             *gin.Engine
	users         *store.MemoryUsers
	comments      *store.MemoryComments
	favorites     *store.MemoryFavorites
	history       *store.MemoryHistory
	notifications *store.MemoryNotifications
	clock         *time.Time
	alice, bob    *model.User
}

func (f *libraryFixture) tok(t *testing.T, u *model.User) string {
	return tokenFor(t, u.ID, u.Email)
}

func (f *libraryFixture) tick(d time.Duration) { *f.clock = f.clock.Add(d) }

// newLibraryFixture wires memory stores around the real handlers. Videos 1..5 exist and are ready.
func newLibraryFixture(t *testing.T) *libraryFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	catalog := store.NewMemoryVideoCatalog()
	for i := uint64(1); i <= 5; i++ {
		catalog.AddVideo(model.Video{ID: i, Title: "视频" + strconv.FormatUint(i, 10), Status: "ready", IsActive: true, DurationSec: 600})
	}
	f := &libraryFixture{
		users:         store.NewMemoryUsers(),
		comments:      store.NewMemoryComments(),
		favorites:     store.NewMemoryFavorites(catalog),
		history:       store.NewMemoryHistory(catalog),
		notifications: store.NewMemoryNotifications(),
	}
	start := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	f.clock = &start
	var err error
	if f.alice, err = f.users.CreateUserWithHash("alice@example.com", "hash-a", "爱丽丝"); err != nil {
		t.Fatal(err)
	}
	if f.bob, err = f.users.CreateUserWithHash("bob@example.com", "hash-b", "鲍勃"); err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 5; i++ {
		f.comments.AddVideo(i)
	}
	api := &API{
		Cfg:           config.Config{AuthSecret: watchTestSecret},
		Users:         f.users,
		Comments:      f.comments,
		Favorites:     f.favorites,
		History:       f.history,
		Notifications: f.notifications,
		Clock:         func() time.Time { return *f.clock },
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/auth/me", api.Me)
	v1.PATCH("/auth/me", api.RequireUser, api.UpdateMe)
	v1.POST("/videos/:id/comments", api.RequireUser, api.CreateComment)
	v1.GET("/videos/:id/comments", api.ListComments)
	v1.GET("/me/summary", api.RequireUser, api.MeSummary)
	v1.GET("/me/favorites", api.RequireUser, api.ListFavorites)
	v1.GET("/me/favorite-folders", api.RequireUser, api.ListFavoriteFolders)
	v1.GET("/me/favorites/:videoId", api.RequireUser, api.GetFavoriteStatus)
	v1.POST("/me/favorites/:videoId", api.RequireUser, api.AddFavorite)
	v1.DELETE("/me/favorites/:videoId", api.RequireUser, api.RemoveFavorite)
	v1.GET("/me/history", api.RequireUser, api.ListHistory)
	v1.POST("/me/history/:videoId", api.RequireUser, api.UpsertHistory)
	v1.DELETE("/me/history/:videoId", api.RequireUser, api.DeleteHistoryItem)
	v1.DELETE("/me/history", api.RequireUser, api.ClearHistory)
	v1.GET("/me/notifications", api.RequireUser, api.ListNotifications)
	v1.GET("/me/notifications/unread-count", api.RequireUser, api.UnreadNotificationCount)
	v1.POST("/me/notifications/read", api.RequireUser, api.MarkNotificationsRead)
	f.r = r
	return f
}

type listResp struct {
	OK      bool              `json:"ok"`
	Data    []json.RawMessage `json:"data"`
	Total   int               `json:"total"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
	HasMore bool              `json:"has_more"`
}

func decodeList(t *testing.T, body []byte) listResp {
	t.Helper()
	var out listResp
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return out
}

func videoIDs(t *testing.T, l listResp) []uint64 {
	t.Helper()
	ids := make([]uint64, 0, len(l.Data))
	for _, raw := range l.Data {
		var v struct {
			ID uint64 `json:"id"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, v.ID)
	}
	return ids
}

func equalIDs(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLibraryRequiresAuth(t *testing.T) {
	f := newLibraryFixture(t)
	cases := []struct{ method, path, body string }{
		{http.MethodPatch, "/api/v1/auth/me", `{"nickname":"x"}`},
		{http.MethodGet, "/api/v1/me/summary", ""},
		{http.MethodGet, "/api/v1/me/favorites", ""},
		{http.MethodGet, "/api/v1/me/favorite-folders", ""},
		{http.MethodGet, "/api/v1/me/favorites/1", ""},
		{http.MethodPost, "/api/v1/me/favorites/1", ""},
		{http.MethodDelete, "/api/v1/me/favorites/1", ""},
		{http.MethodGet, "/api/v1/me/history", ""},
		{http.MethodPost, "/api/v1/me/history/1", ""},
		{http.MethodDelete, "/api/v1/me/history/1", ""},
		{http.MethodDelete, "/api/v1/me/history", ""},
		{http.MethodGet, "/api/v1/me/notifications", ""},
		{http.MethodGet, "/api/v1/me/notifications/unread-count", ""},
		{http.MethodPost, "/api/v1/me/notifications/read", `{"all":true}`},
	}
	for _, tc := range cases {
		w := doJSON(t, f.r, tc.method, tc.path, tc.body, "")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s guest status=%d", tc.method, tc.path, w.Code)
		}
		w = doJSON(t, f.r, tc.method, tc.path, tc.body, "not-a-token")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s bad token status=%d", tc.method, tc.path, w.Code)
		}
	}
}

func TestFavoritesAddRemoveIdempotent(t *testing.T) {
	f := newLibraryFixture(t)
	tok := f.tok(t, f.alice)

	status := func(id string) bool {
		w := doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites/"+id, "", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("status code=%d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Data struct {
				Favorited bool `json:"favorited"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data.Favorited
	}

	if status("1") {
		t.Fatal("new user should not have video 1 favorited")
	}
	for i := 0; i < 2; i++ {
		w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/1", "", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("add #%d code=%d body=%s", i, w.Code, w.Body.String())
		}
	}
	if !status("1") {
		t.Fatal("video 1 should be favorited")
	}
	list := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites", "", tok).Body.Bytes())
	if list.Total != 1 || !equalIDs(videoIDs(t, list), []uint64{1}) {
		t.Fatalf("double add should keep one row: %+v", list)
	}
	var item struct {
		FavoritedAt string `json:"favorited_at"`
		CoverURL    string `json:"cover_url"`
		Title       string `json:"title"`
	}
	if err := json.Unmarshal(list.Data[0], &item); err != nil {
		t.Fatal(err)
	}
	if item.FavoritedAt == "" || item.CoverURL == "" || item.Title != "视频1" {
		t.Fatalf("item missing fields: %s", list.Data[0])
	}

	folders := doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorite-folders", "", tok)
	var fb struct {
		Data []struct {
			Name      string `json:"name"`
			IsDefault bool   `json:"is_default"`
			Count     int    `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(folders.Body.Bytes(), &fb); err != nil {
		t.Fatal(err)
	}
	if len(fb.Data) != 1 || fb.Data[0].Name != store.DefaultFavoriteFolderName || !fb.Data[0].IsDefault || fb.Data[0].Count != 1 {
		t.Fatalf("default folder = %+v", fb.Data)
	}

	for i := 0; i < 2; i++ {
		w := doJSON(t, f.r, http.MethodDelete, "/api/v1/me/favorites/1", "", tok)
		if w.Code != http.StatusOK {
			t.Fatalf("remove #%d code=%d", i, w.Code)
		}
	}
	if status("1") {
		t.Fatal("video 1 should be gone")
	}
	if w := doJSON(t, f.r, http.MethodDelete, "/api/v1/me/favorites/4", "", tok); w.Code != http.StatusOK {
		t.Fatalf("removing never-favorited code=%d", w.Code)
	}
}

func TestFavoritesInvalidVideo(t *testing.T) {
	f := newLibraryFixture(t)
	tok := f.tok(t, f.alice)
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/999", "", tok); w.Code != http.StatusNotFound {
		t.Fatalf("missing video code=%d", w.Code)
	}
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/abc", "", tok); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id code=%d", w.Code)
	}
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/0", "", tok); w.Code != http.StatusBadRequest {
		t.Fatalf("zero id code=%d", w.Code)
	}
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/history/999", `{"progress_sec":5}`, tok); w.Code != http.StatusNotFound {
		t.Fatalf("history missing video code=%d", w.Code)
	}
	list := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites", "", tok).Body.Bytes())
	if list.Total != 0 {
		t.Fatalf("nothing should be stored: %+v", list)
	}
}

func TestFavoritesOwnershipAndOrdering(t *testing.T) {
	f := newLibraryFixture(t)
	a, b := f.tok(t, f.alice), f.tok(t, f.bob)

	for _, id := range []string{"1", "2", "3"} {
		if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/"+id, "", a); w.Code != http.StatusOK {
			t.Fatalf("alice add %s code=%d", id, w.Code)
		}
		f.tick(time.Minute)
	}
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/5", "", b); w.Code != http.StatusOK {
		t.Fatalf("bob add code=%d", w.Code)
	}

	aList := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites", "", a).Body.Bytes())
	if !equalIDs(videoIDs(t, aList), []uint64{3, 2, 1}) {
		t.Fatalf("alice newest-first = %v", videoIDs(t, aList))
	}
	bList := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites", "", b).Body.Bytes())
	if !equalIDs(videoIDs(t, bList), []uint64{5}) {
		t.Fatalf("bob sees %v", videoIDs(t, bList))
	}

	statusFor := func(tok, id string) bool {
		w := doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites/"+id, "", tok)
		var body struct {
			Data struct {
				Favorited bool `json:"favorited"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return body.Data.Favorited
	}
	if statusFor(b, "1") {
		t.Fatal("bob must not see alice's favorite")
	}
	// Bob removing video 1 must not touch alice's favorite.
	doJSON(t, f.r, http.MethodDelete, "/api/v1/me/favorites/1", "", b)
	if !statusFor(a, "1") {
		t.Fatal("bob's delete removed alice's favorite")
	}

	// Alice's folder id is not usable to read through bob's token.
	folders := decodeFolderIDs(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorite-folders", "", a).Body.Bytes())
	if len(folders) != 1 {
		t.Fatalf("alice folders=%v", folders)
	}
	foreign := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?folder_id="+strconv.FormatUint(folders[0], 10), "", b).Body.Bytes())
	if foreign.Total != 0 {
		t.Fatalf("bob read alice's folder: %+v", foreign)
	}
	own := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?folder_id="+strconv.FormatUint(folders[0], 10), "", a).Body.Bytes())
	if own.Total != 3 {
		t.Fatalf("alice folder filter total=%d", own.Total)
	}
	if w := doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?folder_id=nope", "", a); w.Code != http.StatusBadRequest {
		t.Fatalf("bad folder_id code=%d", w.Code)
	}
}

func decodeFolderIDs(t *testing.T, body []byte) []uint64 {
	t.Helper()
	var resp struct {
		Data []struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	ids := make([]uint64, 0, len(resp.Data))
	for _, d := range resp.Data {
		ids = append(ids, d.ID)
	}
	return ids
}

func TestFavoritesPagination(t *testing.T) {
	f := newLibraryFixture(t)
	tok := f.tok(t, f.alice)
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/"+id, "", tok)
		f.tick(time.Minute)
	}

	def := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites", "", tok).Body.Bytes())
	if def.Limit != 24 || def.Offset != 0 || def.Total != 5 || def.HasMore {
		t.Fatalf("defaults = %+v", def)
	}

	page1 := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?limit=2", "", tok).Body.Bytes())
	if !equalIDs(videoIDs(t, page1), []uint64{5, 4}) || !page1.HasMore || page1.Total != 5 {
		t.Fatalf("page1 = %+v ids=%v", page1, videoIDs(t, page1))
	}
	page3 := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?limit=2&offset=4", "", tok).Body.Bytes())
	if !equalIDs(videoIDs(t, page3), []uint64{1}) || page3.HasMore {
		t.Fatalf("page3 = %+v ids=%v", page3, videoIDs(t, page3))
	}
	beyond := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?offset=50", "", tok).Body.Bytes())
	if len(beyond.Data) != 0 || beyond.Total != 5 || beyond.HasMore {
		t.Fatalf("beyond = %+v", beyond)
	}
	capped := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?limit=9999", "", tok).Body.Bytes())
	if capped.Limit != 100 {
		t.Fatalf("limit cap = %d", capped.Limit)
	}
	junk := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/favorites?limit=abc&offset=-3", "", tok).Body.Bytes())
	if junk.Limit != 24 || junk.Offset != 0 {
		t.Fatalf("junk query = %+v", junk)
	}
}

func TestHistoryUpsertOrderDeleteClear(t *testing.T) {
	f := newLibraryFixture(t)
	a, b := f.tok(t, f.alice), f.tok(t, f.bob)

	post := func(tok, id, body string) {
		t.Helper()
		if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/history/"+id, body, tok); w.Code != http.StatusOK {
			t.Fatalf("upsert %s code=%d body=%s", id, w.Code, w.Body.String())
		}
	}
	post(a, "1", `{"progress_sec":30}`)
	f.tick(time.Minute)
	post(a, "2", `{"progress_sec":60}`)
	f.tick(time.Minute)
	post(a, "3", "")
	f.tick(time.Minute)
	post(b, "4", `{"progress_sec":9}`)

	type hist struct {
		ID            uint64 `json:"id"`
		ProgressSec   int    `json:"progress_sec"`
		LastWatchedAt string `json:"last_watched_at"`
		CoverURL      string `json:"cover_url"`
	}
	load := func(tok string) (listResp, []hist) {
		t.Helper()
		l := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/history", "", tok).Body.Bytes())
		out := make([]hist, 0, len(l.Data))
		for _, raw := range l.Data {
			var h hist
			if err := json.Unmarshal(raw, &h); err != nil {
				t.Fatal(err)
			}
			out = append(out, h)
		}
		return l, out
	}

	l, items := load(a)
	if l.Total != 3 || len(items) != 3 || items[0].ID != 3 || items[1].ID != 2 || items[2].ID != 1 {
		t.Fatalf("order = %+v", items)
	}
	if items[0].ProgressSec != 0 || items[1].ProgressSec != 60 || items[0].LastWatchedAt == "" || items[0].CoverURL == "" {
		t.Fatalf("fields = %+v", items)
	}

	// Re-watching video 1 moves it to the top, and a request without progress keeps the stored value.
	f.tick(time.Minute)
	post(a, "1", "")
	_, items = load(a)
	if items[0].ID != 1 || items[0].ProgressSec != 30 {
		t.Fatalf("after rewatch = %+v", items)
	}
	post(a, "1", `{"progress_sec":45}`)
	_, items = load(a)
	if items[0].ProgressSec != 45 {
		t.Fatalf("progress update = %+v", items)
	}
	post(a, "1", `{"progress_sec":-5}`)
	_, items = load(a)
	if items[0].ProgressSec != 45 {
		t.Fatalf("negative progress must be ignored: %+v", items)
	}

	bl, _ := load(b)
	if bl.Total != 1 {
		t.Fatalf("bob history total=%d", bl.Total)
	}

	if w := doJSON(t, f.r, http.MethodDelete, "/api/v1/me/history/2", "", a); w.Code != http.StatusOK {
		t.Fatalf("delete code=%d", w.Code)
	}
	if w := doJSON(t, f.r, http.MethodDelete, "/api/v1/me/history/2", "", a); w.Code != http.StatusOK {
		t.Fatalf("repeat delete code=%d", w.Code)
	}
	l, _ = load(a)
	if l.Total != 2 {
		t.Fatalf("after delete total=%d", l.Total)
	}
	// Bob deleting alice's entry changes nothing.
	doJSON(t, f.r, http.MethodDelete, "/api/v1/me/history/1", "", b)
	l, _ = load(a)
	if l.Total != 2 {
		t.Fatalf("foreign delete changed alice's history: %d", l.Total)
	}

	if w := doJSON(t, f.r, http.MethodDelete, "/api/v1/me/history", "", a); w.Code != http.StatusOK {
		t.Fatalf("clear code=%d", w.Code)
	}
	l, _ = load(a)
	if l.Total != 0 || len(l.Data) != 0 {
		t.Fatalf("after clear = %+v", l)
	}
	bl, _ = load(b)
	if bl.Total != 1 {
		t.Fatalf("clear touched bob: %d", bl.Total)
	}

	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/history/1", `{bad`, a); w.Code != http.StatusBadRequest {
		t.Fatalf("bad json code=%d", w.Code)
	}
}

func TestHistoryPagination(t *testing.T) {
	f := newLibraryFixture(t)
	tok := f.tok(t, f.alice)
	for _, id := range []string{"1", "2", "3"} {
		doJSON(t, f.r, http.MethodPost, "/api/v1/me/history/"+id, "", tok)
		f.tick(time.Minute)
	}
	l := decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/history?limit=2", "", tok).Body.Bytes())
	if !equalIDs(videoIDs(t, l), []uint64{3, 2}) || !l.HasMore || l.Total != 3 {
		t.Fatalf("page = %+v", l)
	}
	l = decodeList(t, doJSON(t, f.r, http.MethodGet, "/api/v1/me/history?limit=2&offset=2", "", tok).Body.Bytes())
	if !equalIDs(videoIDs(t, l), []uint64{1}) || l.HasMore {
		t.Fatalf("page2 = %+v", l)
	}
}

func TestMeSummary(t *testing.T) {
	f := newLibraryFixture(t)
	a, b := f.tok(t, f.alice), f.tok(t, f.bob)
	doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/1", "", a)
	doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/2", "", a)
	doJSON(t, f.r, http.MethodPost, "/api/v1/me/history/3", "", a)
	doJSON(t, f.r, http.MethodPost, "/api/v1/me/favorites/4", "", b)

	// Bob replies to Alice so Alice has an unread notification.
	w := doJSON(t, f.r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"top"}`, a)
	var created struct {
		Data struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"re","parent_id":`+strconv.FormatUint(created.Data.ID, 10)+`}`, b); w.Code != http.StatusCreated {
		t.Fatalf("reply code=%d body=%s", w.Code, w.Body.String())
	}

	sum := doJSON(t, f.r, http.MethodGet, "/api/v1/me/summary", "", a)
	var body struct {
		Data struct {
			Favorites int `json:"favorites"`
			History   int `json:"history"`
			Unread    int `json:"unread_notifications"`
		} `json:"data"`
	}
	if err := json.Unmarshal(sum.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Favorites != 2 || body.Data.History != 1 || body.Data.Unread != 1 {
		t.Fatalf("alice summary = %+v", body.Data)
	}
	sum = doJSON(t, f.r, http.MethodGet, "/api/v1/me/summary", "", b)
	body.Data.Unread = -1
	if err := json.Unmarshal(sum.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Favorites != 1 || body.Data.History != 0 || body.Data.Unread != 0 {
		t.Fatalf("bob summary = %+v", body.Data)
	}
}

func TestUpdateMeProfile(t *testing.T) {
	f := newLibraryFixture(t)
	tok := f.tok(t, f.alice)

	me := doJSON(t, f.r, http.MethodGet, "/api/v1/auth/me", "", tok)
	var meBody struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &meBody); err != nil {
		t.Fatal(err)
	}
	if hue, ok := meBody.Data["avatar_hue"].(float64); !ok || hue != -1 {
		t.Fatalf("default avatar_hue = %v", meBody.Data["avatar_hue"])
	}

	w := doJSON(t, f.r, http.MethodPatch, "/api/v1/auth/me", `{"nickname":"  新昵称  ","avatar_hue":200,"user_id":999,"is_admin":true}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("patch code=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Data struct {
			ID        uint64 `json:"id"`
			Nickname  string `json:"nickname"`
			AvatarHue int    `json:"avatar_hue"`
			IsAdmin   bool   `json:"is_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.ID != f.alice.ID || out.Data.Nickname != "新昵称" || out.Data.AvatarHue != 200 || out.Data.IsAdmin {
		t.Fatalf("patch result = %+v", out.Data)
	}
	if other, _ := f.users.GetUserByID(f.bob.ID); other.Nickname != "鲍勃" || other.AvatarHue != -1 {
		t.Fatalf("bob changed: %+v", other)
	}

	// Partial update keeps the other field.
	w = doJSON(t, f.r, http.MethodPatch, "/api/v1/auth/me", `{"avatar_hue":-1}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("hue-only code=%d", w.Code)
	}
	u, _ := f.users.GetUserByID(f.alice.ID)
	if u.Nickname != "新昵称" || u.AvatarHue != -1 {
		t.Fatalf("after hue-only = %+v", u)
	}

	bad := []string{
		`{}`,
		`{"nickname":"   "}`,
		`{"nickname":"` + string(make([]rune, 0)) + `"}`,
		`{"avatar_hue":360}`,
		`{"avatar_hue":-2}`,
		`{"nickname":"` + repeatRunes("字", 65) + `"}`,
		`not json`,
	}
	for _, body := range bad {
		if w := doJSON(t, f.r, http.MethodPatch, "/api/v1/auth/me", body, tok); w.Code != http.StatusBadRequest {
			t.Errorf("body %q code=%d", body, w.Code)
		}
	}
	if w := doJSON(t, f.r, http.MethodPatch, "/api/v1/auth/me", `{"nickname":"`+repeatRunes("字", 64)+`","avatar_hue":359}`, tok); w.Code != http.StatusOK {
		t.Errorf("64 runes / hue 359 code=%d", w.Code)
	}
}

func repeatRunes(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
