package dao

import (
	"testing"

	"openvpn-pannel/internal/models"
)

func TestSetUserMFA(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateUser("alice", "pw", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "alice")

	if err := dm.SetUserMFA(u.ID, models.MFA_TYPE_TOTP, "ABCDEFGH"); err != nil {
		t.Fatalf("SetUserMFA enable: %v", err)
	}
	u = mustUser(t, dm, "alice")
	if u.MFAType != models.MFA_TYPE_TOTP || u.MFAData != "ABCDEFGH" {
		t.Fatalf("启用后 = type:%d data:%q", u.MFAType, u.MFAData)
	}

	// 关闭时必须清空认证数据
	if err := dm.SetUserMFA(u.ID, models.MFA_TYPE_NONE, "IGNORED"); err != nil {
		t.Fatalf("SetUserMFA disable: %v", err)
	}
	u = mustUser(t, dm, "alice")
	if u.MFAType != models.MFA_TYPE_NONE || u.MFAData != "" {
		t.Fatalf("关闭后 = type:%d data:%q", u.MFAType, u.MFAData)
	}
}

func TestConnectedClientInfoRecordMFA(t *testing.T) {
	dm := newTestDaoManager(t)
	rec := &models.ConnectedClientInfoRecord{
		VirtualIPAddr:  "10.8.0.2",
		ServerID:       1,
		Username:       "alice",
		ClientCertName: "cert-alice",
		RealIPAddr:     "203.0.113.9:5000",
	}
	if err := dm.CreateConnectedClientInfoRecord(rec); err != nil {
		t.Fatalf("CreateConnectedClientInfoRecord: %v", err)
	}

	list, err := dm.ListConnectedClientInfoRecordByVirtualIP("10.8.0.2")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListConnectedClientInfoRecordByVirtualIP = %v, %v", list, err)
	}
	if list[0].ClientCertName != "cert-alice" || list[0].RealIPAddr != "203.0.113.9:5000" {
		t.Fatalf("cert/realip 未保存: %+v", list[0])
	}
	if list[0].MFAVerified {
		t.Fatalf("新建记录 MFAVerified 应为 false")
	}

	if err := dm.UpdateConnectedClientInfoRecordMFAVerified(1, "10.8.0.2", true); err != nil {
		t.Fatalf("UpdateConnectedClientInfoRecordMFAVerified: %v", err)
	}
	got, err := dm.GetConnectedClientInfoRecord(1, "10.8.0.2")
	if err != nil || !got.MFAVerified {
		t.Fatalf("MFAVerified 更新失败: %+v, %v", got, err)
	}
}
