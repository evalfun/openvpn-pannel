package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ginsessions "github.com/gin-contrib/sessions"
)

func newTestSessionStore() *memorySessionStore {
	store := newMemorySessionStore([]byte("test-secret-key-32-bytes-long-0000"))
	store.Options(ginsessions.Options{Path: "/", HttpOnly: true, MaxAge: 36000})
	return store
}

// TestMemorySessionStoreRoundTrip 验证：会话本体保存在内存，Cookie 只带会话 ID，
// 且能跨请求取回数据；登出（MaxAge=-1）后服务端会话被删除。
func TestMemorySessionStoreRoundTrip(t *testing.T) {
	store := newTestSessionStore()

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	s1, err := store.Get(req1, "session")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	s1.Values["user_id"] = uint(12345)
	if err := s1.Save(req1, rec1); err != nil {
		t.Fatalf("save: %v", err)
	}
	if s1.ID == "" {
		t.Fatal("session ID should be assigned on save")
	}

	cookies := rec1.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie set")
	}
	cookie := cookies[0]
	// Cookie 中不应出现会话数据本身（user_id 的值）。
	if strings.Contains(cookie.Value, "12345") {
		t.Fatalf("session data leaked into cookie: %s", cookie.Value)
	}

	// 第二次请求携带 Cookie：应能取回内存中的会话数据。
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(cookie)
	s2, err := store.Get(req2, "session")
	if err != nil {
		t.Fatalf("get with cookie: %v", err)
	}
	if got := s2.Values["user_id"]; got != uint(12345) {
		t.Fatalf("user_id = %v, want 12345", got)
	}
	if s2.IsNew {
		t.Fatal("session should not be new when cookie is valid")
	}

	// 登出：删除服务端会话并清除 Cookie。
	s2.Options.MaxAge = -1
	if err := s2.Save(req2, httptest.NewRecorder()); err != nil {
		t.Fatalf("logout save: %v", err)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.AddCookie(cookie)
	s3, err := store.Get(req3, "session")
	if err != nil {
		t.Fatalf("get after logout: %v", err)
	}
	if got := s3.Values["user_id"]; got != nil {
		t.Fatalf("session should be cleared after logout, got %v", got)
	}
}

// TestMemorySessionStoreTamperedCookie 伪造/损坏的 Cookie 应被视为新会话，且不报错。
func TestMemorySessionStoreTamperedCookie(t *testing.T) {
	store := newTestSessionStore()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "not-a-valid-session-cookie"})
	s, err := store.Get(req, "session")
	if err != nil {
		t.Fatalf("unexpected error for tampered cookie: %v", err)
	}
	if !s.IsNew {
		t.Fatal("tampered cookie should yield a new session")
	}
	if len(s.Values) != 0 {
		t.Fatalf("new session should have no values, got %v", s.Values)
	}
}

// TestMemorySessionStorePruneExpired 过期会话应被清理。
func TestMemorySessionStorePruneExpired(t *testing.T) {
	store := newTestSessionStore()
	store.entries["alive"] = &memorySessionEntry{values: map[interface{}]interface{}{"user_id": uint(1)}, expires: time.Now().Add(time.Hour)}
	store.entries["dead"] = &memorySessionEntry{values: map[interface{}]interface{}{"user_id": uint(2)}, expires: time.Now().Add(-time.Minute)}

	store.pruneExpired(time.Now())

	if _, ok := store.entries["alive"]; !ok {
		t.Fatal("non-expired session should be kept")
	}
	if _, ok := store.entries["dead"]; ok {
		t.Fatal("expired session should be pruned")
	}
}
