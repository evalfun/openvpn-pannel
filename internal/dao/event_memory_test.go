package dao

import (
	"path/filepath"
	"strconv"
	"testing"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/models"
)

// newMemoryEventDaoManager 构造启用内存事件的 DaoManager（max_memory_events = max）。
func newMemoryEventDaoManager(t *testing.T, max int) *DaoManager {
	t.Helper()
	cfg := &config.Config{
		SQLiteDB:        filepath.Join(t.TempDir(), "test.db"),
		PasswordSalt:    "test-salt",
		MaxMemoryEvents: max,
	}
	dm, err := NewDaoManager(cfg)
	if err != nil {
		t.Fatalf("NewDaoManager: %v", err)
	}
	if err := models.MigrateDB(dm.DB); err != nil {
		t.Fatalf("MigrateDB: %v", err)
	}
	return dm
}

// 内存事件启用时，事件不应写入数据库，只进内存存储。
func TestMemoryEventsNotPersistedToDB(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 10)
	if !dm.useMemoryEvents() {
		t.Fatalf("MaxMemoryEvents>0 时应启用内存事件")
	}

	if err := dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "1.2.3.4", "启动成功"); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if err := dm.CreateCertificateEvent(&models.CertificateEvent{
		EventType:  models.CERT_EVENT_TYPE_CREATE,
		EventTime:  123,
		RealIPAddr: "1.2.3.4",
		CertName:   "ca",
	}); err != nil {
		t.Fatalf("CreateCertificateEvent: %v", err)
	}

	// 直接查库应为空。
	var serverCount, certCount int64
	dm.DB.Model(&models.ServerEvent{}).Count(&serverCount)
	dm.DB.Model(&models.CertificateEvent{}).Count(&certCount)
	if serverCount != 0 || certCount != 0 {
		t.Fatalf("内存模式下数据库不应有事件: server=%d cert=%d", serverCount, certCount)
	}

	// 但通过 DAO 查询应能看到。
	resp, err := dm.GetEventList(1, nil, "", 0, 0, 1, 20)
	if err != nil {
		t.Fatalf("GetEventList: %v", err)
	}
	if resp.Total != 1 || len(resp.EventList) != 1 {
		t.Fatalf("内存事件 total=%d len=%d, 期望 1/1", resp.Total, len(resp.EventList))
	}
	if resp.EventList[0].ID == 0 {
		t.Fatalf("内存事件应分配自增 ID")
	}

	certResp, err := dm.GetCertificateEventList(nil, "", 0, 0, 1, 20)
	if err != nil {
		t.Fatalf("GetCertificateEventList: %v", err)
	}
	if certResp.Total != 1 || len(certResp.EventList) != 1 {
		t.Fatalf("证书内存事件 total=%d len=%d, 期望 1/1", certResp.Total, len(certResp.EventList))
	}
}

// 超过上限时应丢弃最旧的事件，只保留最新的 max 条。
func TestMemoryEventsCapDropsOldest(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 3)
	for i := 0; i < 5; i++ {
		if err := dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "ip", uintToStr(i)); err != nil {
			t.Fatalf("CreateEvent %d: %v", i, err)
		}
	}
	if got := dm.eventMemory.countServerEvents(); got != 3 {
		t.Fatalf("内存事件数 = %d, 期望 3", got)
	}

	resp, _ := dm.GetEventList(1, nil, "", 0, 0, 1, 20)
	if resp.Total != 3 {
		t.Fatalf("total = %d, 期望 3", resp.Total)
	}
	// 最新在前：应为 4、3、2。
	want := []string{"4", "3", "2"}
	for i, e := range resp.EventList {
		if e.EventData != want[i] {
			t.Fatalf("第 %d 条 = %q, 期望 %q", i, e.EventData, want[i])
		}
	}

	// 证书事件同样受上限约束。
	for i := 0; i < 5; i++ {
		_ = dm.CreateCertificateEvent(&models.CertificateEvent{
			EventType: models.CERT_EVENT_TYPE_SIGN,
			EventTime: uint64(i),
			EventData: uintToStr(i),
		})
	}
	certResp, _ := dm.GetCertificateEventList(nil, "", 0, 0, 1, 20)
	if certResp.Total != 3 {
		t.Fatalf("证书事件 total = %d, 期望 3", certResp.Total)
	}
	if certResp.EventList[0].EventData != "4" {
		t.Fatalf("最新证书事件 = %q, 期望 4", certResp.EventList[0].EventData)
	}
}

// 内存事件查询应支持按服务器、类型、关键字、时间范围过滤与分页，语义与数据库一致。
func TestMemoryEventsFilterAndPagination(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	// server 1 三条，server 2 两条。
	_ = dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "10.0.0.1", "alpha")
	_ = dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_EXIT_SUCCESS, "10.0.0.2", "beta")
	_ = dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "10.0.0.3", "gamma")
	_ = dm.CreateEvent(2, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "10.0.0.4", "delta")
	_ = dm.CreateEvent(2, models.SERVER_EVENT_TYPE_SERVER_EXIT_FAIL, "10.0.0.5", "epsilon")

	// 按服务器隔离。
	resp, _ := dm.GetEventList(1, nil, "", 0, 0, 1, 20)
	if resp.Total != 3 {
		t.Fatalf("server1 total = %d, 期望 3", resp.Total)
	}

	// 按类型过滤。
	resp, _ = dm.GetEventList(1, []int{models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS}, "", 0, 0, 1, 20)
	if resp.Total != 2 {
		t.Fatalf("server1 start_success total = %d, 期望 2", resp.Total)
	}

	// 关键字匹配 EventData。
	resp, _ = dm.GetEventList(1, nil, "gamma", 0, 0, 1, 20)
	if resp.Total != 1 || resp.EventList[0].EventData != "gamma" {
		t.Fatalf("关键字 gamma 匹配失败: total=%d", resp.Total)
	}
	// 关键字匹配 RealIPAddr。
	resp, _ = dm.GetEventList(1, nil, "10.0.0.2", 0, 0, 1, 20)
	if resp.Total != 1 || resp.EventList[0].EventData != "beta" {
		t.Fatalf("关键字 IP 匹配失败: total=%d", resp.Total)
	}

	// 分页：page_size=2。
	p1, _ := dm.GetEventList(1, nil, "", 0, 0, 1, 2)
	p2, _ := dm.GetEventList(1, nil, "", 0, 0, 2, 2)
	if p1.Total != 3 || len(p1.EventList) != 2 {
		t.Fatalf("第 1 页 total=%d len=%d, 期望 3/2", p1.Total, len(p1.EventList))
	}
	if len(p2.EventList) != 1 {
		t.Fatalf("第 2 页 len=%d, 期望 1", len(p2.EventList))
	}
	// 倒序：第 1 页应为 gamma、beta。
	if p1.EventList[0].EventData != "gamma" || p1.EventList[1].EventData != "beta" {
		t.Fatalf("第 1 页顺序异常: %q, %q", p1.EventList[0].EventData, p1.EventList[1].EventData)
	}
	if p2.EventList[0].EventData != "alpha" {
		t.Fatalf("第 2 页 = %q, 期望 alpha", p2.EventList[0].EventData)
	}
}

// 清空应只影响目标服务器（服务器事件）或全部（证书事件）。
func TestMemoryEventsClear(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	_ = dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "ip", "s1")
	_ = dm.CreateEvent(2, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "ip", "s2")
	_ = dm.CreateCertificateEvent(&models.CertificateEvent{EventType: models.CERT_EVENT_TYPE_CREATE})

	if err := dm.ClearEvent(1); err != nil {
		t.Fatalf("ClearEvent: %v", err)
	}
	if r, _ := dm.GetEventList(1, nil, "", 0, 0, 1, 20); r.Total != 0 {
		t.Fatalf("清空 server1 后 total = %d, 期望 0", r.Total)
	}
	if r, _ := dm.GetEventList(2, nil, "", 0, 0, 1, 20); r.Total != 1 {
		t.Fatalf("清空 server1 不应影响 server2: total = %d", r.Total)
	}

	if err := dm.ClearCertificateEvent(); err != nil {
		t.Fatalf("ClearCertificateEvent: %v", err)
	}
	if r, _ := dm.GetCertificateEventList(nil, "", 0, 0, 1, 20); r.Total != 0 {
		t.Fatalf("清空证书事件后 total = %d, 期望 0", r.Total)
	}
}

// 仪表盘使用的统计与最近事件接口也应走内存。
func TestMemoryEventsDashboardCounts(t *testing.T) {
	dm := newMemoryEventDaoManager(t, 100)
	for i := 0; i < 4; i++ {
		_ = dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "ip", uintToStr(i))
	}
	n, err := dm.CountServerEvents()
	if err != nil {
		t.Fatalf("CountServerEvents: %v", err)
	}
	if n != 4 {
		t.Fatalf("CountServerEvents = %d, 期望 4", n)
	}

	recent, err := dm.ListRecentServerEvents(2)
	if err != nil {
		t.Fatalf("ListRecentServerEvents: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("ListRecentServerEvents len = %d, 期望 2", len(recent))
	}
	if recent[0].EventData != "3" || recent[1].EventData != "2" {
		t.Fatalf("最近事件顺序异常: %q, %q", recent[0].EventData, recent[1].EventData)
	}
}

// 未启用内存事件（max=0）时行为不变：事件写入数据库。
func TestMemoryEventsDisabledUsesDB(t *testing.T) {
	dm := newTestDaoManager(t) // MaxMemoryEvents 默认 0
	if dm.useMemoryEvents() {
		t.Fatalf("MaxMemoryEvents=0 时不应启用内存事件")
	}
	if err := dm.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "ip", "db"); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	var count int64
	dm.DB.Model(&models.ServerEvent{}).Count(&count)
	if count != 1 {
		t.Fatalf("数据库事件数 = %d, 期望 1", count)
	}
	resp, _ := dm.GetEventList(1, nil, "", 0, 0, 1, 20)
	if resp.Total != 1 || resp.EventList[0].EventData != "db" {
		t.Fatalf("DB 模式查询异常: total=%d", resp.Total)
	}
}

func uintToStr(i int) string {
	return strconv.Itoa(i)
}
