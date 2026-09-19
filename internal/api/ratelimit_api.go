package api

import (
	"fmt"
	"strconv"
	"time"

	"openvpn-pannel/internal/models"

	"github.com/gin-gonic/gin"
)

type rateLimitRuleParam struct {
	Priority               int    `json:"priority" binding:"min=0"`
	UploadThresholdBytes   uint64 `json:"upload_threshold_bytes"`
	DownloadThresholdBytes uint64 `json:"download_threshold_bytes"`
	LimitUploadKB          uint64 `json:"limit_upload_kb"`
	LimitDownloadKB        uint64 `json:"limit_download_kb"`
	AllowConnect           bool   `json:"allow_connect"`
}

type rateLimitPlanParam struct {
	ID            uint                 `json:"id"`
	Name          string               `json:"name" binding:"required,min=1,max=100"`
	Description   string               `json:"description" binding:"max=16384"`
	PeriodSeconds uint64               `json:"period_seconds" binding:"required,min=1"`
	Rules         []rateLimitRuleParam `json:"rules" binding:"required,min=1,max=100,dive"`
}

func (p rateLimitPlanParam) toModels() (*models.RateLimitPlan, []*models.RateLimitRule) {
	plan := &models.RateLimitPlan{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		PeriodSeconds: p.PeriodSeconds,
	}
	rules := make([]*models.RateLimitRule, 0, len(p.Rules))
	for _, r := range p.Rules {
		rules = append(rules, &models.RateLimitRule{
			Priority:               r.Priority,
			UploadThresholdBytes:   r.UploadThresholdBytes,
			DownloadThresholdBytes: r.DownloadThresholdBytes,
			LimitUploadKB:          r.LimitUploadKB,
			LimitDownloadKB:        r.LimitDownloadKB,
			AllowConnect:           r.AllowConnect,
		})
	}
	return plan, rules
}

type rateLimitPlanResponse struct {
	ID            uint                    `json:"id"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	PeriodSeconds uint64                  `json:"period_seconds"`
	CreatedAt     int64                   `json:"created_at"`
	UserCount     int64                   `json:"user_count"`
	Rules         []*models.RateLimitRule `json:"rules"`
}

// ListRateLimitPlanHandler 列出全部达量限速方案（含规则与关联用户数）。
func (a *App) ListRateLimitPlanHandler(c *gin.Context, user *models.User) {
	plans, err := a.daoManager.ListRateLimitPlans()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	ruleMap, err := a.daoManager.ListRateLimitRulesForPlans()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	countMap, err := a.daoManager.CountUsersForRateLimitPlans()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	data := make([]*rateLimitPlanResponse, 0, len(plans))
	for _, plan := range plans {
		data = append(data, &rateLimitPlanResponse{
			ID:            plan.ID,
			Name:          plan.Name,
			Description:   plan.Description,
			PeriodSeconds: plan.PeriodSeconds,
			CreatedAt:     plan.CreatedAt,
			UserCount:     countMap[plan.ID],
			Rules:         ruleMap[plan.ID],
		})
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": data})
}

// CreateRateLimitPlanHandler 创建达量限速方案。
func (a *App) CreateRateLimitPlanHandler(c *gin.Context, user *models.User) {
	var param rateLimitPlanParam
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	plan, rules := param.toModels()
	plan.CreatedAt = time.Now().Unix()
	if err := a.daoManager.CreateRateLimitPlan(plan, rules); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "id": plan.ID})
}

// UpdateRateLimitPlanHandler 更新达量限速方案（规则整体替换）。
func (a *App) UpdateRateLimitPlanHandler(c *gin.Context, user *models.User) {
	var param rateLimitPlanParam
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if param.ID == 0 {
		c.JSON(400, gin.H{"result": "failed", "error": "id 不能为空"})
		return
	}
	plan, rules := param.toModels()
	if err := a.daoManager.UpdateRateLimitPlan(plan, rules); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil})
}

// DeleteRateLimitPlanHandler 删除达量限速方案。
func (a *App) DeleteRateLimitPlanHandler(c *gin.Context, user *models.User) {
	var param struct {
		ID uint `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if err := a.daoManager.DeleteRateLimitPlan(param.ID); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil})
}

// ListRateLimitPlanUsersHandler 列出方案关联的用户。
func (a *App) ListRateLimitPlanUsersHandler(c *gin.Context, user *models.User) {
	planID, err := strconv.ParseUint(c.DefaultQuery("id", "0"), 10, 64)
	if err != nil || planID == 0 {
		c.JSON(400, gin.H{"result": "failed", "error": "id 参数错误"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := c.DefaultQuery("query", "")
	users, count, err := a.daoManager.ListUsersForRateLimitPlan(uint(planID), page, pageSize, query)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	type Item struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
	}
	data := make([]*Item, 0, len(users))
	for _, u := range users {
		data = append(data, &Item{ID: u.ID, Username: u.Username})
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": data, "count": count})
}

// AddUsersToRateLimitPlanHandler 批量把用户关联到方案。
func (a *App) AddUsersToRateLimitPlanHandler(c *gin.Context, user *models.User) {
	var param struct {
		PlanID    uint     `json:"plan_id" binding:"required"`
		Usernames []string `json:"usernames" binding:"required,min=1,max=1000"`
	}
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if _, _, err := a.daoManager.GetRateLimitPlanWithRules(param.PlanID); err != nil {
		c.JSON(404, gin.H{"result": "failed", "error": "限速方案不存在"})
		return
	}
	names := make([]string, 0, len(param.Usernames))
	seen := make(map[string]bool)
	for _, name := range param.Usernames {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	affected, err := a.daoManager.AddUsersToRateLimitPlan(param.PlanID, names)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "affected": affected})
}

// RemoveUsersFromRateLimitPlanHandler 从方案移除用户。
func (a *App) RemoveUsersFromRateLimitPlanHandler(c *gin.Context, user *models.User) {
	var param struct {
		PlanID  uint   `json:"plan_id" binding:"required"`
		UserIDs []uint `json:"user_ids" binding:"required,min=1,max=1000"`
	}
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	affected, err := a.daoManager.RemoveUsersFromRateLimitPlan(param.PlanID, param.UserIDs)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "affected": affected})
}

type rateLimitUserStatusItem struct {
	UserID          uint   `json:"user_id"`
	Username        string `json:"username"`
	PlanID          uint   `json:"plan_id"`
	PlanName        string `json:"plan_name"`
	HasRule         bool   `json:"has_rule"`
	RulePriority    int    `json:"rule_priority"`
	AllowConnect    bool   `json:"allow_connect"`
	LimitUploadKB   uint64 `json:"limit_upload_kb"`
	LimitDownloadKB uint64 `json:"limit_download_kb"`
	CycleUpload     uint64 `json:"cycle_upload"`
	CycleDownload   uint64 `json:"cycle_download"`
	ResetRemaining  uint64 `json:"reset_remaining_sec"`
}

// ListRateLimitUserStatusHandler 列出已关联达量限速方案的用户及其当前匹配到的规则与限制。
func (a *App) ListRateLimitUserStatusHandler(c *gin.Context, user *models.User) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := c.DefaultQuery("query", "")
	users, count, err := a.daoManager.ListUsersWithRateLimitPlan(page, pageSize, query)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	planNameMap, err := a.daoManager.RateLimitPlanNameMap()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	planRulesMap, err := a.daoManager.ListRateLimitRulesForPlans()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}

	data := make([]*rateLimitUserStatusItem, 0, len(users))
	for _, u := range users {
		item := &rateLimitUserStatusItem{
			UserID:       u.ID,
			Username:     u.Username,
			PlanID:       u.RateLimitPlanID,
			PlanName:     planNameMap[u.RateLimitPlanID],
			AllowConnect: true,
		}
		if cycleUp, cycleDown, err := a.daoManager.GetUserCycleTraffic(u.ID); err == nil {
			item.CycleUpload = cycleUp
			item.CycleDownload = cycleDown
			if rules := planRulesMap[u.RateLimitPlanID]; len(rules) > 0 {
				rule := models.MatchRateLimitRule(rules, cycleUp, cycleDown)
				if rule != nil {
					item.HasRule = true
					item.RulePriority = rule.Priority
					item.AllowConnect = rule.AllowConnect
					item.LimitUploadKB = rule.LimitUploadKB
					item.LimitDownloadKB = rule.LimitDownloadKB
				}
			}
		}
		if remaining, err := a.daoManager.GetRateLimitResetRemaining(u.ID); err == nil {
			item.ResetRemaining = remaining
		}
		data = append(data, item)
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": data, "count": count})
}

// ResetRateLimitUserCycleHandler 立即重置某用户当前达量限速周期的流量统计。
func (a *App) ResetRateLimitUserCycleHandler(c *gin.Context, user *models.User) {
	var param struct {
		UserID uint `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if _, err := a.daoManager.GetUserByID(param.UserID); err != nil {
		c.JSON(404, gin.H{"result": "failed", "error": "用户不存在"})
		return
	}
	if err := a.daoManager.ResetUserRateLimitCycle(param.UserID); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil})
}

type onlineUserItem struct {
	Username          string `json:"username"`
	ServerID          uint   `json:"server_id"`
	ServerName        string `json:"server_name"`
	VirtualIPAddr     string `json:"virtual_ip_addr"`
	SessionUpload     uint64 `json:"session_upload"`
	SessionDownload   uint64 `json:"session_download"`
	CycleUpload       uint64 `json:"cycle_upload"`
	CycleDownload     uint64 `json:"cycle_download"`
	PlanID            uint   `json:"plan_id"`
	PlanName          string `json:"plan_name"`
	LimitUploadKB     uint64 `json:"limit_upload_kb"`
	LimitDownloadKB   uint64 `json:"limit_download_kb"`
	AllowConnect      bool   `json:"allow_connect"`
	ResetRemainingSec uint64 `json:"reset_remaining_sec"`
}

// ListOnlineUserHandler 列出当前在线用户及其会话/周期流量、限速方案与生效限速。
func (a *App) ListOnlineUserHandler(c *gin.Context, user *models.User) {
	records, err := a.daoManager.ListConnectedClientInfoRecord()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	serverList, err := a.daoManager.ListOpenVPNServer()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	serverNameMap := make(map[uint]string, len(serverList))
	for _, server := range serverList {
		serverNameMap[server.ID] = server.Name
	}
	planNameMap, err := a.daoManager.RateLimitPlanNameMap()
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	planPeriodMap := make(map[uint]uint64)
	if plans, err := a.daoManager.ListRateLimitPlans(); err == nil {
		for _, plan := range plans {
			planPeriodMap[plan.ID] = plan.PeriodSeconds
		}
	}

	// 汇总每个用户在线会话的流量与周期起点
	userByName := make(map[string]*models.User)
	sessionByUser := make(map[string][2]uint64)
	for _, record := range records {
		if record.Username == "" {
			continue
		}
		if _, ok := userByName[record.Username]; !ok {
			u, err := a.daoManager.GetUserByUsername(record.Username)
			if err != nil {
				continue
			}
			userByName[record.Username] = u
		}
		data := sessionByUser[record.Username]
		data[0] += record.ByteSent
		data[1] += record.ByteReceived
		sessionByUser[record.Username] = data
	}

	now := uint64(time.Now().Unix())
	data := make([]*onlineUserItem, 0, len(records))
	for _, record := range records {
		if record.Username == "" {
			continue
		}
		u, ok := userByName[record.Username]
		if !ok {
			continue
		}
		uploadKB, downloadKB, allowConnect, err := a.daoManager.ResolveUserRateLimitAndConnect(u.ID, record.ServerID)
		if err != nil {
			uploadKB, downloadKB, allowConnect = 0, 0, true
		}
		cycleUpload := u.RateLimitCycleUpload + sessionByUser[record.Username][0]
		cycleDownload := u.RateLimitCycleDownload + sessionByUser[record.Username][1]
		var remaining uint64
		if u.RateLimitPlanID != 0 && u.RateLimitCycleStart != 0 {
			period := planPeriodMap[u.RateLimitPlanID]
			if period > 0 {
				elapsed := now - u.RateLimitCycleStart
				if elapsed < period {
					remaining = period - elapsed
				}
			}
		}
		data = append(data, &onlineUserItem{
			Username:          record.Username,
			ServerID:          record.ServerID,
			ServerName:        serverNameMap[record.ServerID],
			VirtualIPAddr:     record.VirtualIPAddr,
			SessionUpload:     record.ByteSent,
			SessionDownload:   record.ByteReceived,
			CycleUpload:       cycleUpload,
			CycleDownload:     cycleDownload,
			PlanID:            u.RateLimitPlanID,
			PlanName:          planNameMap[u.RateLimitPlanID],
			LimitUploadKB:     uploadKB,
			LimitDownloadKB:   downloadKB,
			AllowConnect:      allowConnect,
			ResetRemainingSec: remaining,
		})
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": data})
}

// formatRetryAfter 把剩余秒数格式化为“X 天 X 小时 X 分钟”。
func formatRetryAfter(seconds uint64) string {
	days := seconds / 86400
	seconds %= 86400
	hours := seconds / 3600
	seconds %= 3600
	minutes := seconds / 60
	parts := ""
	if days > 0 {
		parts += fmt.Sprintf("%d天", days)
	}
	if hours > 0 {
		parts += fmt.Sprintf("%d小时", hours)
	}
	if minutes > 0 {
		parts += fmt.Sprintf("%d分钟", minutes)
	}
	if parts == "" {
		parts = "1分钟"
	}
	return parts
}
