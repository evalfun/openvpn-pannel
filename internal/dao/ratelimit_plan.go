package dao

import (
	"sort"
	"time"

	"openvpn-pannel/internal/models"

	"gorm.io/gorm"
)

// sortRateLimitRules 按优先级从大到小排序（匹配按此顺序进行）。
// 优先级相同再按流量阈值之和升序，最后按 id，保证顺序稳定。
func sortRateLimitRules(rules []*models.RateLimitRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		si := rules[i].UploadThresholdBytes + rules[i].DownloadThresholdBytes
		sj := rules[j].UploadThresholdBytes + rules[j].DownloadThresholdBytes
		if si != sj {
			return si < sj
		}
		return rules[i].ID < rules[j].ID
	})
}

// CreateRateLimitPlan 创建达量限速方案及其规则。
func (um *DaoManager) CreateRateLimitPlan(plan *models.RateLimitPlan, rules []*models.RateLimitRule) error {
	return um.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(plan).Error; err != nil {
			return err
		}
		sortRateLimitRules(rules)
		for _, rule := range rules {
			rule.ID = 0
			rule.PlanID = plan.ID
			if err := tx.Create(rule).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateRateLimitPlan 更新方案及其规则（规则整体替换）。
func (um *DaoManager) UpdateRateLimitPlan(plan *models.RateLimitPlan, rules []*models.RateLimitRule) error {
	return um.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.RateLimitPlan{}).Where("id = ?", plan.ID).Updates(map[string]interface{}{
			"name":           plan.Name,
			"description":    plan.Description,
			"period_seconds": plan.PeriodSeconds,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("plan_id = ?", plan.ID).Delete(&models.RateLimitRule{}).Error; err != nil {
			return err
		}
		sortRateLimitRules(rules)
		for _, rule := range rules {
			rule.ID = 0
			rule.PlanID = plan.ID
			if err := tx.Create(rule).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteRateLimitPlan 删除方案：解除用户关联并清除周期数据、删除规则与方案。
func (um *DaoManager) DeleteRateLimitPlan(planID uint) error {
	return um.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("rate_limit_plan_id = ?", planID).Updates(map[string]interface{}{
			"rate_limit_plan_id":        0,
			"rate_limit_cycle_start":    0,
			"rate_limit_cycle_upload":   0,
			"rate_limit_cycle_download": 0,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("plan_id = ?", planID).Delete(&models.RateLimitRule{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", planID).Delete(&models.RateLimitPlan{}).Error
	})
}

// ListRateLimitPlans 列出全部达量限速方案。
func (um *DaoManager) ListRateLimitPlans() ([]*models.RateLimitPlan, error) {
	var plans []*models.RateLimitPlan
	if err := um.DB.Order("id asc").Find(&plans).Error; err != nil {
		return nil, err
	}
	return plans, nil
}

// GetRateLimitPlanWithRules 获取方案及其规则（规则按阈值升序）。
func (um *DaoManager) GetRateLimitPlanWithRules(planID uint) (*models.RateLimitPlan, []*models.RateLimitRule, error) {
	var plan models.RateLimitPlan
	if err := um.DB.First(&plan, planID).Error; err != nil {
		return nil, nil, err
	}
	var rules []*models.RateLimitRule
	if err := um.DB.Where("plan_id = ?", planID).Order("priority desc, upload_threshold_bytes + download_threshold_bytes asc, id asc").Find(&rules).Error; err != nil {
		return nil, nil, err
	}
	return &plan, rules, nil
}

// ListRateLimitRulesForPlans 返回 planID -> 规则列表（按阈值升序）。
func (um *DaoManager) ListRateLimitRulesForPlans() (map[uint][]*models.RateLimitRule, error) {
	var rules []*models.RateLimitRule
	if err := um.DB.Order("priority desc, upload_threshold_bytes + download_threshold_bytes asc, id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	result := make(map[uint][]*models.RateLimitRule)
	for _, rule := range rules {
		result[rule.PlanID] = append(result[rule.PlanID], rule)
	}
	return result, nil
}

// CountUsersForRateLimitPlans 统计每个方案关联的用户数。
func (um *DaoManager) CountUsersForRateLimitPlans() (map[uint]int64, error) {
	type Row struct {
		PlanID uint
		Count  int64
	}
	var rows []Row
	if err := um.DB.Model(&models.User{}).
		Select("rate_limit_plan_id as plan_id, count(*) as count").
		Where("rate_limit_plan_id != 0").
		Group("rate_limit_plan_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]int64)
	for _, row := range rows {
		result[row.PlanID] = row.Count
	}
	return result, nil
}

// AddUsersToRateLimitPlan 批量关联用户到方案，并将这些用户的周期重置为当前时间。
// 返回成功关联的用户数。
func (um *DaoManager) AddUsersToRateLimitPlan(planID uint, usernames []string) (int64, error) {
	if len(usernames) == 0 {
		return 0, nil
	}
	now := uint64(time.Now().Unix())
	result := um.DB.Model(&models.User{}).Where("username in ?", usernames).Updates(map[string]interface{}{
		"rate_limit_plan_id":        planID,
		"rate_limit_cycle_start":    now,
		"rate_limit_cycle_upload":   0,
		"rate_limit_cycle_download": 0,
	})
	return result.RowsAffected, result.Error
}

// RemoveUsersFromRateLimitPlan 从方案移除用户（仅移除仍属于该方案的用户）并清除周期数据。
func (um *DaoManager) RemoveUsersFromRateLimitPlan(planID uint, userIDs []uint) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	result := um.DB.Model(&models.User{}).
		Where("id in ? and rate_limit_plan_id = ?", userIDs, planID).
		Updates(map[string]interface{}{
			"rate_limit_plan_id":        0,
			"rate_limit_cycle_start":    0,
			"rate_limit_cycle_upload":   0,
			"rate_limit_cycle_download": 0,
		})
	return result.RowsAffected, result.Error
}

// ListUsersForRateLimitPlan 分页列出方案关联的用户。
func (um *DaoManager) ListUsersForRateLimitPlan(planID uint, page, pageSize int, query string) ([]*models.User, int64, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	base := um.DB.Model(&models.User{}).Where("rate_limit_plan_id = ?", planID)
	if query != "" {
		base = base.Where("username like ?", "%"+query+"%")
	}
	var count int64
	if err := base.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	var users []*models.User
	if err := base.Order("id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, count, nil
}

// ListUsersWithRateLimitPlan 分页列出已关联达量限速方案的用户。
func (um *DaoManager) ListUsersWithRateLimitPlan(page, pageSize int, query string) ([]*models.User, int64, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	base := um.DB.Model(&models.User{}).Where("rate_limit_plan_id != 0")
	if query != "" {
		base = base.Where("username like ?", "%"+query+"%")
	}
	var count int64
	if err := base.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	var users []*models.User
	if err := base.Order("id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, count, nil
}

// ensureRateLimitCycle 保证用户的达量限速周期起点已初始化；若已跨周期则重置周期流量。
// 返回 (是否写入了数据库, 错误)。
func (um *DaoManager) ensureRateLimitCycle(user *models.User) (bool, error) {
	if user.RateLimitPlanID == 0 {
		return false, nil
	}
	var plan models.RateLimitPlan
	if err := um.DB.First(&plan, user.RateLimitPlanID).Error; err != nil {
		// 方案不存在：视为无方案，不再重置
		return false, nil
	}
	period := plan.PeriodSeconds
	if period == 0 {
		return false, nil
	}
	now := uint64(time.Now().Unix())
	if user.RateLimitCycleStart == 0 {
		user.RateLimitCycleStart = now
		user.RateLimitCycleUpload = 0
		user.RateLimitCycleDownload = 0
		return true, um.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
			"rate_limit_cycle_start":    now,
			"rate_limit_cycle_upload":   0,
			"rate_limit_cycle_download": 0,
		}).Error
	}
	if now < user.RateLimitCycleStart {
		return false, nil
	}
	elapsed := now - user.RateLimitCycleStart
	if elapsed < period {
		return false, nil
	}
	newStart := user.RateLimitCycleStart + (elapsed/period)*period
	user.RateLimitCycleStart = newStart
	user.RateLimitCycleUpload = 0
	user.RateLimitCycleDownload = 0
	return true, um.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"rate_limit_cycle_start":    newStart,
		"rate_limit_cycle_upload":   0,
		"rate_limit_cycle_download": 0,
	}).Error
}

// GetUserCycleTraffic 返回用户当前周期内已用流量（上传、下载字节数），
// 包含已完成会话的累计值以及当前在线会话的实时值（按用户名跨服务器汇总）。
func (um *DaoManager) GetUserCycleTraffic(userID uint) (uint64, uint64, error) {
	user, err := um.GetUserByID(userID)
	if err != nil {
		return 0, 0, err
	}
	if user.RateLimitPlanID == 0 {
		return 0, 0, nil
	}
	if _, err := um.ensureRateLimitCycle(user); err != nil {
		return 0, 0, err
	}
	upload := user.RateLimitCycleUpload
	download := user.RateLimitCycleDownload

	var records []*models.ConnectedClientInfoRecord
	if err := um.DB.Where("username = ?", user.Username).Find(&records).Error; err != nil {
		return 0, 0, err
	}
	for _, record := range records {
		upload += record.ByteSent
		download += record.ByteReceived
	}
	return upload, download, nil
}

// AddUserCycleTraffic 会话结束时把本次会话流量累加到用户周期统计。
func (um *DaoManager) AddUserCycleTraffic(username string, uploadBytes, downloadBytes uint64) error {
	result := um.DB.Model(&models.User{}).
		Where("username = ? and rate_limit_plan_id != 0", username).
		Updates(map[string]interface{}{
			"rate_limit_cycle_upload":   gorm.Expr("rate_limit_cycle_upload + ?", uploadBytes),
			"rate_limit_cycle_download": gorm.Expr("rate_limit_cycle_download + ?", downloadBytes),
		})
	return result.Error
}

// ResetUserRateLimitCycle 手动将用户的周期重置为当前时间。
func (um *DaoManager) ResetUserRateLimitCycle(userID uint) error {
	now := uint64(time.Now().Unix())
	return um.DB.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"rate_limit_cycle_start":    now,
		"rate_limit_cycle_upload":   0,
		"rate_limit_cycle_download": 0,
	}).Error
}

// ResetExpiredRateLimitCycles 检查所有关联方案的用户的周期，对已到期的进行重置。
// 返回重置的用户数。
func (um *DaoManager) ResetExpiredRateLimitCycles() (int, error) {
	var users []*models.User
	if err := um.DB.Where("rate_limit_plan_id != 0").Find(&users).Error; err != nil {
		return 0, err
	}
	plans := make(map[uint]uint64)
	var planList []*models.RateLimitPlan
	if err := um.DB.Find(&planList).Error; err != nil {
		return 0, err
	}
	for _, plan := range planList {
		plans[plan.ID] = plan.PeriodSeconds
	}
	resetCount := 0
	for _, user := range users {
		if _, ok := plans[user.RateLimitPlanID]; !ok {
			continue
		}
		changed, err := um.ensureRateLimitCycle(user)
		if err != nil {
			return resetCount, err
		}
		if changed {
			resetCount++
		}
	}
	return resetCount, nil
}

// GetUserRateLimitPlanID 读取用户关联的方案 id（0=未关联）。
func (um *DaoManager) GetUserRateLimitPlanID(userID uint) (uint, error) {
	var user models.User
	if err := um.DB.First(&user, userID).Error; err != nil {
		return 0, err
	}
	return user.RateLimitPlanID, nil
}

// RateLimitPlanNameMap 返回 planID -> 方案名。
func (um *DaoManager) RateLimitPlanNameMap() (map[uint]string, error) {
	var plans []*models.RateLimitPlan
	if err := um.DB.Find(&plans).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]string)
	for _, plan := range plans {
		result[plan.ID] = plan.Name
	}
	return result, nil
}

// GetRateLimitResetRemaining 返回用户当前周期距离下次重置的剩余秒数。
// 未关联方案时返回 0。
func (um *DaoManager) GetRateLimitResetRemaining(userID uint) (uint64, error) {
	user, err := um.GetUserByID(userID)
	if err != nil {
		return 0, err
	}
	if user.RateLimitPlanID == 0 {
		return 0, nil
	}
	var plan models.RateLimitPlan
	if err := um.DB.First(&plan, user.RateLimitPlanID).Error; err != nil {
		return 0, nil
	}
	if plan.PeriodSeconds == 0 || user.RateLimitCycleStart == 0 {
		return 0, nil
	}
	now := uint64(time.Now().Unix())
	if now < user.RateLimitCycleStart {
		return 0, nil
	}
	elapsed := now - user.RateLimitCycleStart
	if elapsed >= plan.PeriodSeconds {
		return 0, nil
	}
	return plan.PeriodSeconds - elapsed, nil
}
