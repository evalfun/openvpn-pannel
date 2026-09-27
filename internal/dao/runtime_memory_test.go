package dao

import (
	"testing"

	"openvpn-pannel/internal/models"
)

// 内存运行时模式下，在线会话记录的增删改查都应走内存，数据库保持为空。
func TestRuntimeMemoryConnectedClients(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	if !dm.useMemoryRuntime() {
		t.Fatalf("max_memory_events>0 时应启用内存运行时数据")
	}

	rec := &models.ConnectedClientInfoRecord{
		VirtualIPAddr:   "10.8.0.2",
		VirtualIP6Addr:  "fd00::2",
		ServerID:        1,
		Username:        "alice",
		ByteReceived:    10,
		ByteSent:        20,
		UploadLimitKB:   100,
		DownloadLimitKB: 200,
		ClientCertName:  "cert-a",
		RealIPAddr:      "1.2.3.4",
	}
	if err := dm.CreateConnectedClientInfoRecord(rec); err != nil {
		t.Fatalf("CreateConnectedClientInfoRecord: %v", err)
	}

	// 读回
	got, err := dm.GetConnectedClientInfoRecord(1, "10.8.0.2")
	if err != nil {
		t.Fatalf("GetConnectedClientInfoRecord: %v", err)
	}
	if got.Username != "alice" || got.UploadLimitKB != 100 {
		t.Fatalf("记录内容异常: %+v", got)
	}

	// 流量更新
	if err := dm.UpdateConnectedClientInfoRecordTraffic(1, "10.8.0.2", 111, 222); err != nil {
		t.Fatalf("UpdateTraffic: %v", err)
	}
	got, _ = dm.GetConnectedClientInfoRecord(1, "10.8.0.2")
	if got.ByteReceived != 111 || got.ByteSent != 222 {
		t.Fatalf("流量更新失败: %+v", got)
	}

	// 限速更新
	if err := dm.UpdateConnectedClientInfoRecordLimit(1, "10.8.0.2", 5, 6); err != nil {
		t.Fatalf("UpdateLimit: %v", err)
	}
	got, _ = dm.GetConnectedClientInfoRecord(1, "10.8.0.2")
	if got.UploadLimitKB != 5 || got.DownloadLimitKB != 6 {
		t.Fatalf("限速更新失败: %+v", got)
	}

	// MFA 状态更新
	if err := dm.UpdateConnectedClientInfoRecordMFAVerified(1, "10.8.0.2", true); err != nil {
		t.Fatalf("UpdateMFA: %v", err)
	}
	got, _ = dm.GetConnectedClientInfoRecord(1, "10.8.0.2")
	if !got.MFAVerified {
		t.Fatalf("MFA 状态未更新")
	}

	// 多视图查询
	if n, _ := dm.CountConnectedClientInfoRecords(); n != 1 {
		t.Fatalf("Count = %d, 期望 1", n)
	}
	if l, _ := dm.ListConnectedClientInfoRecordByServerID(1); len(l) != 1 {
		t.Fatalf("ListByServerID len = %d, 期望 1", len(l))
	}
	if l, _ := dm.ListConnectedClientInfoRecord(); len(l) != 1 {
		t.Fatalf("List len = %d, 期望 1", len(l))
	}
	if l, _ := dm.ListConnectedClientInfoRecordByUsername("alice"); len(l) != 1 {
		t.Fatalf("ListByUsername len = %d, 期望 1", len(l))
	}
	if l, _ := dm.ListConnectedClientInfoRecordByVirtualIP("fd00::2"); len(l) != 1 {
		t.Fatalf("ListByVirtualIP(ipv6) len = %d, 期望 1", len(l))
	}

	// 删除
	if err := dm.DeleteConnectedClientInfoRecord("10.8.0.2", 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := dm.GetConnectedClientInfoRecord(1, "10.8.0.2"); err == nil {
		t.Fatalf("删除后应查不到")
	}
	if n, _ := dm.CountConnectedClientInfoRecords(); n != 0 {
		t.Fatalf("删除后 Count = %d, 期望 0", n)
	}

	// 数据库始终为空
	var dbCount int64
	dm.DB.Model(&models.ConnectedClientInfoRecord{}).Count(&dbCount)
	if dbCount != 0 {
		t.Fatalf("数据库在线会话记录数 = %d, 期望 0", dbCount)
	}
}

// 按服务器删除在线会话应只影响目标服务器。
func TestRuntimeMemoryDeleteClientsByServer(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	_ = dm.CreateConnectedClientInfoRecord(&models.ConnectedClientInfoRecord{ServerID: 1, VirtualIPAddr: "10.0.0.1", Username: "a"})
	_ = dm.CreateConnectedClientInfoRecord(&models.ConnectedClientInfoRecord{ServerID: 2, VirtualIPAddr: "10.0.0.2", Username: "b"})

	if err := dm.DeleteConnectedClientInfoRecordByServerID(1); err != nil {
		t.Fatalf("DeleteByServer: %v", err)
	}
	if l, _ := dm.ListConnectedClientInfoRecordByServerID(1); len(l) != 0 {
		t.Fatalf("server1 应清空, len=%d", len(l))
	}
	if l, _ := dm.ListConnectedClientInfoRecordByServerID(2); len(l) != 1 {
		t.Fatalf("server2 不应受影响, len=%d", len(l))
	}
}

// 内存运行时模式下，已下发 ACL 记录走内存，数据库保持为空；且 /user/acl/del 依赖的"先读后删"语义正确。
func TestRuntimeMemoryAddedACL(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)

	acls := []*models.AddedServerACLRecord{
		{ServerID: 1, VirtualIPAddr: "10.0.0.2", VirtualIP6Addr: "fd00::2", ACLType: 1, ACLValue: "10.0.0.0/8"},
		{ServerID: 1, VirtualIPAddr: "10.0.0.2", VirtualIP6Addr: "fd00::2", ACLType: 1, ACLValue: "192.168.0.0/16"},
	}
	if err := dm.SaveAddedACL(acls); err != nil {
		t.Fatalf("SaveAddedACL: %v", err)
	}

	// 先读：脚本需要拿到记录才能清理
	list, err := dm.ListAddedACLByIP("10.0.0.2", 1)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListAddedACLByIP len=%d err=%v, 期望 2", len(list), err)
	}
	if list[0].ACLValue == "" {
		t.Fatalf("ACL 记录内容为空")
	}
	// 按服务器列出（服务器状态页用）
	if l, _ := dm.ListAddedACLByServerID(1); len(l) != 2 {
		t.Fatalf("ListAddedACLByServerID len=%d, 期望 2", len(l))
	}

	// 后删
	if err := dm.DeleteAddedACLByIP("10.0.0.2", 1); err != nil {
		t.Fatalf("DeleteAddedACLByIP: %v", err)
	}
	if l, _ := dm.ListAddedACLByIP("10.0.0.2", 1); len(l) != 0 {
		t.Fatalf("删除后应为空, len=%d", len(l))
	}

	// 数据库始终为空
	var dbCount int64
	dm.DB.Model(&models.AddedServerACLRecord{}).Count(&dbCount)
	if dbCount != 0 {
		t.Fatalf("数据库 ACL 记录数 = %d, 期望 0", dbCount)
	}

	// 按服务器删除
	_ = dm.SaveAddedACL([]*models.AddedServerACLRecord{{ServerID: 1, VirtualIPAddr: "10.0.0.3", ACLType: 1, ACLValue: "a"}})
	_ = dm.SaveAddedACL([]*models.AddedServerACLRecord{{ServerID: 2, VirtualIPAddr: "10.0.0.4", ACLType: 1, ACLValue: "b"}})
	if err := dm.DeleteAddedACLByServerID(1); err != nil {
		t.Fatalf("DeleteAddedACLByServerID: %v", err)
	}
	if l, _ := dm.ListAddedACLByServerID(1); len(l) != 0 {
		t.Fatalf("server1 ACL 应清空, len=%d", len(l))
	}
	if l, _ := dm.ListAddedACLByServerID(2); len(l) != 1 {
		t.Fatalf("server2 ACL 不应受影响, len=%d", len(l))
	}
}

// 内存运行时模式下，达量限速周期状态走内存，数据库 users 表不被写入。
func TestRuntimeMemoryRateLimitCycle(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)

	plan := &models.RateLimitPlan{Name: "p", PeriodSeconds: 3600, CreatedAt: 1}
	if err := dm.CreateRateLimitPlan(plan, nil); err != nil {
		t.Fatalf("CreateRateLimitPlan: %v", err)
	}
	if err := dm.CreateUser("cyc", "pass", "", models.RATE_LIMIT_TYPE_FIXED, 500, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "cyc")
	if _, err := dm.AddUsersToRateLimitPlan(plan.ID, []string{"cyc"}); err != nil {
		t.Fatalf("AddUsersToRateLimitPlan: %v", err)
	}

	// ensureRateLimitCycle 初始化周期：应写内存而非数据库
	if _, err := dm.ensureRateLimitCycle(u); err != nil {
		t.Fatalf("ensureRateLimitCycle: %v", err)
	}

	// 累加周期流量
	if err := dm.AddUserCycleTraffic("cyc", 300, 400); err != nil {
		t.Fatalf("AddUserCycleTraffic: %v", err)
	}
	up, down, err := dm.GetUserCycleTraffic(u.ID)
	if err != nil {
		t.Fatalf("GetUserCycleTraffic: %v", err)
	}
	if up != 300 || down != 400 {
		t.Fatalf("周期流量 = (%d,%d), 期望 (300,400)", up, down)
	}

	// 数据库中的周期字段应保持初始（0），证明未写库
	var dbUser models.User
	if err := dm.DB.First(&dbUser, u.ID).Error; err != nil {
		t.Fatalf("读取用户: %v", err)
	}
	if dbUser.RateLimitCycleUpload != 0 || dbUser.RateLimitCycleDownload != 0 {
		t.Fatalf("数据库周期流量被写入: (%d,%d), 期望 (0,0)", dbUser.RateLimitCycleUpload, dbUser.RateLimitCycleDownload)
	}

	// 内存中的周期状态会覆盖到读取的用户对象上
	u2, _ := dm.GetUserByID(u.ID)
	if u2.RateLimitCycleUpload != 300 || u2.RateLimitCycleDownload != 400 {
		t.Fatalf("读取用户周期字段未覆盖: (%d,%d)", u2.RateLimitCycleUpload, u2.RateLimitCycleDownload)
	}

	// 手动重置
	if err := dm.ResetUserRateLimitCycle(u.ID); err != nil {
		t.Fatalf("ResetUserRateLimitCycle: %v", err)
	}
	if up, down, _ := dm.GetUserCycleTraffic(u.ID); up != 0 || down != 0 {
		t.Fatalf("重置后周期流量 = (%d,%d), 期望 (0,0)", up, down)
	}

	// 数据库周期字段仍为 0
	if err := dm.DB.First(&dbUser, u.ID).Error; err != nil {
		t.Fatalf("读取用户: %v", err)
	}
	if dbUser.RateLimitCycleUpload != 0 || dbUser.RateLimitCycleDownload != 0 {
		t.Fatalf("重置写入数据库: (%d,%d)", dbUser.RateLimitCycleUpload, dbUser.RateLimitCycleDownload)
	}
}

// 未关联达量限速方案的用户累加周期流量应为空操作（与 DB 语义一致）。
func TestRuntimeMemoryCycleTrafficNoPlan(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	if err := dm.CreateUser("noplan", "pass", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "noplan")
	if err := dm.AddUserCycleTraffic("noplan", 100, 100); err != nil {
		t.Fatalf("AddUserCycleTraffic: %v", err)
	}
	if up, down, _ := dm.GetUserCycleTraffic(u.ID); up != 0 || down != 0 {
		t.Fatalf("无方案用户周期流量应为 0, 得到 (%d,%d)", up, down)
	}
}

// 管理端把用户加入达量限速方案时，内存周期状态应同步重置。
func TestRuntimeMemoryPlanAssignSyncsCycle(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	plan := &models.RateLimitPlan{Name: "p", PeriodSeconds: 3600, CreatedAt: 1}
	if err := dm.CreateRateLimitPlan(plan, nil); err != nil {
		t.Fatalf("CreateRateLimitPlan: %v", err)
	}
	if err := dm.CreateUser("sync", "pass", "", models.RATE_LIMIT_TYPE_FIXED, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "sync")

	if _, err := dm.AddUsersToRateLimitPlan(plan.ID, []string{"sync"}); err != nil {
		t.Fatalf("AddUsersToRateLimitPlan: %v", err)
	}
	// 内存中应有周期记录（start 非 0）
	if c := dm.runtimeMemory.cycleOf(u.ID); c == nil || c.start == 0 {
		t.Fatalf("加入方案后内存周期应已初始化: %+v", c)
	}
	// 读取用户对象应带上周期字段
	got, _ := dm.GetUserByID(u.ID)
	if got.RateLimitCycleStart == 0 {
		t.Fatalf("读取用户应带内存周期 start")
	}

	// 移除后内存周期应清除
	affected, err := dm.RemoveUsersFromRateLimitPlan(plan.ID, []uint{u.ID})
	if err != nil || affected != 1 {
		t.Fatalf("RemoveUsersFromRateLimitPlan affected=%d err=%v", affected, err)
	}
	if c := dm.runtimeMemory.cycleOf(u.ID); c != nil {
		t.Fatalf("移除方案后内存周期应清除")
	}
}

// 管理端重置用户流量时，内存中的在线会话流量与终身流量都应清零。
func TestRuntimeMemoryResetTrafficSyncsClients(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	if err := dm.CreateUser("bob", "pass", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "bob")
	// 制造内存终身流量
	if err := dm.UpdateUserTraffic("bob", 111, 222); err != nil {
		t.Fatalf("UpdateUserTraffic: %v", err)
	}
	_ = dm.CreateConnectedClientInfoRecord(&models.ConnectedClientInfoRecord{
		ServerID: 1, VirtualIPAddr: "10.0.0.2", Username: "bob", ByteReceived: 100, ByteSent: 200,
	})
	if err := dm.BatchResetUserTraffic([]uint{u.ID}); err != nil {
		t.Fatalf("BatchResetUserTraffic: %v", err)
	}
	rec, err := dm.GetConnectedClientInfoRecord(1, "10.0.0.2")
	if err != nil {
		t.Fatalf("GetConnectedClientInfoRecord: %v", err)
	}
	if rec.ByteReceived != 0 || rec.ByteSent != 0 {
		t.Fatalf("重置后内存会话流量应为 0, 得到 (%d,%d)", rec.ByteReceived, rec.ByteSent)
	}
	// 终身流量也应清零
	got, _ := dm.GetUserByID(u.ID)
	if got.UploadTraffic != 0 || got.DownloadTraffic != 0 {
		t.Fatalf("重置后内存终身流量应为 0, 得到 (%d,%d)", got.UploadTraffic, got.DownloadTraffic)
	}
}

// 内存模式下，UpdateUserTraffic 累加终身流量到内存，不写数据库；读取时叠加。
func TestRuntimeMemoryLifetimeTraffic(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	if err := dm.CreateUser("carol", "pass", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "carol")
	if err := dm.UpdateUserTraffic("carol", 500, 700); err != nil {
		t.Fatalf("UpdateUserTraffic: %v", err)
	}
	if err := dm.UpdateUserTraffic("carol", 100, 200); err != nil {
		t.Fatalf("UpdateUserTraffic 2: %v", err)
	}
	// 读取叠加：500+100=600, 700+200=900
	got, _ := dm.GetUserByID(u.ID)
	if got.UploadTraffic != 600 || got.DownloadTraffic != 900 {
		t.Fatalf("内存终身流量 = (%d,%d), 期望 (600,900)", got.UploadTraffic, got.DownloadTraffic)
	}
	// 数据库保持 0
	var dbUser models.User
	dm.DB.First(&dbUser, u.ID)
	if dbUser.UploadTraffic != 0 || dbUser.DownloadTraffic != 0 {
		t.Fatalf("数据库终身流量被写入: (%d,%d)", dbUser.UploadTraffic, dbUser.DownloadTraffic)
	}
}

// 未启用内存模式时，在线会话/ACL/周期仍走数据库（行为不变）。
func TestRuntimeMemoryDisabledUsesDB(t *testing.T) {
	dm := newTestDaoManager(t) // MaxMemoryEvents = 0
	if dm.useMemoryRuntime() {
		t.Fatalf("MaxMemoryEvents=0 时不应启用内存运行时数据")
	}
	if err := dm.CreateConnectedClientInfoRecord(&models.ConnectedClientInfoRecord{ServerID: 1, VirtualIPAddr: "10.0.0.9", Username: "x"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var n int64
	dm.DB.Model(&models.ConnectedClientInfoRecord{}).Count(&n)
	if n != 1 {
		t.Fatalf("DB 模式在线记录数 = %d, 期望 1", n)
	}
	if err := dm.SaveAddedACL([]*models.AddedServerACLRecord{{ServerID: 1, VirtualIPAddr: "10.0.0.9", ACLType: 1, ACLValue: "v"}}); err != nil {
		t.Fatalf("SaveAddedACL: %v", err)
	}
	dm.DB.Model(&models.AddedServerACLRecord{}).Count(&n)
	if n != 1 {
		t.Fatalf("DB 模式 ACL 记录数 = %d, 期望 1", n)
	}
}
