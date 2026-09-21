package api

import (
	"encoding/base32"
	"net/http"
	"strings"
	"sync"
	"time"

	ginsessions "github.com/gin-contrib/sessions"
	"github.com/gorilla/securecookie"
	gsessions "github.com/gorilla/sessions"
)

// memorySessionStore 是基于内存的会话存储：浏览器 Cookie 中只保存随机会话 ID（经签名），
// 会话本体数据保存在进程内存中。相比把整段会话数据加密后放进 Cookie 的实现，
// 不会把会话内容暴露/携带到客户端，也不会因会话数据增长而撑大请求头。
//
// 说明：会话保存在单个进程内存里，面板重启后需要重新登录；内置过期清理避免会话常驻占用内存。
type memorySessionStore struct {
	codecs  []securecookie.Codec
	options *gsessions.Options

	mu      sync.RWMutex
	entries map[string]*memorySessionEntry
}

type memorySessionEntry struct {
	values  map[interface{}]interface{}
	expires time.Time
}

const (
	memorySessionDefaultMaxAge = 36000 // 10 小时
	memorySessionCleanupTick   = time.Minute
)

func newMemorySessionStore(authKey []byte) *memorySessionStore {
	store := &memorySessionStore{
		codecs:  securecookie.CodecsFromPairs(authKey),
		options: &gsessions.Options{Path: "/", MaxAge: memorySessionDefaultMaxAge, HttpOnly: true},
		entries: make(map[string]*memorySessionEntry),
	}
	store.applyCodecMaxAge()
	go store.cleanupLoop()
	return store
}

// Options 设置存储级默认选项（满足 gin-contrib sessions.Store 接口）。
func (s *memorySessionStore) Options(options ginsessions.Options) {
	s.options = options.ToGorillaOptions()
	s.applyCodecMaxAge()
}

func (s *memorySessionStore) applyCodecMaxAge() {
	for _, codec := range s.codecs {
		if sc, ok := codec.(*securecookie.SecureCookie); ok {
			sc.MaxAge(s.options.MaxAge)
		}
	}
}

// Get 返回请求对应的会话；不存在时为新建会话。
func (s *memorySessionStore) Get(r *http.Request, name string) (*gsessions.Session, error) {
	return gsessions.GetRegistry(r).Get(s, name)
}

// New 解析 Cookie 中的会话 ID 并装载内存中的会话数据。
// 解密失败（过期或伪造）时视为新会话，不返回错误，避免产生告警日志。
func (s *memorySessionStore) New(r *http.Request, name string) (*gsessions.Session, error) {
	session := gsessions.NewSession(s, name)
	opts := *s.options
	session.Options = &opts
	session.IsNew = true

	cookie, err := r.Cookie(name)
	if err != nil {
		return session, nil
	}
	var id string
	if err := securecookie.DecodeMulti(name, cookie.Value, &id, s.codecs...); err != nil {
		return session, nil
	}
	session.ID = id

	s.mu.RLock()
	entry, ok := s.entries[id]
	if ok && time.Now().Before(entry.expires) {
		session.Values = cloneSessionValues(entry.values)
		session.IsNew = false
	}
	s.mu.RUnlock()
	return session, nil
}

// Save 把会话数据写入内存，并仅在 Cookie 中写入会话 ID；MaxAge<0 时删除会话。
func (s *memorySessionStore) Save(r *http.Request, w http.ResponseWriter, session *gsessions.Session) error {
	if session.Options.MaxAge < 0 {
		s.mu.Lock()
		delete(s.entries, session.ID)
		s.mu.Unlock()
		session.Values = make(map[interface{}]interface{})
		http.SetCookie(w, gsessions.NewCookie(session.Name(), "", session.Options))
		return nil
	}

	if session.ID == "" {
		session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
	}
	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.codecs...)
	if err != nil {
		return err
	}

	maxAge := session.Options.MaxAge
	if maxAge <= 0 {
		maxAge = s.options.MaxAge
		if maxAge <= 0 {
			maxAge = memorySessionDefaultMaxAge
		}
	}

	s.mu.Lock()
	s.entries[session.ID] = &memorySessionEntry{
		values:  cloneSessionValues(session.Values),
		expires: time.Now().Add(time.Duration(maxAge) * time.Second),
	}
	s.mu.Unlock()

	http.SetCookie(w, gsessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

// cleanupLoop 周期清理过期会话，避免内存中沉积大量已失效会话。
func (s *memorySessionStore) cleanupLoop() {
	ticker := time.NewTicker(memorySessionCleanupTick)
	defer ticker.Stop()
	for range ticker.C {
		s.pruneExpired(time.Now())
	}
}

// pruneExpired 删除已过期的会话（便于测试直接调用）。
func (s *memorySessionStore) pruneExpired(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, entry := range s.entries {
		if now.After(entry.expires) {
			delete(s.entries, id)
		}
	}
}

func cloneSessionValues(values map[interface{}]interface{}) map[interface{}]interface{} {
	if values == nil {
		return make(map[interface{}]interface{})
	}
	cloned := make(map[interface{}]interface{}, len(values))
	for k, v := range values {
		cloned[k] = v
	}
	return cloned
}
