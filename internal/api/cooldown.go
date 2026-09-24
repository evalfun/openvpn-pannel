package api

import (
	"sync"
	"time"
)

// loginCooldownInterval 为同一用户两次登录/验证码尝试之间的最小间隔。
// 用于防止密码与动态验证码被快速暴力枚举。
const loginCooldownInterval = 2 * time.Second

// cooldownLimiter 为每个 key（如用户名）提供最小尝试间隔限制：
// 同一 key 在 interval 内只允许一次尝试。
type cooldownLimiter struct {
	mu       sync.Mutex
	last     map[string]time.Time
	interval time.Duration
	// maxEntries 触发惰性清理的阈值：随机 key 可能撑大 map，超过阈值时清掉已过期的条目。
	maxEntries int
}

func newCooldownLimiter(interval time.Duration) *cooldownLimiter {
	return &cooldownLimiter{
		last:       make(map[string]time.Time),
		interval:   interval,
		maxEntries: 4096,
	}
}

// Allow 判断 key 当前是否允许尝试。允许时记录本次尝试时间并返回 (true, 0)；
// 处于冷却期则返回 (false, 剩余冷却时间)。
func (l *cooldownLimiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.last[key]; ok {
		if remain := l.interval - now.Sub(last); remain > 0 {
			return false, remain
		}
	}
	l.last[key] = now
	if len(l.last) > l.maxEntries {
		for k, t := range l.last {
			if now.Sub(t) >= l.interval {
				delete(l.last, k)
			}
		}
	}
	return true, 0
}

// retryAfterSeconds 把剩余冷却时间向上取整为秒数，至少为 1。
func retryAfterSeconds(remain time.Duration) int {
	secs := int(remain / time.Second)
	if remain%time.Second != 0 {
		secs++
	}
	if secs < 1 {
		secs = 1
	}
	return secs
}
