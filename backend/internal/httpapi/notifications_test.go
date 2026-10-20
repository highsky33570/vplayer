package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

type notifItem struct {
	ID        uint64  `json:"id"`
	Kind      string  `json:"kind"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	VideoID   *uint64 `json:"video_id"`
	CommentID *uint64 `json:"comment_id"`
	ActorName string  `json:"actor_name"`
	IsRead    bool    `json:"is_read"`
	CreatedAt string  `json:"created_at"`
}

func (f *libraryFixture) postComment(t *testing.T, tok string, videoID uint64, body string, parentID uint64) (int, uint64) {
	t.Helper()
	payload := map[string]any{"body": body}
	if parentID != 0 {
		payload["parent_id"] = parentID
	}
	raw, _ := json.Marshal(payload)
	w := doJSON(t, f.r, http.MethodPost, "/api/v1/videos/"+strconv.FormatUint(videoID, 10)+"/comments", string(raw), tok)
	var resp struct {
		Data struct {
			ID       uint64  `json:"id"`
			ParentID *uint64 `json:"parent_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code == http.StatusCreated && parentID != 0 && (resp.Data.ParentID == nil || *resp.Data.ParentID != parentID) {
		t.Fatalf("parent_id not echoed: %s", w.Body.String())
	}
	return w.Code, resp.Data.ID
}

func (f *libraryFixture) notifs(t *testing.T, tok, query string) (listResp, []notifItem) {
	t.Helper()
	w := doJSON(t, f.r, http.MethodGet, "/api/v1/me/notifications"+query, "", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("list code=%d body=%s", w.Code, w.Body.String())
	}
	l := decodeList(t, w.Body.Bytes())
	items := make([]notifItem, 0, len(l.Data))
	for _, raw := range l.Data {
		var n notifItem
		if err := json.Unmarshal(raw, &n); err != nil {
			t.Fatal(err)
		}
		items = append(items, n)
	}
	return l, items
}

func (f *libraryFixture) unread(t *testing.T, tok string) int {
	t.Helper()
	w := doJSON(t, f.r, http.MethodGet, "/api/v1/me/notifications/unread-count", "", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("unread code=%d", w.Code)
	}
	var body struct {
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data.UnreadCount
}

func TestCommentReplyCreatesNotification(t *testing.T) {
	f := newLibraryFixture(t)
	a, b := f.tok(t, f.alice), f.tok(t, f.bob)

	code, parentID := f.postComment(t, a, 2, "我是顶层评论", 0)
	if code != http.StatusCreated {
		t.Fatalf("parent code=%d", code)
	}
	if n := f.unread(t, a); n != 0 {
		t.Fatalf("unread before reply = %d", n)
	}

	f.tick(time.Minute)
	long := strings.Repeat("回", 300)
	code, replyID := f.postComment(t, b, 2, long, parentID)
	if code != http.StatusCreated {
		t.Fatalf("reply code=%d", code)
	}

	l, items := f.notifs(t, a, "")
	if l.Total != 1 || len(items) != 1 {
		t.Fatalf("alice notifications = %+v", items)
	}
	n := items[0]
	if n.Kind != store.NotificationKindCommentReply || n.Title != "有人回复了你的评论" || n.IsRead {
		t.Fatalf("notification = %+v", n)
	}
	if n.VideoID == nil || *n.VideoID != 2 || n.CommentID == nil || *n.CommentID != replyID {
		t.Fatalf("links = %+v", n)
	}
	if n.ActorName != "鲍勃" {
		t.Fatalf("actor = %q", n.ActorName)
	}
	if got := len([]rune(n.Body)); got > 101 || !strings.HasPrefix(n.Body, "回回回") || !strings.HasSuffix(n.Body, "…") {
		t.Fatalf("body not truncated: %d runes", got)
	}
	// The actor user id and email are never exposed.
	raw := doJSON(t, f.r, http.MethodGet, "/api/v1/me/notifications", "", a).Body.String()
	for _, banned := range []string{"actor_user_id", "bob@example.com", "alice@example.com"} {
		if strings.Contains(raw, banned) {
			t.Fatalf("response leaked %s: %s", banned, raw)
		}
	}
	if n := f.unread(t, a); n != 1 {
		t.Fatalf("alice unread = %d", n)
	}
	if n := f.unread(t, b); n != 0 {
		t.Fatalf("bob unread = %d", n)
	}
	if _, items := f.notifs(t, b, ""); len(items) != 0 {
		t.Fatalf("bob should have nothing: %+v", items)
	}
}

func TestCommentReplyRules(t *testing.T) {
	f := newLibraryFixture(t)
	a, b := f.tok(t, f.alice), f.tok(t, f.bob)
	_, parent := f.postComment(t, a, 1, "parent", 0)
	_, other := f.postComment(t, a, 2, "other video", 0)

	// Replying to yourself does not notify.
	if code, _ := f.postComment(t, a, 1, "self reply", parent); code != http.StatusCreated {
		t.Fatalf("self reply code=%d", code)
	}
	if n := f.unread(t, a); n != 0 {
		t.Fatalf("self reply notified: %d", n)
	}

	// A parent from another video is rejected.
	if code, _ := f.postComment(t, b, 1, "wrong video", other); code != http.StatusBadRequest {
		t.Fatalf("cross-video code=%d", code)
	}
	// A missing parent is a 404.
	if code, _ := f.postComment(t, b, 1, "ghost", 9999); code != http.StatusNotFound {
		t.Fatalf("missing parent code=%d", code)
	}
	// parent_id 0 is invalid when sent explicitly.
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/videos/1/comments", `{"body":"x","parent_id":0}`, b); w.Code != http.StatusBadRequest {
		t.Fatalf("zero parent code=%d", w.Code)
	}
	// A deleted parent can no longer be replied to.
	if err := f.comments.SoftDeleteComment(parent, f.alice.ID, *f.clock); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.postComment(t, b, 1, "to deleted", parent); code != http.StatusNotFound {
		t.Fatalf("deleted parent code=%d", code)
	}
	if n := f.unread(t, a); n != 0 {
		t.Fatalf("rejected replies must not notify: %d", n)
	}

	// Flat comments still work and never notify.
	if code, _ := f.postComment(t, b, 1, "plain", 0); code != http.StatusCreated {
		t.Fatalf("flat comment code=%d", code)
	}
	if n := f.unread(t, a); n != 0 {
		t.Fatalf("flat comment notified: %d", n)
	}
}

func TestNotificationsMarkReadAndOwnership(t *testing.T) {
	f := newLibraryFixture(t)
	a, b := f.tok(t, f.alice), f.tok(t, f.bob)
	_, aParent := f.postComment(t, a, 1, "a-top", 0)
	_, bParent := f.postComment(t, b, 1, "b-top", 0)
	for i := 0; i < 3; i++ {
		f.tick(time.Minute)
		f.postComment(t, b, 1, "reply to a "+strconv.Itoa(i), aParent)
	}
	f.tick(time.Minute)
	f.postComment(t, a, 1, "reply to b", bParent)

	_, aItems := f.notifs(t, a, "")
	_, bItems := f.notifs(t, b, "")
	if len(aItems) != 3 || len(bItems) != 1 {
		t.Fatalf("counts a=%d b=%d", len(aItems), len(bItems))
	}
	// Newest first.
	if aItems[0].ID < aItems[1].ID || aItems[1].ID < aItems[2].ID {
		t.Fatalf("order = %+v", aItems)
	}
	if f.unread(t, a) != 3 || f.unread(t, b) != 1 {
		t.Fatal("unread counts wrong")
	}

	// Bob tries to mark Alice's notification: nothing changes for Alice.
	w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/notifications/read", `{"ids":[`+strconv.FormatUint(aItems[0].ID, 10)+`]}`, b)
	if w.Code != http.StatusOK {
		t.Fatalf("foreign mark code=%d", w.Code)
	}
	if f.unread(t, a) != 3 {
		t.Fatal("bob marked alice's notification as read")
	}

	// Alice marks one of her own.
	w = doJSON(t, f.r, http.MethodPost, "/api/v1/me/notifications/read", `{"ids":[`+strconv.FormatUint(aItems[0].ID, 10)+`, 99999]}`, a)
	if w.Code != http.StatusOK {
		t.Fatalf("mark code=%d body=%s", w.Code, w.Body.String())
	}
	var marked struct {
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &marked)
	if marked.Data.UnreadCount != 2 || f.unread(t, a) != 2 {
		t.Fatalf("after mark unread = %d", marked.Data.UnreadCount)
	}
	_, aItems = f.notifs(t, a, "")
	if !aItems[0].IsRead || aItems[1].IsRead {
		t.Fatalf("read flags = %+v", aItems)
	}
	if f.unread(t, b) != 1 {
		t.Fatal("bob's unread changed")
	}

	// Mark all read only affects the caller.
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/notifications/read", `{"all":true}`, a); w.Code != http.StatusOK {
		t.Fatalf("mark all code=%d", w.Code)
	}
	if f.unread(t, a) != 0 || f.unread(t, b) != 1 {
		t.Fatalf("after mark all a=%d b=%d", f.unread(t, a), f.unread(t, b))
	}

	for _, body := range []string{`{}`, `{"ids":[]}`, `nope`} {
		if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/notifications/read", body, a); w.Code != http.StatusBadRequest {
			t.Errorf("body %q code=%d", body, w.Code)
		}
	}
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 1)
	}
	if w := doJSON(t, f.r, http.MethodPost, "/api/v1/me/notifications/read", `{"ids":[`+strings.Join(ids, ",")+`]}`, a); w.Code != http.StatusBadRequest {
		t.Errorf("101 ids code=%d", w.Code)
	}
}

func TestNotificationsKindFilterAndPagination(t *testing.T) {
	f := newLibraryFixture(t)
	a := f.tok(t, f.alice)
	base := *f.clock
	for i := 0; i < 5; i++ {
		kind := store.NotificationKindSystem
		if i%2 == 0 {
			kind = store.NotificationKindCommentReply
		}
		if _, err := f.notifications.CreateNotification(store.Notification{
			UserID: f.alice.ID, Kind: kind, Title: "n" + strconv.Itoa(i), CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A different user's notification never appears.
	if _, err := f.notifications.CreateNotification(store.Notification{UserID: f.bob.ID, Kind: store.NotificationKindSystem, Title: "bob", CreatedAt: base}); err != nil {
		t.Fatal(err)
	}

	l, items := f.notifs(t, a, "")
	if l.Total != 5 || l.Limit != 20 || len(items) != 5 || items[0].Title != "n4" {
		t.Fatalf("all = %+v %+v", l, items)
	}
	l, items = f.notifs(t, a, "?kind=comment_reply")
	if l.Total != 3 || len(items) != 3 {
		t.Fatalf("reply filter = %+v", l)
	}
	for _, it := range items {
		if it.Kind != store.NotificationKindCommentReply {
			t.Fatalf("filter leaked %s", it.Kind)
		}
	}
	l, items = f.notifs(t, a, "?kind=system")
	if l.Total != 2 || len(items) != 2 {
		t.Fatalf("system filter = %+v", l)
	}
	if w := doJSON(t, f.r, http.MethodGet, "/api/v1/me/notifications?kind=bogus", "", a); w.Code != http.StatusBadRequest {
		t.Fatalf("bad kind code=%d", w.Code)
	}

	l, items = f.notifs(t, a, "?limit=2")
	if len(items) != 2 || !l.HasMore || l.Limit != 2 {
		t.Fatalf("page1 = %+v", l)
	}
	l, items = f.notifs(t, a, "?limit=2&offset=4")
	if len(items) != 1 || l.HasMore || items[0].Title != "n0" {
		t.Fatalf("page3 = %+v %+v", l, items)
	}
	l, _ = f.notifs(t, a, "?limit=500")
	if l.Limit != 50 {
		t.Fatalf("limit cap = %d", l.Limit)
	}

	// System notifications carry no actor and no links.
	var sys notifItem
	_, items = f.notifs(t, a, "?kind=system")
	sys = items[0]
	if sys.ActorName != "" || sys.VideoID != nil || sys.CommentID != nil {
		t.Fatalf("system = %+v", sys)
	}
}
