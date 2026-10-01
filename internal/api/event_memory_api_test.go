package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/models"
)

// newMemoryEventTestApp 构造启用内存事件的面板 App（max_memory_events = max）。
func newMemoryEventTestApp(t *testing.T, max int) *App {
	t.Helper()
	cfg := &config.Config{
		SQLiteDB:          filepath.Join(t.TempDir(), "test.db"),
		PasswordSalt:      "testsalt",
		WorkingDir:        t.TempDir(),
		InternalAPIListen: "127.0.0.1:0",
		MaxMemoryEvents:   max,
	}
	app := NewApp(cfg, "test")
	if err := models.MigrateDB(app.daoManager.DB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return app
}

// 内存事件启用时，事件经 App 记录后应能从事件列表接口读到，且不落库；
// 仪表盘概览的事件数也应来自内存。
func TestMemoryEventsAPIIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newMemoryEventTestApp(t, 50)

	// 通过 DAO 记录事件（等价于各调用点 a.daoManager.CreateEvent）。
	if err := app.daoManager.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_START_SUCCESS, "10.0.0.1", "启动成功"); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if err := app.daoManager.CreateEvent(1, models.SERVER_EVENT_TYPE_SERVER_EXIT_SUCCESS, "10.0.0.2", "正常停止"); err != nil {
		t.Fatalf("CreateEvent 2: %v", err)
	}

	// 数据库中不应出现事件记录（保护闪存）。
	var dbCount int64
	app.daoManager.DB.Model(&models.ServerEvent{}).Count(&dbCount)
	if dbCount != 0 {
		t.Fatalf("内存模式数据库事件数 = %d, 期望 0", dbCount)
	}

	// 事件列表接口应返回两条（内存）。
	engine := gin.New()
	engine.GET("/event/list", func(c *gin.Context) { app.GetEventList(c, nil) })
	req := httptest.NewRequest("GET", "/event/list?id=1", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("event/list 状态码 = %d, body=%s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Result string `json:"result"`
		Total  int64  `json:"total"`
		Data   []struct {
			ID        uint   `json:"id"`
			EventData string `json:"event_data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("解析 event/list: %v", err)
	}
	if listResp.Result != "success" || listResp.Total != 2 || len(listResp.Data) != 2 {
		t.Fatalf("event/list total=%d len=%d, 期望 2/2", listResp.Total, len(listResp.Data))
	}
	if listResp.Data[0].ID == 0 {
		t.Fatalf("内存事件应带自增 ID")
	}

	// 仪表盘概览事件数应为 2。
	dashEngine := gin.New()
	dashEngine.GET("/dashboard/summary", func(c *gin.Context) { app.GetDashboardSummaryHandler(c, nil) })
	dreq := httptest.NewRequest("GET", "/dashboard/summary", nil)
	drec := httptest.NewRecorder()
	dashEngine.ServeHTTP(drec, dreq)
	if drec.Code != 200 {
		t.Fatalf("dashboard/summary 状态码 = %d, body=%s", drec.Code, drec.Body.String())
	}
	var dashResp struct {
		Result string `json:"result"`
		Data   struct {
			Summary struct {
				EventTotal int64 `json:"event_total"`
			} `json:"summary"`
			RecentEvents []struct {
				EventData string `json:"event_data"`
			} `json:"recent_events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(drec.Body.Bytes(), &dashResp); err != nil {
		t.Fatalf("解析 dashboard: %v", err)
	}
	if dashResp.Data.Summary.EventTotal != 2 {
		t.Fatalf("仪表盘 event_total = %d, 期望 2", dashResp.Data.Summary.EventTotal)
	}
	if len(dashResp.Data.RecentEvents) != 2 {
		t.Fatalf("仪表盘最近事件数 = %d, 期望 2", len(dashResp.Data.RecentEvents))
	}
}

// 内存运行时模式下，内部上下线接口对会话/事件表均不写库；终身流量仍写库（设计如此）。
func TestRuntimeMemoryInternalOnlineOfflineNoDBWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newMemoryEventTestApp(t, 100)

	if err := app.daoManager.CreateUser("alice", "pass", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	engine := gin.New()
	engine.POST("/user/online", app.UserOnlineInternalHandler)
	engine.POST("/user/offline", app.UserOfflineInternalHandler)

	encodedUser := base64.StdEncoding.EncodeToString([]byte("alice"))
	onlineBody := "username: " + encodedUser + "\n" +
		"server_id: 7\n" +
		"real_ip_addr: 1.2.3.4:5555\n" +
		"virtual_ip_addr: 10.8.0.2\n" +
		"virtual_ip6_addr: fd00::2\n" +
		"client_cert_name: cert-alice"

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest("POST", "/user/online", strings.NewReader(onlineBody)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "success") {
		t.Fatalf("online 失败: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 在线会话可见（内存），数据库无记录。
	if recs, _ := app.daoManager.ListConnectedClientInfoRecord(); len(recs) != 1 {
		t.Fatalf("内存在线会话 len=%d, 期望 1", len(recs))
	}
	var connCount int64
	app.daoManager.DB.Model(&models.ConnectedClientInfoRecord{}).Count(&connCount)
	if connCount != 0 {
		t.Fatalf("数据库在线会话数 = %d, 期望 0", connCount)
	}

	// 下线（带流量）。
	offlineBody := onlineBody + "\nbytes_send: 1000\nbytes_received: 2000"
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest("POST", "/user/offline", strings.NewReader(offlineBody)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "success") {
		t.Fatalf("offline 失败: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 会话/事件表仍为空；终身流量改为保存在内存（数据库不写）。
	app.daoManager.DB.Model(&models.ConnectedClientInfoRecord{}).Count(&connCount)
	if connCount != 0 {
		t.Fatalf("下线后数据库在线会话数 = %d, 期望 0", connCount)
	}
	var eventCount int64
	app.daoManager.DB.Model(&models.ServerEvent{}).Count(&eventCount)
	if eventCount != 0 {
		t.Fatalf("数据库服务器事件数 = %d, 期望 0", eventCount)
	}
	// 数据库中的终身流量应保持 0（未写库）。
	var u models.User
	if err := app.daoManager.DB.Where("username = ?", "alice").First(&u).Error; err != nil {
		t.Fatalf("读取用户: %v", err)
	}
	if u.UploadTraffic != 0 || u.DownloadTraffic != 0 {
		t.Fatalf("数据库终身流量 = (%d,%d), 期望 (0,0)（内存模式不应写库）", u.UploadTraffic, u.DownloadTraffic)
	}
	// 通过 DAO 读取（会叠加内存态）应看到累加后的终身流量。
	user, err := app.daoManager.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user.UploadTraffic != 1000 || user.DownloadTraffic != 2000 {
		t.Fatalf("内存终身流量 = (%d,%d), 期望 (1000,2000)", user.UploadTraffic, user.DownloadTraffic)
	}
}
