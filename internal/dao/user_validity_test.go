package dao

import (
	"errors"
	"testing"
	"time"

	"openvpn-pannel/internal/models"
)

// 认证时必须拦截已禁用与已过有效期的用户；未失效的用户不受影响。
func TestAuthUserRejectsDisabledAndExpired(t *testing.T) {
	dm := newTestDaoManager(t)

	// 正常用户可认证
	if err := dm.CreateUser("normal", "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, 0); err != nil {
		t.Fatalf("create normal: %v", err)
	}
	if _, err := dm.AuthUser("normal", "pw"); err != nil {
		t.Fatalf("normal auth: %v", err)
	}

	// 有效期已过：认证被拒，且错误可识别
	past := uint64(time.Now().Add(-time.Hour).Unix())
	if err := dm.CreateUser("expired", "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, past); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	if _, err := dm.AuthUser("expired", "pw"); !errors.Is(err, models.ErrUserExpired) {
		t.Fatalf("expired auth err = %v, want ErrUserExpired", err)
	}

	// 有效期在未来：允许认证
	future := uint64(time.Now().Add(time.Hour).Unix())
	if err := dm.CreateUser("future", "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, future); err != nil {
		t.Fatalf("create future: %v", err)
	}
	if _, err := dm.AuthUser("future", "pw"); err != nil {
		t.Fatalf("future auth: %v", err)
	}

	// 配错密码仍然返回通用认证失败，不泄露账号状态
	if _, err := dm.AuthUser("expired", "wrong"); err == nil || errors.Is(err, models.ErrUserExpired) {
		t.Fatalf("expired wrong password err = %v, want generic auth failure", err)
	}
}

// 禁用/启用接口：禁用后无法认证，重新启用后恢复。
func TestSetUsersDisabled(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateUser("u1", "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, 0); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := dm.CreateUser("u2", "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, 0); err != nil {
		t.Fatalf("create u2: %v", err)
	}
	u1 := mustUser(t, dm, "u1")
	u2 := mustUser(t, dm, "u2")

	// 批量禁用
	if err := dm.SetUsersDisabled([]uint{u1.ID, u2.ID}, true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	for _, name := range []string{"u1", "u2"} {
		if _, err := dm.AuthUser(name, "pw"); !errors.Is(err, models.ErrUserDisabled) {
			t.Fatalf("%s auth err = %v, want ErrUserDisabled", name, err)
		}
	}

	// 只重新启用 u1
	if err := dm.SetUsersDisabled([]uint{u1.ID}, false); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, err := dm.AuthUser("u1", "pw"); err != nil {
		t.Fatalf("u1 re-enabled auth: %v", err)
	}
	if _, err := dm.AuthUser("u2", "pw"); !errors.Is(err, models.ErrUserDisabled) {
		t.Fatalf("u2 should stay disabled, err = %v", err)
	}

	// 空列表为无操作
	if err := dm.SetUsersDisabled(nil, true); err != nil {
		t.Fatalf("empty list: %v", err)
	}
}

// 更新用户信息应能写入有效期（0=永不过期）。
func TestUpdateUserInfoSetsExpireAt(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateUser("u1", "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, 0); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	u := mustUser(t, dm, "u1")
	expireAt := uint64(time.Now().Add(24 * time.Hour).Unix())
	if err := dm.UpdateUserInfo(u.ID, "desc", "", u.RateLimitType, 0, 0, expireAt); err != nil {
		t.Fatalf("UpdateUserInfo: %v", err)
	}
	got := mustUser(t, dm, "u1")
	if got.ExpireAt != expireAt {
		t.Fatalf("ExpireAt = %d, want %d", got.ExpireAt, expireAt)
	}
	if got.Description != "desc" {
		t.Fatalf("Description = %q, want desc", got.Description)
	}
}

// 批量设置有效期：一次写入多个用户，0 表示永久。
func TestSetUsersExpireAt(t *testing.T) {
	dm := newTestDaoManager(t)
	for _, name := range []string{"u1", "u2", "u3"} {
		if err := dm.CreateUser(name, "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0, 0); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	u1 := mustUser(t, dm, "u1")
	u2 := mustUser(t, dm, "u2")

	expireAt := uint64(time.Now().Add(48 * time.Hour).Unix())
	if err := dm.SetUsersExpireAt([]uint{u1.ID, u2.ID}, expireAt); err != nil {
		t.Fatalf("SetUsersExpireAt: %v", err)
	}
	if got := mustUser(t, dm, "u1"); got.ExpireAt != expireAt {
		t.Fatalf("u1.ExpireAt = %d, want %d", got.ExpireAt, expireAt)
	}
	if got := mustUser(t, dm, "u2"); got.ExpireAt != expireAt {
		t.Fatalf("u2.ExpireAt = %d, want %d", got.ExpireAt, expireAt)
	}
	if got := mustUser(t, dm, "u3"); got.ExpireAt != 0 {
		t.Fatalf("u3.ExpireAt = %d, want 0（未在批量范围内）", got.ExpireAt)
	}

	// 置 0 表示恢复为永久
	if err := dm.SetUsersExpireAt([]uint{u1.ID, u2.ID}, 0); err != nil {
		t.Fatalf("SetUsersExpireAt(0): %v", err)
	}
	if got := mustUser(t, dm, "u1"); got.ExpireAt != 0 {
		t.Fatalf("u1.ExpireAt = %d, want 0", got.ExpireAt)
	}

	// 空列表为无操作
	if err := dm.SetUsersExpireAt(nil, 123); err != nil {
		t.Fatalf("empty list: %v", err)
	}
}
