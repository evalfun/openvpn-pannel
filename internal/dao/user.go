package dao

import (
	"errors"
	"fmt"
	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/models"
	"openvpn-pannel/internal/passwd"

	"gorm.io/gorm"
)

type DaoManager struct {
	DB  *gorm.DB
	cfg *config.Config
}

func NewDaoManager(cfg *config.Config) (*DaoManager, error) {
	db, err := models.ConnectDB(cfg)

	return &DaoManager{
		DB:  db,
		cfg: cfg,
	}, err
}
func (um *DaoManager) CreateUser(username, password, description string, rateLimitType uint, uploadLimitKB, downloadLimitKB uint64) error {

	hashedPasswd, err := passwd.Hash(password)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %s", err.Error())
	}

	// RateLimitType 在模型上带 default:1（用于老数据迁移回填）。gorm 默认会忽略零值字段，
	// 导致显式选择的“不设置限速策略”(0) 被写成默认值 1。这里用 map 插入，保证 0 也能写入。
	record := map[string]interface{}{
		"username":          username,
		"password":          hashedPasswd,
		"description":       description,
		"rate_limit_type":   rateLimitType,
		"upload_limit_kb":   uploadLimitKB,
		"download_limit_kb": downloadLimitKB,
	}
	return um.DB.Model(&models.User{}).Create(record).Error
}

// CreateUserWithGroup 创建一个用户，并在 groupName 非空时将其加入同名用户组。
// groupName 为空表示不加入任何用户组。用户名已存在或用户组不存在时返回可读错误，
// 用户创建与加入用户组在同一事务内完成，避免出现“建了用户却没进组”的中间状态。
func (um *DaoManager) CreateUserWithGroup(username, password, description, groupName string) error {
	if _, err := um.GetUserByUsername(username); err == nil {
		return fmt.Errorf("用户名已存在")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	var group *models.Group
	if groupName != "" {
		g, err := um.GetGroupByName(groupName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("用户组「%s」不存在", groupName)
			}
			return fmt.Errorf("查询用户组失败: %s", err.Error())
		}
		group = g
	}

	hashedPasswd, err := passwd.Hash(password)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %s", err.Error())
	}

	tx := um.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %s", tx.Error.Error())
	}
	record := map[string]interface{}{
		"username":          username,
		"password":          hashedPasswd,
		"description":       description,
		"rate_limit_type":   models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN,
		"upload_limit_kb":   0,
		"download_limit_kb": 0,
	}
	if err := tx.Model(&models.User{}).Create(record).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("创建用户失败: %s", err.Error())
	}
	if group != nil {
		var created models.User
		if err := tx.Where("username = ?", username).First(&created).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("创建用户失败: %s", err.Error())
		}
		if err := tx.Create(&models.UserGroup{UserID: created.ID, GroupID: group.ID}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("加入用户组「%s」失败: %s", groupName, err.Error())
		}
	}
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("提交事务失败: %s", err.Error())
	}
	return nil
}

func (um *DaoManager) AuthUser(username, password string) (*models.User, error) {
	var user models.User
	err := um.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	authenticated := false
	if passwd.IsBcrypt(user.Password) {
		authenticated = passwd.VerifyBcrypt(user.Password, password)
	} else if passwd.VerifyLegacySHA256(user.Password, password, um.cfg.PasswordSalt) {
		// 旧版 sha256(password+盐) 校验通过：透明升级为 bcrypt，升级失败不影响本次登录。
		authenticated = true
		if upgraded, herr := passwd.Hash(password); herr == nil {
			um.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("password", upgraded)
		}
	}
	if !authenticated {
		return nil, fmt.Errorf("authentication failed")
	}
	return &user, nil
}

func (um *DaoManager) BlukDeleteUser(userIDList []uint) error {
	// 删除用户组中的用户记录
	tx := um.DB.Begin()
	// err := tx.Create(server).Error
	err := tx.Where("id in (?)", userIDList).Delete(&models.UserGroup{}).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户组中的用户记录失败 %s", err.Error())
	}
	// 删除 ServerPermission 中的记录
	err = tx.Where("id in (?) and obj_type = ?", userIDList, models.SERVER_PERM_OBJ_TYPE_USER).Delete(&models.ServerPermission{}).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户权限记录失败 %s", err.Error())
	}
	// 删除用户
	err = tx.Where("id in (?)", userIDList).Delete(&models.User{}).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户组中的用户记录失败 %s", err.Error())
	}
	err = tx.Commit().Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除用户组中的用户记录失败: 提交事务失败: %s", err.Error())
	}
	return nil
}

func (um *DaoManager) GetUserByID(userID uint) (*models.User, error) {
	var user models.User
	err := um.DB.First(&user, userID).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (um *DaoManager) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := um.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (um *DaoManager) UpdateUserInfo(userID uint, description, password string, rateLimitType uint, uploadLimitKB, downloadLimitKB uint64) error {
	var user models.User
	err := um.DB.First(&user, userID).Error
	if err != nil {
		return err
	}
	user.Description = description
	user.RateLimitType = rateLimitType
	user.UploadLimitKB = uploadLimitKB
	user.DownloadLimitKB = downloadLimitKB
	if password != "" {
		hashedPasswd, err := passwd.Hash(password)
		if err != nil {
			return fmt.Errorf("密码哈希失败: %s", err.Error())
		}
		user.Password = hashedPasswd
	}
	return um.DB.Save(&user).Error
}

// SetUserMFA 设置用户的多因素认证类型与数据。mfaType 为 MFA_TYPE_NONE 时清空数据。
func (um *DaoManager) SetUserMFA(userID uint, mfaType uint, data string) error {
	if mfaType == models.MFA_TYPE_NONE {
		data = ""
	}
	result := um.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"mfa_type": mfaType,
			"mfa_data": data,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (um *DaoManager) UpdateUserTraffic(username string, uploadBytes, downloadBytes uint64) error {
	result := um.DB.Model(&models.User{}).
		Where("username = ?", username).
		Updates(map[string]interface{}{
			"upload_traffic":   gorm.Expr("upload_traffic + ?", uploadBytes),
			"download_traffic": gorm.Expr("download_traffic + ?", downloadBytes),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (um *DaoManager) ResetUserTraffic(userID uint) error {
	tx := um.DB.Begin()
	err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"upload_traffic":   0,
		"download_traffic": 0,
	}).Error
	if err != nil {
		tx.Rollback()
		return err
	}
	var user models.User
	err = tx.First(&user, userID).Error
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Model(&models.ConnectedClientInfoRecord{}).Where("username = ?", user.Username).Updates(map[string]interface{}{
		"byte_received": 0,
		"byte_sent":     0,
	}).Error
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// 查询用户数量（excludePlanID != 0 时排除已关联该限速方案的用户）
func (um *DaoManager) GetUserCount(queryName string, excludeGroupID uint, excludePlanID uint) (int64, error) {
	var userCount int64
	base := um.DB.Model(&models.User{}).
		Where("id not in (?)", um.DB.Table("user_groups").Where("group_id=?", excludeGroupID).Select("user_id"))
	if excludePlanID != 0 {
		base = base.Where("rate_limit_plan_id <> ?", excludePlanID)
	}
	if queryName != "" {
		base = base.Where("username like ?", "%"+queryName+"%")
	}
	result := base.Count(&userCount)
	if result.Error != nil {
		return 0, errors.New("查询用户数量失败 " + result.Error.Error())
	}
	return userCount, nil
}

// 列出用户（excludePlanID != 0 时排除已关联该限速方案的用户）
func (um *DaoManager) ListUsers(page int, pageSize int, queryName string, excludeGroupID uint, excludePlanID uint) ([]*models.User, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	var userList []*models.User
	base := um.DB.Model(&models.User{}).
		Where("id not in (?)", um.DB.Table("user_groups").Where("group_id=?", excludeGroupID).Select("user_id"))
	if excludePlanID != 0 {
		base = base.Where("rate_limit_plan_id <> ?", excludePlanID)
	}
	if queryName != "" {
		base = base.Where("username like ?", "%"+queryName+"%")
	}
	result := base.Limit(pageSize).Offset((page - 1) * pageSize).Find(&userList)
	if result.Error != nil {
		return nil, errors.New("列出用户失败 " + result.Error.Error())
	}
	return userList, nil
}
