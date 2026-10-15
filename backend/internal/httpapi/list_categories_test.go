package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/model"
)

func testCategoriesRouter(api *API) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/categories", api.ListCategories)
	return r
}

func TestListCategoriesDemoTopLevelParentIDNull(t *testing.T) {
	r := testCategoriesRouter(&API{}) // Store nil => demoCategories
	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var body struct {
		OK   bool              `json:"ok"`
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || len(body.Data) == 0 {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}

	for i, raw := range body.Data {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"id", "name", "slug", "sort", "is_active", "created_at", "parent_id"} {
			if _, ok := m[key]; !ok {
				t.Fatalf("item[%d] missing field %q: %s", i, key, string(raw))
			}
		}
		if m["parent_id"] != nil {
			t.Fatalf("demo top-level item[%d] parent_id want null, got %v", i, m["parent_id"])
		}
	}
}

func TestListCategoriesJSONChildParentID(t *testing.T) {
	// Simulates the additive JSON shape ListCategories will return once children exist.
	parent := uint64(2)
	rows := []model.Category{
		{ID: 2, ParentID: nil, Name: "电影", Slug: "movie", Sort: 10, IsActive: true},
		{ID: 100, ParentID: &parent, Name: "动作片", Slug: "type-101", Sort: 11, IsActive: true},
	}
	raw, err := json.Marshal(gin.H{"ok": true, "data": rows})
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID       uint64  `json:"id"`
			ParentID *uint64 `json:"parent_id"`
			Name     string  `json:"name"`
			Slug     string  `json:"slug"`
			Sort     int     `json:"sort"`
			IsActive bool    `json:"is_active"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("len=%d", len(body.Data))
	}
	if body.Data[0].ParentID != nil {
		t.Fatalf("top-level parent_id=%v want null", body.Data[0].ParentID)
	}
	if body.Data[0].Slug != "movie" || body.Data[0].Name != "电影" || body.Data[0].Sort != 10 {
		t.Fatalf("existing fields changed: %+v", body.Data[0])
	}
	if body.Data[1].ParentID == nil || *body.Data[1].ParentID != 2 {
		t.Fatalf("child parent_id=%v want 2", body.Data[1].ParentID)
	}
	if body.Data[1].Slug != "type-101" || body.Data[1].Name != "动作片" {
		t.Fatalf("child fields: %+v", body.Data[1])
	}
}

func TestCategoryJSONKeepsExistingFields(t *testing.T) {
	c := model.Category{
		ID: 2, Name: "电影", Slug: "movie", Sort: 10, IsActive: true,
		SourceSystem: "olehdtv", SourceID: "1",
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["id"].(float64) != 2 || m["name"] != "电影" || m["slug"] != "movie" || m["sort"].(float64) != 10 {
		t.Fatalf("core fields: %v", m)
	}
	if m["is_active"] != true {
		t.Fatalf("is_active=%v", m["is_active"])
	}
	if m["source_system"] != "olehdtv" || m["source_id"] != "1" {
		t.Fatalf("source fields: %v", m)
	}
	if _, ok := m["parent_id"]; !ok {
		t.Fatal("parent_id must be present (null for top-level)")
	}
	if m["parent_id"] != nil {
		t.Fatalf("parent_id=%v want null", m["parent_id"])
	}
}
