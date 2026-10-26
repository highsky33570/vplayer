package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

// memPublicVideos mirrors ready-list filtering for ListVideos tests (no MySQL).
type memPublicVideos struct {
	videos     []model.Video
	catSlug    map[uint64]string // category_id -> slug
	lastList   listCall
	lastCount  listCall
	listCalls  int
	countCalls int
}

type listCall struct {
	categoryID          uint64
	query               string
	excludeCategorySlug string
	limit               int
	offset              int
}

func (m *memPublicVideos) filter(categoryID uint64, query, excludeCategorySlug string) []model.Video {
	q := strings.ToLower(strings.TrimSpace(query))
	excl := store.NormalizeExcludeCategorySlug(excludeCategorySlug)
	var out []model.Video
	for _, v := range m.videos {
		if v.Status != "ready" && v.Status != "demo" {
			continue
		}
		if !v.IsActive {
			continue
		}
		if categoryID > 0 && v.CategoryID != categoryID {
			continue
		}
		if excl != "" {
			if slug := m.catSlug[v.CategoryID]; slug == excl {
				continue
			}
		}
		if q != "" {
			blob := strings.ToLower(v.Title + " " + v.Description + " " + v.Actors)
			if !strings.Contains(blob, q) {
				continue
			}
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

func (m *memPublicVideos) CountReadyVideosFiltered(categoryID uint64, query, excludeCategorySlug string) (int, error) {
	m.countCalls++
	m.lastCount = listCall{categoryID: categoryID, query: query, excludeCategorySlug: excludeCategorySlug}
	return len(m.filter(categoryID, query, excludeCategorySlug)), nil
}

func (m *memPublicVideos) ListReadyVideosFiltered(categoryID uint64, query, excludeCategorySlug, order string, limit, offset int) ([]model.Video, error) {
	m.listCalls++
	m.lastList = listCall{
		categoryID: categoryID, query: query, excludeCategorySlug: excludeCategorySlug,
		limit: limit, offset: offset,
	}
	filtered := m.filter(categoryID, query, excludeCategorySlug)
	if order == "hot" {
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].ViewCount != filtered[j].ViewCount {
				return filtered[i].ViewCount > filtered[j].ViewCount
			}
			return filtered[i].ID > filtered[j].ID
		})
	}
	if offset > len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out := make([]model.Video, end-offset)
	copy(out, filtered[offset:end])
	return out, nil
}

func testListVideosRouter(repo publicVideoRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := &API{PublicVideos: repo}
	r.GET("/api/v1/videos", api.ListVideos)
	return r
}

func samplePublicCatalog() *memPublicVideos {
	t0 := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	return &memPublicVideos{
		catSlug: map[uint64]string{
			2:  "movie",
			16: "midnight",
		},
		videos: []model.Video{
			{ID: 101, Title: "Midnight Newest", CategoryID: 16, Status: "ready", IsActive: true, CreatedAt: t0},
			{ID: 102, Title: "Movie New", CategoryID: 2, Status: "ready", IsActive: true, CreatedAt: t0.Add(-time.Minute)},
			{ID: 103, Title: "Midnight Mid", CategoryID: 16, Status: "ready", IsActive: true, CreatedAt: t0.Add(-2 * time.Minute)},
			{ID: 104, Title: "Movie Older", CategoryID: 2, Status: "ready", IsActive: true, CreatedAt: t0.Add(-3 * time.Minute)},
			{ID: 105, Title: "Movie Oldest", CategoryID: 2, Status: "ready", IsActive: true, CreatedAt: t0.Add(-4 * time.Minute)},
		},
	}
}

func decodeVideoList(t *testing.T, w *httptest.ResponseRecorder) (ids []uint64, total int) {
	t.Helper()
	var body struct {
		OK    bool          `json:"ok"`
		Data  []model.Video `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK {
		t.Fatalf("body=%s", w.Body.String())
	}
	for _, v := range body.Data {
		ids = append(ids, v.ID)
	}
	return ids, body.Total
}

func TestListVideos_sortHotByViews(t *testing.T) {
	mem := samplePublicCatalog()
	mem.videos[0].ViewCount = 2
	mem.videos[1].ViewCount = 9
	mem.videos[4].ViewCount = 40
	r := testListVideosRouter(mem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos?limit=5&offset=0&sort=hot", nil)
	r.ServeHTTP(w, req)
	ids, _ := decodeVideoList(t, w)
	if len(ids) < 2 || ids[0] != 105 || ids[1] != 102 {
		t.Fatalf("ids=%v want hottest first", ids)
	}
}

func TestListVideos_excludeMidnightUnscoped(t *testing.T) {
	mem := samplePublicCatalog()
	r := testListVideosRouter(mem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos?limit=12&offset=0&exclude_category_slug=midnight", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	ids, total := decodeVideoList(t, w)
	for _, id := range ids {
		if id == 101 || id == 103 {
			t.Fatalf("midnight video leaked: %v", ids)
		}
	}
	if total != 3 {
		t.Fatalf("total=%d want 3 non-midnight", total)
	}
	if mem.lastList.excludeCategorySlug != "midnight" || mem.lastCount.excludeCategorySlug != "midnight" {
		t.Fatalf("list=%+v count=%+v", mem.lastList, mem.lastCount)
	}
}

func TestListVideos_excludeFillsLimitBeforePaging(t *testing.T) {
	mem := samplePublicCatalog()
	r := testListVideosRouter(mem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos?limit=3&offset=0&exclude_category_slug=midnight", nil)
	r.ServeHTTP(w, req)
	ids, _ := decodeVideoList(t, w)
	// Newest overall include midnight; after exclusion, fill 3 from older non-midnight.
	want := []uint64{102, 104, 105}
	if len(ids) != 3 {
		t.Fatalf("ids=%v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids=%v want %v (exclusion before limit)", ids, want)
		}
	}
}

func TestListVideos_withoutExcludeUnchanged(t *testing.T) {
	mem := samplePublicCatalog()
	r := testListVideosRouter(mem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos?limit=12&offset=0", nil)
	r.ServeHTTP(w, req)
	ids, total := decodeVideoList(t, w)
	if total != 5 || len(ids) != 5 {
		t.Fatalf("ids=%v total=%d", ids, total)
	}
	if ids[0] != 101 {
		t.Fatalf("newest midnight should lead without exclude: %v", ids)
	}
	if mem.lastList.excludeCategorySlug != "" {
		t.Fatalf("exclude must stay empty: %+v", mem.lastList)
	}
}

func TestListVideos_categoryMidnightStillReturns(t *testing.T) {
	mem := samplePublicCatalog()
	r := testListVideosRouter(mem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos?category_id=16&limit=12", nil)
	r.ServeHTTP(w, req)
	ids, total := decodeVideoList(t, w)
	if total != 2 || len(ids) != 2 {
		t.Fatalf("ids=%v total=%d", ids, total)
	}
	for _, id := range ids {
		if id != 101 && id != 103 {
			t.Fatalf("unexpected id %d", id)
		}
	}
	if mem.lastList.excludeCategorySlug != "" {
		t.Fatal("category browse must not send exclusion")
	}
}

func TestListVideos_searchWithoutExcludeFindsMidnight(t *testing.T) {
	mem := samplePublicCatalog()
	r := testListVideosRouter(mem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos?q=Midnight&limit=12", nil)
	r.ServeHTTP(w, req)
	ids, total := decodeVideoList(t, w)
	if total != 2 || len(ids) != 2 {
		t.Fatalf("ids=%v total=%d", ids, total)
	}
	if mem.lastList.excludeCategorySlug != "" || mem.lastList.query != "Midnight" {
		t.Fatalf("list=%+v", mem.lastList)
	}
}

func TestListVideos_excludeMidnightStillDropsLeafMembership(t *testing.T) {
	// Phase 3: leaf_source_id must not bypass exclude_category_slug=midnight
	// (exclusion remains on canonical videos.category_id).
	mem := samplePublicCatalog()
	for i := range mem.videos {
		if mem.videos[i].CategoryID == 16 {
			mem.videos[i].LeafSourceID = "506"
		}
	}
	r := testListVideosRouter(mem)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos?limit=12&exclude_category_slug=midnight", nil)
	r.ServeHTTP(w, req)
	ids, total := decodeVideoList(t, w)
	for _, id := range ids {
		if id == 101 || id == 103 {
			t.Fatalf("midnight+leaf leaked: %v", ids)
		}
	}
	if total != 3 {
		t.Fatalf("total=%d want 3", total)
	}
}
