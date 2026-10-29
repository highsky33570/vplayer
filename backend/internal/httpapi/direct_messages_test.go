package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/store"
)

func TestSendDirectMessageCreatesUnreadAndLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := store.NewMemoryUsers()
	notifs := store.NewMemoryNotifications()
	alice, err := users.CreateUser("alice-dm@example.com", "password1", "爱丽丝")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := users.CreateUser("bob-dm@example.com", "password1", "test2")
	if err != nil {
		t.Fatal(err)
	}
	api := &API{
		Users:         users,
		Notifications: notifs,
		Cfg:           config.Config{AuthSecret: "test-auth-secret"},
	}
	r := gin.New()
	r.GET("/users/search", api.RequireUser, api.SearchPublicUsers)
	r.GET("/users/:id/public", api.RequireUser, api.LookupPublicUser)
	r.POST("/me/messages", api.RequireUser, api.SendDirectMessage)
	r.GET("/me/notifications/unread-count", api.RequireUser, api.UnreadNotificationCount)

	tok, err := auth.IssueToken(api.authSecret(), alice.ID, alice.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	lookup := httptest.NewRequest(http.MethodGet, "/users/search?q=test2", nil)
	lookup.Header.Set("Authorization", "Bearer "+tok)
	lw := httptest.NewRecorder()
	r.ServeHTTP(lw, lookup)
	if lw.Code != http.StatusOK {
		t.Fatalf("lookup status %d: %s", lw.Code, lw.Body.String())
	}
	var lookEnv struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID       uint64 `json:"id"`
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if err := json.Unmarshal(lw.Body.Bytes(), &lookEnv); err != nil {
		t.Fatal(err)
	}
	if len(lookEnv.Data) != 1 || lookEnv.Data[0].ID != bob.ID {
		t.Fatalf("search test2: %+v", lookEnv.Data)
	}

	byID := httptest.NewRequest(http.MethodGet, "/users/"+strconv.FormatUint(bob.ID, 10)+"/public", nil)
	byID.Header.Set("Authorization", "Bearer "+tok)
	idw := httptest.NewRecorder()
	r.ServeHTTP(idw, byID)
	if idw.Code != http.StatusOK {
		t.Fatalf("id lookup status %d: %s", idw.Code, idw.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"to_user_id": bob.ID,
		"title":      "你好",
		"body":       "欢迎一起看片",
	})
	req := httptest.NewRequest(http.MethodPost, "/me/messages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("send status %d: %s", w.Code, w.Body.String())
	}

	unreadReq := httptest.NewRequest(http.MethodGet, "/me/notifications/unread-count", nil)
	bobTok, err := auth.IssueToken(api.authSecret(), bob.ID, bob.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	unreadReq.Header.Set("Authorization", "Bearer "+bobTok)
	uw := httptest.NewRecorder()
	r.ServeHTTP(uw, unreadReq)
	if uw.Code != http.StatusOK {
		t.Fatalf("unread status %d: %s", uw.Code, uw.Body.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(uw.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.UnreadCount < 1 {
		t.Fatalf("expected bob unread >= 1, got %d", envelope.Data.UnreadCount)
	}
}

func TestNotifyWelcomeSignup(t *testing.T) {
	notifs := store.NewMemoryNotifications()
	api := &API{Notifications: notifs}
	api.notifyWelcomeSignup(42)
	rows, total, err := notifs.ListNotifications(42, store.NotificationKindSystem, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("want 1 welcome notice, got total=%d len=%d", total, len(rows))
	}
	if rows[0].Title != welcomeTitle {
		t.Fatalf("title=%q", rows[0].Title)
	}
	if rows[0].IsRead {
		t.Fatal("welcome should be unread")
	}
}
