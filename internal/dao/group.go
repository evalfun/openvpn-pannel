package dao

import (
	"errors"
	"fmt"
	"openvpn-pannel/internal/models"

	"gorm.io/gorm"
)

func (um *DaoManager) CreateGroup(name, description string, uploadLimitKB, downloadLimitKB uint64) error {
	group := models.Group{
		Name:            name,
		Description:     description,
		UploadLimitKB:   uploadLimitKB,
		DownloadLimitKB: downloadLimitKB,
	}
	return um.DB.Create(&group).Error
}

// 删除用户组
func (um *DaoManager) DeleteGroup(groupID uint) error {
	tx := um.DB.Begin()
	// 清理用户组中的ACL
	result := tx.Where("group_id = ?", groupID).Delete(&models.GroupACL{})
	if result.Error != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户组中的ACL记录失败 %s", result.Error.Error())
	}
	// 清理用户组中的用户
	result = tx.Where("group_id = ?", groupID).Delete(&models.UserGroup{})
	if result.Error != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户组中的用户记录失败 %s", result.Error.Error())
	}
	// 删除 ServerPermission 中的记录
	err := tx.Where("id =? and obj_type = ?", groupID, models.SERVER_PERM_OBJ_TYPE_GROUP).Delete(&models.ServerPermission{}).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除服务器权限记录失败 %s", err.Error())
	}

	err = tx.Delete(&models.Group{}, groupID).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户组失败: %s", err.Error())
	}

	err = tx.Commit().Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户组失败: 提交事务失败: %s", err.Error())
	}
	return nil
}

func (um *DaoManager) AddUserToGroup(userID, groupID uint) error {
	// 检查用户是否存在组内
	var count int64
	um.DB.Model(&models.UserGroup{}).Where("user_id = ? AND group_id = ?", userID, groupID).Count(&count)
	if count > 0 {
		return fmt.Errorf("user %d is already in group %d", userID, groupID)
	}
	userGroup := models.UserGroup{
		UserID:  userID,
		GroupID: groupID,
	}
	return um.DB.Create(&userGroup).Error
}
func (um *DaoManager) BatchAddUsersToGroup(userNameList []string, groupName string) error {
	var groupModel *models.Group
	tx := um.DB.Begin()
	err := tx.Where("name = ?", groupName).First(&groupModel).Error
	if err != nil {
		tx.Rollback()
		return err
	}
	var userModelList []*models.User
	err = um.DB.Where("username IN ?", userNameList).Find(&userModelList).Error
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, userModel := range userModelList {
		var count int64
		um.DB.Model(&models.UserGroup{}).Where("user_id = ? AND group_id = ?", userModel.ID, groupModel.ID).Count(&count)
		if count > 0 {
			tx.Rollback()
			return fmt.Errorf("user %d is already in group %d", userModel.ID, groupModel.ID)
		}
		err := tx.Create(&models.UserGroup{
			UserID:  userModel.ID,
			GroupID: groupModel.ID,
		}).Error
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("添加用户 %s 到用户组 %s 失败: %s", userModel.Username, groupModel.Name, err.Error())
		}
	}
	return tx.Commit().Error
}

func (um *DaoManager) UserInGroup(userID, groupID uint) (bool, error) {
	var count int64
	err := um.DB.Model(&models.UserGroup{}).Where("user_id = ? AND group_id = ?", userID, groupID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (um *DaoManager) RemoveUserFromGroup(userID, groupID uint) error {
	return um.DB.Where("user_id = ? AND group_id = ?", userID, groupID).Delete(&models.UserGroup{}).Error
}

func (um *DaoManager) GetGroupByID(groupID uint) (*models.Group, error) {
	var group models.Group
	err := um.DB.First(&group, groupID).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (um *DaoManager) GetGroupByName(groupName string) (*models.Group, error) {
	var group models.Group
	err := um.DB.Where("name = ?", groupName).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// 列出用户组里的用户
func (um *DaoManager) ListUsersInGroup(groupID uint) ([]*models.User, error) {
	var users []*models.User
	err := um.DB.Joins("JOIN user_groups ON users.id = user_groups.user_id").
		Where("user_groups.group_id = ?", groupID).Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

// ListUsersInGroupPaged 分页列出用户组里的用户（可按用户名模糊搜索），同时返回总数。
func (um *DaoManager) ListUsersInGroupPaged(groupID uint, query string, page, pageSize int) ([]*models.User, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	base := um.DB.Model(&models.User{}).
		Joins("JOIN user_groups ON users.id = user_groups.user_id").
		Where("user_groups.group_id = ?", groupID)
	if query != "" {
		base = base.Where("users.username like ?", "%"+query+"%")
	}
	var count int64
	if err := base.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	var users []*models.User
	if err := base.Order("users.id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, count, nil
}

// 列出用户所在的用户组
func (um *DaoManager) ListGroupsForUser(userID uint) ([]*models.Group, error) {
	var groups []*models.Group
	err := um.DB.Joins("JOIN user_groups ON groups.id = user_groups.group_id").
		Where("user_groups.user_id = ?", userID).Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// 列出用户组
func (um *DaoManager) ListGroups(query string, page int, pageSize int) ([]*models.Group, uint, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	var groupList []*models.Group
	var result *gorm.DB
	var count int64
	if query == "" {
		result = um.DB.Limit(pageSize).Offset((page - 1) * pageSize).Find(&groupList)
		um.DB.Model(&models.Group{}).Count(&count)
	} else {
		result = um.DB.Where("name like ?", "%"+query+"%").Limit(pageSize).Offset((page - 1) * pageSize).Find(&groupList)
		um.DB.Model(&models.Group{}).Where("name like ?", "%"+query+"%").Count(&count)
	}

	if result.Error != nil {
		return nil, 0, errors.New("列出用户组失败 " + result.Error.Error())
	}
	return groupList, uint(count), nil
}

// 查询用户组数量
func (um *DaoManager) GetGroupCount() (int64, error) {
	var groupCount int64
	result := um.DB.Model(&models.Group{}).Count(&groupCount)
	if result.Error != nil {
		return 0, errors.New("查询用户组数量失败 " + result.Error.Error())
	}
	return groupCount, nil
}

// 列出用户组的acl
func (um *DaoManager) ListGroupACL(groupID uint) ([]*models.GroupACL, error) {
	var groupACLList []*models.GroupACL
	result := um.DB.Where("group_id = ?", groupID).Find(&groupACLList)
	if result.Error != nil {
		return nil, errors.New("列出用户组ACL失败 " + result.Error.Error())
	}
	return groupACLList, nil
}
func (um *DaoManager) ListGroupACLBluk(groupIDList []uint) ([]*models.GroupACL, error) {
	var groupACLList []*models.GroupACL
	result := um.DB.Where("group_id IN ?", groupIDList).Find(&groupACLList)
	if result.Error != nil {
		return nil, errors.New("列出用户组ACL失败 " + result.Error.Error())
	}
	return groupACLList, nil
}

func (um *DaoManager) GetAllGroupACLByUser(userID uint) ([]*models.GroupACL, error) {
	var groupACLList []*models.GroupACL
	result := um.DB.Where("group_id IN (?)", um.DB.Table("user_groups").Where("user_id=?", userID).Select("group_id")).Find(&groupACLList)
	if result.Error != nil {
		return nil, errors.New("列出用户的所有用户组ACL失败 " + result.Error.Error())
	}
	// for _, groupACL := range groupACLList {
	// 	log.Println(groupACL)
	// }

	return groupACLList, nil
}

// 添加用户组acl
func (um *DaoManager) AddGroupACL(groupID uint, ACLType uint, ACLValue string) error {
	var count int64
	um.DB.Model(&models.GroupACL{}).Where("group_id = ? AND type = ? AND value = ?", groupID, ACLType, ACLValue).Count(&count)
	if count > 0 {
		return fmt.Errorf("用户组 %d 中已有相同的ACL规则 %d %s", groupID, ACLType, ACLValue)
	}
	groupACL := models.GroupACL{
		GroupID: groupID,
		Type:    ACLType,
		Value:   ACLValue,
	}
	return um.DB.Create(&groupACL).Error
}

// 删除用户组ACL
func (um *DaoManager) DeleteGroupACL(GroupACLID uint) error {
	result := um.DB.Delete(&models.GroupACL{}, GroupACLID)
	return result.Error
}

// 更新用户组描述与限速
func (um *DaoManager) UpdateGroup(groupID uint, description string, uploadLimitKB, downloadLimitKB uint64) error {
	// 先确认用户组存在；不依赖 RowsAffected，避免 MySQL 在“值未变化”时返回 0 行导致误判。
	if err := um.DB.First(&models.Group{}, groupID).Error; err != nil {
		return errors.New("没有找到符合条件的用户组")
	}
	result := um.DB.Model(&models.Group{}).Where("id = ?", groupID).Updates(map[string]interface{}{
		"description":       description,
		"upload_limit_kb":   uploadLimitKB,
		"download_limit_kb": downloadLimitKB,
	})
	return result.Error
}
