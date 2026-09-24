package api

import (
	"testing"
	"time"
)

func TestCooldownLimiter(t *testing.T) {
	l := newCooldownLimiter(2 * time.Second)

	if ok, _ := l.Allow("u1"); !ok {
		t.Fatal("first attempt should be allowed")
	}
	// 冷却期内第二次尝试被拒绝，并返回剩余时间。
	ok, remain := l.Allow("u1")
	if ok {
		t.Fatal("second attempt within cooldown should be rejected")
	}
	if remain <= 0 || remain > 2*time.Second {
		t.Fatalf("remaining = %v, want (0,2s]", remain)
	}
	// 不同 key 互不影响。
	if ok, _ := l.Allow("u2"); !ok {
		t.Fatal("different key should be allowed")
	}
	// 模拟冷却时间已过（直接改写记录）。
	l.mu.Lock()
	l.last["u1"] = time.Now().Add(-3 * time.Second)
	l.mu.Unlock()
	if ok, _ := l.Allow("u1"); !ok {
		t.Fatal("attempt after cooldown should be allowed")
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want int
	}{
		{0, 1},
		{100 * time.Millisecond, 1},
		{1 * time.Second, 1},
		{1500 * time.Millisecond, 2},
		{2 * time.Second, 2},
	}
	for _, c := range cases {
		if got := retryAfterSeconds(c.in); got != c.want {
			t.Fatalf("retryAfterSeconds(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
