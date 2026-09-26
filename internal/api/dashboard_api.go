package api

import (
	"time"

	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"

	"github.com/gin-gonic/gin"
)

// 仪表盘概览响应。
type dashboardSummaryResponse struct {
	// 服务器
	ServerTotal   int64 `json:"server_total"`
	ServerRunning int64 `json:"server_running"`
	// 在线客户端
	OnlineClients int64 `json:"online_clients"`
	// 用户与用户组
	UserTotal  int64 `json:"user_total"`
	GroupTotal int64 `json:"group_total"`
	// 证书
	CertCATotal     int64 `json:"cert_ca_total"`
	CertServerTotal int64 `json:"cert_server_total"`
	CertClientTotal int64 `json:"cert_client_total"`
	// 达量限速
	RateLimitPlanTotal int64 `json:"ratelimit_plan_total"`
	RateLimitUserTotal int64 `json:"ratelimit_user_total"`
	// 事件
	EventTotal int64 `json:"event_total"`
	// 资源集
	ActiveResourceSet     string `json:"active_resource_set"`
	ActiveResourceSetName string `json:"active_resource_set_name"`
	// 版本
	BackendBuildDate string `json:"backend_build_date"`
	// 时间
	ServerTime uint64 `json:"server_time"`
}

type dashboardServerItem struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Local       string `json:"local"`
	Port        uint32 `json:"port"`
	Proto       string `json:"proto"`
	Dev         string `json:"dev"`
	ServerCIDR  string `json:"server_cidr"`
	ServerCIDR6 string `json:"server_cidr6"`
	AutoStart   bool   `json:"auto_start"`
	Running     bool   `json:"running"`
}

type dashboardEventItem struct {
	ID         uint   `json:"id"`
	ServerID   uint   `json:"server_id"`
	ServerName string `json:"server_name"`
	EventType  int    `json:"event_type"`
	EventTime  uint64 `json:"event_time"`
	RealIPAddr string `json:"real_ip_addr"`
	EventData  string `json:"event_data"`
}

// GetDashboardSummaryHandler 返回首页概览所需的简要信息。
func (a *App) GetDashboardSummaryHandler(c *gin.Context, user *models.User) {
	// 服务器：总数与运行数
	servers, err := a.daoManager.ListOpenVPNServer()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "获取概览失败: " + err.Error()})
		return
	}
	serverList := make([]dashboardServerItem, 0, len(servers))
	serverNameMap := make(map[uint]string, len(servers))
	runningCount := int64(0)
	for _, s := range servers {
		running := false
		a.lock.RLock()
		if inst, ok := a.ovpnProcessList[s.ID]; ok {
			pl := a.getProcessLock(s.ID)
			pl.RLock()
			running = inst.Running()
			pl.RUnlock()
		}
		a.lock.RUnlock()
		if running {
			runningCount++
		}
		serverNameMap[s.ID] = s.Name
		serverList = append(serverList, dashboardServerItem{
			ID:          s.ID,
			Name:        s.Name,
			Local:       s.Local,
			Port:        s.Port,
			Proto:       s.Proto,
			Dev:         s.Dev,
			ServerCIDR:  s.ServerCIDR,
			ServerCIDR6: s.ServerCIDR6,
			AutoStart:   s.AutoStart,
			Running:     running,
		})
	}

	summary := dashboardSummaryResponse{
		ServerTotal:      int64(len(servers)),
		ServerRunning:    runningCount,
		BackendBuildDate: a.buildDate,
		ServerTime:       uint64(time.Now().Unix()),
	}

	// 在线客户端
	if n, err := a.daoManager.CountConnectedClientInfoRecords(); err == nil {
		summary.OnlineClients = n
	}
	// 用户 / 用户组
	if n, err := a.daoManager.GetUserCount("", 0, 0); err == nil {
		summary.UserTotal = n
	}
	if n, err := a.daoManager.GetGroupCount(); err == nil {
		summary.GroupTotal = n
	}
	// 证书
	if n, err := a.daoManager.CountCertificatesByType(models.CERT_TYPE_CA); err == nil {
		summary.CertCATotal = n
	}
	if n, err := a.daoManager.CountCertificatesByType(models.CERT_TYPE_SERVER); err == nil {
		summary.CertServerTotal = n
	}
	if n, err := a.daoManager.CountCertificatesByType(models.CERT_TYPE_CLIENT); err == nil {
		summary.CertClientTotal = n
	}
	// 达量限速
	if plans, err := a.daoManager.ListRateLimitPlans(); err == nil {
		summary.RateLimitPlanTotal = int64(len(plans))
	}
	if counts, err := a.daoManager.CountUsersForRateLimitPlans(); err == nil {
		var total int64
		for _, n := range counts {
			total += n
		}
		summary.RateLimitUserTotal = total
	}
	// 事件
	if n, err := a.daoManager.CountServerEvents(); err == nil {
		summary.EventTotal = n
	}
	// 当前资源集
	activeSet := a.GetActiveResourceSetID()
	summary.ActiveResourceSet = activeSet
	for _, s := range ovpnserver.ListResourceSets() {
		if s.ID == activeSet {
			summary.ActiveResourceSetName = s.Name
			break
		}
	}

	// 最近事件
	recentEvents := make([]dashboardEventItem, 0, 10)
	if events, err := a.daoManager.ListRecentServerEvents(10); err == nil {
		for _, e := range events {
			recentEvents = append(recentEvents, dashboardEventItem{
				ID:         e.ID,
				ServerID:   e.ServerID,
				ServerName: serverNameMap[e.ServerID],
				EventType:  e.EventType,
				EventTime:  e.EventTime,
				RealIPAddr: e.RealIPAddr,
				EventData:  e.EventData,
			})
		}
	}

	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data": gin.H{
			"summary":       summary,
			"servers":       serverList,
			"recent_events": recentEvents,
		},
	})
}
