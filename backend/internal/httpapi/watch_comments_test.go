package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

const watchTestSecret = "watch-test-secret"

func watchTestRouter(comments *store.MemoryComments, users store.UserRepo, watch watchCatalog, now time.Time) *gin.Engine {
	return watchTestRouterSource(comments, users, watch, now, nil)
}

func watchTestRouterSource(comments *store.MemoryComments, users store.UserRepo, watch watchCatalog, now time.Time, source store.SourceCommentRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:            config.Config{AuthSecret: watchTestSecret},
		Comments:       comments,
		SourceComments: source,
		Users:          users,
		Watch:          watch,
		Clock:          func() time.Time { return now },
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/videos/:id/related", api.ListRelated)
	v1.GET("/videos/:id/episodes", api.ListEpisodes)
	v1.GET("/videos/:id/comments", api.ListComments)
	v1.POST("/videos/:id/comments", api.RequireUser, api.CreateComment)
	v1.DELETE("/comments/:id", api.RequireUser, api.DeleteComment)
	v1.GET("/videos/:id/play", api.RequireUser, api.ResolvePlay)
	return r
}

func doJSON(t *testing.T, r http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func tokenFor(t *testing.T, id uint64, email string) string {
	t.Helper()
	tok, err := auth.IssueToken(watchTestSecret, id, email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestCommentsListAndCreate(t *testing.T) {
	comments := store.NewMemoryComments()
	comments.AddVideo(7)
	users := store.NewMemoryUsers()
	named, err := users.CreateUserWithHash("named@example.com", "password-hash-should-stay-hidden", "小明")
	if err != nil {
		t.Fatal(err)
	}
	blank, err := users.CreateUserWithHash("blank-nick@example.com", "another-password-hash", "   ")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	r := watchTestRouter(comments, users, nil, now)

	empty := doJSON(t, r, http.MethodGet, "/api/v1/videos/7/comments", "", "")
	if empty.Code != http.StatusOK {
		t.Fatalf("empty status=%d body=%s", empty.Code, empty.Body.String())
	}
	var emptyBody struct {
		OK    bool          `json:"ok"`
		Data  []commentJSON `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatal(err)
	}
	if !emptyBody.OK || emptyBody.Total != 0 || len(emptyBody.Data) != 0 {
		t.Fatalf("empty payload %+v", emptyBody)
	}

	missing := doJSON(t, r, http.MethodGet, "/api/v1/videos/99/comments", "", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing video status=%d", missing.Code)
	}

	guest := doJSON(t, r, http.MethodPost, "/api/v1/videos/7/comments", `{"body":"hi"}`, "")
	if guest.Code != http.StatusUnauthorized {
		t.Fatalf("guest post status=%d", guest.Code)
	}

	space := doJSON(t, r, http.MethodPost, "/api/v1/videos/7/comments", `{"body":"  \n\t "}`, tokenFor(t, named.ID, named.Email))
	if space.Code != http.StatusBadRequest {
		t.Fatalf("whitespace status=%d body=%s", space.Code, space.Body.String())
	}

	tooLong := strings.Repeat("你", 1001)
	long := doJSON(t, r, http.MethodPost, "/api/v1/videos/7/comments", `{"body":"`+tooLong+`"}`, tokenFor(t, named.ID, named.Email))
	if long.Code != http.StatusBadRequest {
		t.Fatalf("1001 status=%d", long.Code)
	}

	exact := strings.Repeat("你", 1000)
	okExact := doJSON(t, r, http.MethodPost, "/api/v1/videos/7/comments", `{"body":"`+exact+`"}`, tokenFor(t, named.ID, named.Email))
	if okExact.Code != http.StatusCreated {
		t.Fatalf("1000 status=%d body=%s", okExact.Code, okExact.Body.String())
	}

	injected := doJSON(t, r, http.MethodPost, "/api/v1/videos/7/comments", `{"body":" hello ","user_id":999}`, tokenFor(t, named.ID, named.Email))
	if injected.Code != http.StatusCreated {
		t.Fatalf("post status=%d body=%s", injected.Code, injected.Body.String())
	}
	raw := injected.Body.String()
	for _, banned := range []string{"named@example.com", "password-hash-should-stay-hidden", "password_hash", "is_admin", "blank-nick@example.com"} {
		if strings.Contains(raw, banned) {
			t.Fatalf("response leaked %s: %s", banned, raw)
		}
	}
	var created struct {
		OK   bool        `json:"ok"`
		Data commentJSON `json:"data"`
	}
	if err := json.Unmarshal(injected.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.Author.DisplayName != "小明" || created.Data.CreatedAt != "2026-10-16T12:00:00Z" || !created.Data.Mine {
		t.Fatalf("created %+v", created.Data)
	}
	if created.Data.Body != "hello" {
		t.Fatalf("body not trimmed: %q", created.Data.Body)
	}
	for _, row := range comments.Snapshot() {
		if row.Body == "hello" && row.UserID != named.ID {
			t.Fatalf("user_id from JSON was stored: %+v", row)
		}
		if row.Body == "hello" && row.Status != "visible" {
			t.Fatalf("status=%s", row.Status)
		}
	}

	anon := doJSON(t, r, http.MethodPost, "/api/v1/videos/7/comments", `{"body":"anon"}`, tokenFor(t, blank.ID, blank.Email))
	if anon.Code != http.StatusCreated {
		t.Fatalf("anon status=%d body=%s", anon.Code, anon.Body.String())
	}
	if err := json.Unmarshal(anon.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.Author.DisplayName != "用户" {
		t.Fatalf("empty nickname display=%q", created.Data.Author.DisplayName)
	}
	if strings.Contains(anon.Body.String(), "blank-nick") || strings.Contains(anon.Body.String(), "@") {
		t.Fatalf("email leaked: %s", anon.Body.String())
	}

	comments.Seed(store.Comment{
		VideoID: 7, UserID: named.ID, Body: "hidden", Status: "deleted",
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now,
	})

	listed := doJSON(t, r, http.MethodGet, "/api/v1/videos/7/comments?limit=20&offset=0", "", "")
	var page struct {
		Data    []commentJSON `json:"data"`
		Total   int           `json:"total"`
		HasMore bool          `json:"has_more"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 {
		t.Fatalf("total=%d", page.Total)
	}
	for _, row := range page.Data {
		if row.Body == "hidden" {
			t.Fatal("deleted comment listed")
		}
		if strings.Contains(row.Author.DisplayName, "@") {
			t.Fatal("email in list")
		}
	}
}

func TestCommentsPagination(t *testing.T) {
	comments := store.NewMemoryComments()
	comments.AddVideo(1)
	users := store.NewMemoryUsers()
	u, err := users.CreateUserWithHash("page@example.com", "hash", "页")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		comments.Seed(store.Comment{
			VideoID: 1, UserID: u.ID, Body: "c", Status: "visible",
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	r := watchTestRouter(comments, users, nil, base.Add(time.Hour))
	first := doJSON(t, r, http.MethodGet, "/api/v1/videos/1/comments", "", "")
	var page struct {
		Data    []commentJSON `json:"data"`
		Total   int           `json:"total"`
		Limit   int           `json:"limit"`
		Offset  int           `json:"offset"`
		HasMore bool          `json:"has_more"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 25 || page.Limit != 20 || page.Offset != 0 || !page.HasMore || len(page.Data) != 20 {
		t.Fatalf("first %+v len=%d", page, len(page.Data))
	}
	if page.Data[0].CreatedAt < page.Data[len(page.Data)-1].CreatedAt {
		t.Fatalf("order first=%s last=%s", page.Data[0].CreatedAt, page.Data[len(page.Data)-1].CreatedAt)
	}
	second := doJSON(t, r, http.MethodGet, "/api/v1/videos/1/comments?limit=20&offset=20", "", "")
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.HasMore || len(page.Data) != 5 || page.Offset != 20 {
		t.Fatalf("second has_more=%v len=%d offset=%d", page.HasMore, len(page.Data), page.Offset)
	}
	capped := doJSON(t, r, http.MethodGet, "/api/v1/videos/1/comments?limit=80", "", "")
	if err := json.Unmarshal(capped.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Limit != 50 || len(page.Data) != 25 {
		t.Fatalf("cap limit=%d len=%d", page.Limit, len(page.Data))
	}
}

func TestCommentDeleteAuthorOnly(t *testing.T) {
	comments := store.NewMemoryComments()
	comments.AddVideo(3)
	users := store.NewMemoryUsers()
	owner, err := users.CreateUserWithHash("owner@example.com", "hash-owner", "作者")
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.CreateUserWithHash("other@example.com", "hash-other", "别人")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	row := comments.Seed(store.Comment{VideoID: 3, UserID: owner.ID, Body: "mine", Status: "visible", CreatedAt: now})
	r := watchTestRouter(comments, users, nil, now.Add(time.Minute))

	denied := doJSON(t, r, http.MethodDelete, "/api/v1/comments/"+itoa(row.ID), "", tokenFor(t, other.ID, other.Email))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("other user status=%d body=%s", denied.Code, denied.Body.String())
	}
	missing := doJSON(t, r, http.MethodDelete, "/api/v1/comments/99999", "", tokenFor(t, owner.ID, owner.Email))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", missing.Code)
	}
	ok := doJSON(t, r, http.MethodDelete, "/api/v1/comments/"+itoa(row.ID), "", tokenFor(t, owner.ID, owner.Email))
	if ok.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", ok.Code, ok.Body.String())
	}
	again := doJSON(t, r, http.MethodDelete, "/api/v1/comments/"+itoa(row.ID), "", tokenFor(t, owner.ID, owner.Email))
	if again.Code != http.StatusNotFound {
		t.Fatalf("second delete status=%d", again.Code)
	}
	listed := doJSON(t, r, http.MethodGet, "/api/v1/videos/3/comments", "", "")
	if strings.Contains(listed.Body.String(), "mine") {
		t.Fatalf("deleted still listed: %s", listed.Body.String())
	}
	for _, saved := range comments.Snapshot() {
		if saved.ID == row.ID && saved.Status != "deleted" {
			t.Fatalf("status=%s", saved.Status)
		}
	}
}

func TestCommentRateLimitBoundaries(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	users := store.NewMemoryUsers()
	u, err := users.CreateUserWithHash("rate@example.com", "hash", "频")
	if err != nil {
		t.Fatal(err)
	}

	minute := store.NewMemoryComments()
	minute.AddVideo(1)
	for i := 0; i < 4; i++ {
		minute.Seed(store.Comment{VideoID: 1, UserID: u.ID, Body: "m", Status: "visible", CreatedAt: now.Add(-30 * time.Second)})
	}
	r := watchTestRouter(minute, users, nil, now)
	fifth := doJSON(t, r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"fifth"}`, tokenFor(t, u.ID, u.Email))
	if fifth.Code != http.StatusCreated {
		t.Fatalf("5th status=%d body=%s", fifth.Code, fifth.Body.String())
	}
	sixth := doJSON(t, r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"sixth"}`, tokenFor(t, u.ID, u.Email))
	if sixth.Code != http.StatusTooManyRequests {
		t.Fatalf("6th status=%d body=%s", sixth.Code, sixth.Body.String())
	}

	deletedMinute := store.NewMemoryComments()
	deletedMinute.AddVideo(1)
	for i := 0; i < 5; i++ {
		deletedMinute.Seed(store.Comment{VideoID: 1, UserID: u.ID, Body: "d", Status: "deleted", CreatedAt: now.Add(-10 * time.Second)})
	}
	r = watchTestRouter(deletedMinute, users, nil, now)
	stillLimited := doJSON(t, r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"nope"}`, tokenFor(t, u.ID, u.Email))
	if stillLimited.Code != http.StatusTooManyRequests {
		t.Fatalf("deleted rows must still count, status=%d", stillLimited.Code)
	}

	hour := store.NewMemoryComments()
	hour.AddVideo(1)
	for i := 0; i < 29; i++ {
		hour.Seed(store.Comment{VideoID: 1, UserID: u.ID, Body: "h", Status: "visible", CreatedAt: now.Add(-2 * time.Minute)})
	}
	r = watchTestRouter(hour, users, nil, now)
	thirtieth := doJSON(t, r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"thirty"}`, tokenFor(t, u.ID, u.Email))
	if thirtieth.Code != http.StatusCreated {
		t.Fatalf("30th status=%d body=%s", thirtieth.Code, thirtieth.Body.String())
	}
	thirtyFirst := doJSON(t, r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"too-many"}`, tokenFor(t, u.ID, u.Email))
	if thirtyFirst.Code != http.StatusTooManyRequests {
		t.Fatalf("31st status=%d body=%s", thirtyFirst.Code, thirtyFirst.Body.String())
	}
}

func TestPublicCommentDisplayName(t *testing.T) {
	if PublicCommentDisplayName("") != "用户" || PublicCommentDisplayName("  \t") != "用户" {
		t.Fatal("empty nickname")
	}
	if PublicCommentDisplayName(" 昵称 ") != "昵称" {
		t.Fatal("nickname trim")
	}
}

func itoa(n uint64) string {
	return strings.TrimPrefix(strings.ReplaceAll(jsonNumber(n), " ", ""), "+")
}

func jsonNumber(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

type fakeWatch struct {
	videos   []model.Video
	episodes map[uint64][]store.PublicEpisode
	limit    int
}

func (f *fakeWatch) GetVideo(id uint64) (*model.Video, error) {
	for _, v := range f.videos {
		if v.ID == id {
			cp := v
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeWatch) GetPublicCatalogVideo(id uint64) (*model.Video, error) {
	v, err := f.GetVideo(id)
	if err != nil || v == nil {
		return v, err
	}
	if !v.IsActive || (v.Status != "ready" && v.Status != "demo") {
		return nil, nil
	}
	return v, nil
}

func (f *fakeWatch) ListRelatedVideos(current model.Video, limit int) ([]model.Video, error) {
	f.limit = limit
	return store.RankRelatedVideos(current, f.videos, limit), nil
}

func (f *fakeWatch) ListPublicEpisodes(videoID uint64) ([]store.PublicEpisode, error) {
	return f.episodes[videoID], nil
}

func TestRelatedAndEpisodesHTTP(t *testing.T) {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	watch := &fakeWatch{
		videos: []model.Video{
			{ID: 1, Title: "current", CategoryID: 2, LeafSourceID: "L", Status: "ready", IsActive: true, PlaybackURL: "secret://play", CreatedAt: now},
			{ID: 2, Title: "leaf", CategoryID: 2, LeafSourceID: "L", Status: "ready", IsActive: true, Year: "2024", ViewCount: 10, CreatedAt: now.Add(-time.Hour)},
			{ID: 3, Title: "cat", CategoryID: 2, LeafSourceID: "Z", Status: "ready", IsActive: true, CreatedAt: now},
			{ID: 4, Title: "new", CategoryID: 9, Status: "demo", IsActive: true, CreatedAt: now.Add(time.Hour)},
			{ID: 5, Title: "off", CategoryID: 2, LeafSourceID: "L", Status: "ready", IsActive: false, CreatedAt: now},
			{ID: 6, Title: "draft", CategoryID: 2, LeafSourceID: "L", Status: "draft", IsActive: true, CreatedAt: now},
		},
		episodes: map[uint64][]store.PublicEpisode{
			1: {
				{SID: 2, NID: 1, Title: "第二集"},
				{SID: 1, NID: 2, Title: "一-2"},
				{SID: 1, NID: 1, Title: "一-1"},
			},
		},
	}
	r := watchTestRouter(nil, nil, watch, now)
	res := doJSON(t, r, http.MethodGet, "/api/v1/videos/1/related?limit=50", "", "")
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if watch.limit != 10 {
		t.Fatalf("limit passed=%d", watch.limit)
	}
	body := res.Body.String()
	if strings.Contains(body, "secret://play") || strings.Contains(body, "playback_url") || strings.Contains(body, "fallback_url") || strings.Contains(body, "hls_object_key") {
		t.Fatalf("related leaked playback: %s", body)
	}
	var payload struct {
		Data []relatedCardJSON `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var titles []string
	seen := map[uint64]bool{}
	for _, card := range payload.Data {
		if card.ID == 1 || seen[card.ID] {
			t.Fatalf("bad card %+v", card)
		}
		seen[card.ID] = true
		titles = append(titles, card.Title)
	}
	if strings.Join(titles, ",") != "leaf,cat,new" {
		t.Fatalf("titles=%v", titles)
	}
	missing := doJSON(t, r, http.MethodGet, "/api/v1/videos/404/related", "", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing related status=%d", missing.Code)
	}

	eps := doJSON(t, r, http.MethodGet, "/api/v1/videos/1/episodes", "", "")
	epBody := eps.Body.String()
	if strings.Contains(epBody, "playback_url") || strings.Contains(epBody, "fallback_url") || strings.Contains(epBody, "hls_object_key") {
		t.Fatalf("episodes leaked: %s", epBody)
	}
	var epPayload struct {
		Data []store.PublicEpisode `json:"data"`
	}
	if err := json.Unmarshal(eps.Body.Bytes(), &epPayload); err != nil {
		t.Fatal(err)
	}
	if len(epPayload.Data) != 3 || epPayload.Data[0].Title != "一-1" || epPayload.Data[1].Title != "一-2" || epPayload.Data[2].Title != "第二集" {
		t.Fatalf("episodes=%v", epPayload.Data)
	}
	keys := epPayload.Data[0]
	raw, _ := json.Marshal(keys)
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	if _, ok := generic["sid"]; !ok {
		t.Fatal("sid")
	}
	if len(generic) != 3 {
		t.Fatalf("episode fields=%v", generic)
	}

	// Video with zero episode rows must serialize data as [] not null (video 10056 regression).
	empty := doJSON(t, r, http.MethodGet, "/api/v1/videos/2/episodes", "", "")
	if empty.Code != http.StatusOK {
		t.Fatalf("empty episodes status=%d body=%s", empty.Code, empty.Body.String())
	}
	if !strings.Contains(empty.Body.String(), `"data":[]`) {
		t.Fatalf("expected empty JSON array, got %s", empty.Body.String())
	}
	if strings.Contains(empty.Body.String(), `"data":null`) {
		t.Fatalf("data:null must not appear for zero episodes: %s", empty.Body.String())
	}
}

func TestPlayRouteStillRequiresUser(t *testing.T) {
	r := watchTestRouter(nil, nil, nil, time.Now())
	res := doJSON(t, r, http.MethodGet, "/api/v1/videos/1/play", "", "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("play status=%d", res.Code)
	}
}

func TestSourceCommentsAPI(t *testing.T) {
	comments := store.NewMemoryComments()
	comments.AddVideo(7)
	users := store.NewMemoryUsers()
	named, err := users.CreateUserWithHash("named@example.com", "password-hash-should-stay-hidden", "小明")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	source := store.NewMemorySourceComments()
	if _, _, _, err := source.UpsertSourceComments(7, "olehdtv", []store.SourceCommentInput{
		{SourceCommentID: "413210", SourceVideoID: "82861", AuthorName: "游客", Body: "源站正文", SourceCreatedRaw: "2026-10-19 01:56:45", DiggUp: 63, DiggDown: 1},
		{SourceCommentID: "412878", SourceVideoID: "83449", AuthorName: "", Body: "父评论", SourceCreatedRaw: "2026-10-16 22:34:16"},
		{SourceCommentID: "413117", SourceVideoID: "83449", ParentSourceCommentID: "412878", AuthorName: "游客", Body: "回复正文", SourceCreatedRaw: "10:28", DiggUp: 2, DiggDown: 0},
	}, now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 21; i++ {
		if _, _, _, err := source.UpsertSourceComments(8, "olehdtv", []store.SourceCommentInput{{
			SourceCommentID: strconv.Itoa(1000 + i), SourceVideoID: "37782", AuthorName: "游客", Body: "p", SourceCreatedRaw: "2026-10-01 00:00:00",
		}}, now); err != nil {
			t.Fatal(err)
		}
	}
	comments.AddVideo(8)
	r := watchTestRouterSource(comments, users, nil, now, source)
	token := tokenFor(t, named.ID, named.Email)
	created := doJSON(t, r, http.MethodPost, "/api/v1/videos/7/comments", `{"body":"native body"}`, token)
	if created.Code != http.StatusCreated {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	var createdBody struct {
		Data commentJSON `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatal(err)
	}
	if createdBody.Data.Origin != "vplayer" || !createdBody.Data.Mine {
		t.Fatalf("native create %+v", createdBody.Data)
	}

	listed := doJSON(t, r, http.MethodGet, "/api/v1/videos/7/comments?limit=20&offset=0&source_limit=20&source_offset=0", "", token)
	if listed.Code != http.StatusOK {
		t.Fatalf("list %d %s", listed.Code, listed.Body.String())
	}
	raw := listed.Body.String()
	for _, banned := range []string{"named@example.com", "password-hash-should-stay-hidden", "password_hash", "source_comment_id", "source_video_id", "parent_source_comment_id", "413210", "82861"} {
		if strings.Contains(raw, banned) {
			t.Fatalf("response exposed %s: %s", banned, raw)
		}
	}
	var page struct {
		Data    []commentJSON         `json:"data"`
		Total   int                   `json:"total"`
		HasMore bool                  `json:"has_more"`
		Source  sourceCommentPageJSON `json:"source"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Data) != 1 || page.Data[0].Origin != "vplayer" || page.Data[0].Body != "native body" {
		t.Fatalf("native page %+v", page)
	}
	if page.Source.Error != "" || page.Source.Total != 3 || len(page.Source.Data) != 2 {
		t.Fatalf("source page %+v", page.Source)
	}
	var parent sourceCommentJSON
	for _, row := range page.Source.Data {
		if row.Body == "父评论" {
			parent = row
		}
		if row.Origin != "source" || row.Mine {
			t.Fatalf("source row %+v", row)
		}
		if row.Body == "源站正文" && (row.SourceCreatedRaw != "2026-10-19 01:56:45" || row.DiggUp != 63 || row.DiggDown != 1 || row.Author.DisplayName != "游客") {
			t.Fatalf("raw source %+v", row)
		}
	}
	if parent.Author.DisplayName != "游客" || len(parent.Replies) != 1 {
		t.Fatalf("parent %+v", parent)
	}
	reply := parent.Replies[0]
	if reply.Origin != "source" || reply.Mine || reply.Body != "回复正文" || reply.SourceCreatedRaw != "10:28" || reply.DiggUp != 2 {
		t.Fatalf("reply %+v", reply)
	}
	var sourceMap map[string]any
	sourceRaw, _ := json.Marshal(page.Source)
	if err := json.Unmarshal(sourceRaw, &sourceMap); err != nil {
		t.Fatal(err)
	}
	walkBannedKeys(t, sourceMap)

	more := doJSON(t, r, http.MethodGet, "/api/v1/videos/8/comments?source_limit=20&source_offset=0", "", "")
	var page2 struct {
		Source sourceCommentPageJSON `json:"source"`
	}
	if err := json.Unmarshal(more.Body.Bytes(), &page2); err != nil {
		t.Fatal(err)
	}
	if page2.Source.Total != 21 || len(page2.Source.Data) != 20 || !page2.Source.HasMore {
		t.Fatalf("source page1 %+v", page2.Source)
	}
	rest := doJSON(t, r, http.MethodGet, "/api/v1/videos/8/comments?source_limit=20&source_offset=20", "", "")
	if err := json.Unmarshal(rest.Body.Bytes(), &page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Source.Data) != 1 || page2.Source.HasMore {
		t.Fatalf("source page2 %+v", page2.Source)
	}

	missing := doJSON(t, r, http.MethodDelete, "/api/v1/comments/999999", "", token)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("delete missing %d", missing.Code)
	}
	if len(source.Snapshot()) != 24 {
		t.Fatalf("source rows changed: %d", len(source.Snapshot()))
	}
	deleted := doJSON(t, r, http.MethodDelete, "/api/v1/comments/"+strconv.FormatUint(createdBody.Data.ID, 10), "", token)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete native %d %s", deleted.Code, deleted.Body.String())
	}
	if len(source.Snapshot()) != 24 {
		t.Fatal("native delete removed source comments")
	}

	source.SetListErr(errors.New("db down"))
	failed := doJSON(t, r, http.MethodGet, "/api/v1/videos/7/comments", "", "")
	if failed.Code != http.StatusOK {
		t.Fatalf("source failure status %d", failed.Code)
	}
	var failedPage struct {
		Data   []commentJSON         `json:"data"`
		Source sourceCommentPageJSON `json:"source"`
	}
	if err := json.Unmarshal(failed.Body.Bytes(), &failedPage); err != nil {
		t.Fatal(err)
	}
	if failedPage.Source.Error != "原站评论加载失败" {
		t.Fatalf("source error %+v", failedPage.Source)
	}
}

func walkBannedKeys(t *testing.T, v any) {
	t.Helper()
	switch node := v.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case "id", "video_id", "source_comment_id", "source_video_id", "parent_source_comment_id", "user_id", "email", "avatar_path", "avatar_url":
				t.Fatalf("source payload exposed %s", key)
			}
			walkBannedKeys(t, child)
		}
	case []any:
		for _, child := range node {
			walkBannedKeys(t, child)
		}
	}
}
