package dao

import (
	"openvpn-pannel/internal/models"
)

// ListActiveGroupsForUser 返回用户在指定服务器上的“活跃用户组”。
//
// 活跃用户组 = 用户所属、且被该服务器允许连接的用户组。判定顺序与 GetACLByUser 的
// 权限解析保持一致：
//   - 用户级权限存在 DENY：用户整体被拒绝，返回空（无活跃组）；
//   - 用户级权限存在 PERMIT：用户所属的全部用户组都视为活跃；
//   - 否则按用户组权限判断：该组存在 PERMIT 且不存在 DENY 时才活跃（DENY 优先）。
//
// 例：用户 A 同属组 A、组 B；服务器 A 只允许组 A 连接；则用户 A 连到服务器 A 时，
// 活跃用户组只有组 A。
func (um *DaoManager) ListActiveGroupsForUser(userID, serverID uint) ([]*models.Group, error) {
	// 1. 用户级权限
	var userPermList []*models.ServerPermission
	err := um.DB.Where("server_id = ? and obj_type = ? and obj_id = ?",
		serverID, models.SERVER_PERM_OBJ_TYPE_USER, userID).Find(&userPermList).Error
	if err != nil {
		return nil, err
	}
	userAllow := false
	for _, perm := range userPermList {
		if perm.Action == models.SERVER_PERM_ACTION_DENY {
			// 用户被显式拒绝，无可用权限
			return nil, nil
		}
		if perm.Action == models.SERVER_PERM_ACTION_PERMIT {
			userAllow = true
		}
	}

	groupList, err := um.ListGroupsForUser(userID)
	if err != nil {
		return nil, err
	}
	// 2. 用户级放行：其所有用户组均视为活跃
	if userAllow {
		return groupList, nil
	}
	if len(groupList) == 0 {
		return nil, nil
	}

	// 3. 用户组级权限：DENY 优先，存在 PERMIT 才活跃
	groupIDList := make([]uint, 0, len(groupList))
	for _, group := range groupList {
		groupIDList = append(groupIDList, group.ID)
	}
	var groupPermList []*models.ServerPermission
	err = um.DB.Where("server_id = ? and obj_type = ? and obj_id in ?",
		serverID, models.SERVER_PERM_OBJ_TYPE_GROUP, groupIDList).Find(&groupPermList).Error
	if err != nil {
		return nil, err
	}
	permMap := make(map[uint]bool)
	for _, perm := range groupPermList {
		if perm.Action == models.SERVER_PERM_ACTION_DENY {
			permMap[perm.ObjID] = false
			continue
		}
		if perm.Action == models.SERVER_PERM_ACTION_PERMIT {
			if allowed, ok := permMap[perm.ObjID]; ok && !allowed {
				continue
			}
			permMap[perm.ObjID] = true
		}
	}

	activeGroups := make([]*models.Group, 0, len(groupList))
	for _, group := range groupList {
		if permMap[group.ID] {
			activeGroups = append(activeGroups, group)
		}
	}
	return activeGroups, nil
}

// minRate 计算活跃组限速的最小值(KB/s)，用于“依据活跃用户组的最低速率”。
// 0 表示不限速，视为无穷大不参与比较；若没有有限值则返回 0（不限速）。
func minRate(values []uint64) uint64 {
	var result uint64
	found := false
	for _, v := range values {
		if v == 0 {
			continue
		}
		if !found || v < result {
			result = v
			found = true
		}
	}
	return result
}

// maxRate 计算活跃组限速的最大值(KB/s)，用于“依据活跃用户组的最高速率”。
// 0 表示不限速，视为最大值；任一活跃组不限速则整体不限速，返回 0。
func maxRate(values []uint64) uint64 {
	var result uint64
	for _, v := range values {
		if v == 0 {
			return 0
		}
		if v > result {
			result = v
		}
	}
	return result
}

// ResolveUserRateLimit 计算用户连到指定服务器时的生效限速，返回 (上传, 下载) KB/s。
// 0 表示不限速。约定：上传 = 服务器 -> 客户端；下载 = 客户端 -> 服务器。
//
// 先看用户限速策略：
//   - 不设置限速：不限速；
//   - 固定限速：使用用户自身的值；
//   - 依据活跃用户组最低/最高速率：对活跃用户组的限速分别取最低/最高。
//
// 未知策略按默认策略（活跃用户组最低速率）处理。
func (um *DaoManager) ResolveUserRateLimit(userID, serverID uint) (uint64, uint64, error) {
	var user models.User
	if err := um.DB.First(&user, userID).Error; err != nil {
		return 0, 0, err
	}

	switch user.RateLimitType {
	case models.RATE_LIMIT_TYPE_NONE:
		return 0, 0, nil
	case models.RATE_LIMIT_TYPE_FIXED:
		return user.UploadLimitKB, user.DownloadLimitKB, nil
	}

	groups, err := um.ListActiveGroupsForUser(userID, serverID)
	if err != nil {
		return 0, 0, err
	}
	uploads := make([]uint64, 0, len(groups))
	downloads := make([]uint64, 0, len(groups))
	for _, group := range groups {
		uploads = append(uploads, group.UploadLimitKB)
		downloads = append(downloads, group.DownloadLimitKB)
	}

	if user.RateLimitType == models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MAX {
		return maxRate(uploads), maxRate(downloads), nil
	}
	// 默认/未知策略：依据活跃用户组的最低速率
	return minRate(uploads), minRate(downloads), nil
}
