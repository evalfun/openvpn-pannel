package dao

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"openvpn-pannel/internal/models"
	"openvpn-pannel/internal/passwd"
)

// TestCreateUserUsesBcrypt 验证新用户密码以 bcrypt 存储且可登录。
func TestCreateUserUsesBcrypt(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateUser("alice", "s3cret", "", models.RATE_LIMIT_TYPE_NONE, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "alice")
	if !passwd.IsBcrypt(u.Password) {
		t.Fatalf("password should be bcrypt, got %q", u.Password)
	}
	if _, err := dm.AuthUser("alice", "s3cret"); err != nil {
		t.Fatalf("AuthUser correct password: %v", err)
	}
	if _, err := dm.AuthUser("alice", "wrong"); err == nil {
		t.Fatal("AuthUser with wrong password should fail")
	}
}

// TestAuthUserLegacyUpgrade 验证旧版 sha256 哈希可登录并在成功后透明升级为 bcrypt。
func TestAuthUserLegacyUpgrade(t *testing.T) {
	dm := newTestDaoManager(t)
	sum := sha256.Sum256([]byte("legacypw" + dm.cfg.PasswordSalt))
	legacy := fmt.Sprintf("%x", sum)
	if err := dm.CreateUser("legacy", "legacypw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0); err != nil {
		t.Fatalf("create user: %v", err)
	}
	// 把密码改写为旧版 sha256 哈希，模拟历史数据。
	if err := dm.DB.Model(&models.User{}).Where("username = ?", "legacy").Update("password", legacy).Error; err != nil {
		t.Fatalf("set legacy password: %v", err)
	}

	if _, err := dm.AuthUser("legacy", "legacypw"); err != nil {
		t.Fatalf("legacy login should succeed: %v", err)
	}
	u := mustUser(t, dm, "legacy")
	if !passwd.IsBcrypt(u.Password) {
		t.Fatalf("legacy hash should be upgraded to bcrypt, got %q", u.Password)
	}
	// 升级后仍可正常登录，且错误密码失败。
	if _, err := dm.AuthUser("legacy", "legacypw"); err != nil {
		t.Fatalf("login after upgrade: %v", err)
	}
	if _, err := dm.AuthUser("legacy", "nope"); err == nil {
		t.Fatal("wrong password after upgrade should fail")
	}
}

// TestUpdateUserPasswordUsesBcrypt 验证修改密码后使用 bcrypt。
func TestUpdateUserPasswordUsesBcrypt(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateUser("bob", "oldpw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "bob")
	if err := dm.UpdateUserInfo(u.ID, "", "newpw", u.RateLimitType, 0, 0); err != nil {
		t.Fatalf("UpdateUserInfo: %v", err)
	}
	updated := mustUser(t, dm, "bob")
	if !passwd.IsBcrypt(updated.Password) {
		t.Fatalf("updated password should be bcrypt, got %q", updated.Password)
	}
	if _, err := dm.AuthUser("bob", "newpw"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

// TestBatchResetUserTraffic 验证批量清除流量：历史流量与在线会话流量同时归零，且不影响未选中的用户。
func TestBatchResetUserTraffic(t *testing.T) {
	dm := newTestDaoManager(t)
	for _, n := range []string{"u1", "u2", "u3"} {
		if err := dm.CreateUser(n, "pw", "", models.RATE_LIMIT_TYPE_NONE, 0, 0); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	u1 := mustUser(t, dm, "u1")
	u2 := mustUser(t, dm, "u2")
	u3 := mustUser(t, dm, "u3")

	for _, u := range []*models.User{u1, u2, u3} {
		if err := dm.UpdateUserTraffic(u.Username, 100, 200); err != nil {
			t.Fatalf("update traffic %s: %v", u.Username, err)
		}
		if err := dm.DB.Create(&models.ConnectedClientInfoRecord{
			VirtualIPAddr: "10.8.0." + u.Username[1:], ServerID: 1, Username: u.Username,
			ByteReceived: 10, ByteSent: 20,
		}).Error; err != nil {
			t.Fatalf("create record %s: %v", u.Username, err)
		}
	}

	if err := dm.BatchResetUserTraffic([]uint{u1.ID, u2.ID}); err != nil {
		t.Fatalf("BatchResetUserTraffic: %v", err)
	}

	for _, name := range []string{"u1", "u2"} {
		u := mustUser(t, dm, name)
		if u.UploadTraffic != 0 || u.DownloadTraffic != 0 {
			t.Fatalf("%s 历史流量未清零: %d/%d", name, u.UploadTraffic, u.DownloadTraffic)
		}
		var rec models.ConnectedClientInfoRecord
		if err := dm.DB.Where("username = ?", name).First(&rec).Error; err != nil {
			t.Fatalf("get record %s: %v", name, err)
		}
		if rec.ByteReceived != 0 || rec.ByteSent != 0 {
			t.Fatalf("%s 会话流量未清零: %d/%d", name, rec.ByteReceived, rec.ByteSent)
		}
	}
	u3b := mustUser(t, dm, "u3")
	if u3b.UploadTraffic != 100 || u3b.DownloadTraffic != 200 {
		t.Fatalf("u3 不应受影响: %d/%d", u3b.UploadTraffic, u3b.DownloadTraffic)
	}
	var rec3 models.ConnectedClientInfoRecord
	if err := dm.DB.Where("username = ?", "u3").First(&rec3).Error; err != nil {
		t.Fatalf("get u3 record: %v", err)
	}
	if rec3.ByteReceived != 10 || rec3.ByteSent != 20 {
		t.Fatalf("u3 会话流量不应受影响: %d/%d", rec3.ByteReceived, rec3.ByteSent)
	}
}
