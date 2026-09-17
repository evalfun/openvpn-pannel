package dao

import (
	"errors"
	"fmt"
	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/models"

	"crypto/sha256"

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

	hash := sha256.Sum256([]byte(password + um.cfg.PasswordSalt))
	hashed_passwd := fmt.Sprintf("%x", hash)

	// RateLimitType 在模型上带 default:1（用于老数据迁移回填）。gorm 默认会忽略零值字段，
	// 导致显式选择的“不设置限速策略”(0) 被写成默认值 1。这里用 map 插入，保证 0 也能写入。
	record := map[string]interface{}{
		"username":          username,
		"password":          string(hashed_passwd),
		"description":       description,
		"rate_limit_type":   rateLimitType,
		"upload_limit_kb":   uploadLimitKB,
		"download_limit_kb": downloadLimitKB,
	}
	return um.DB.Model(&models.User{}).Create(record).Error
}

func (um *DaoManager) AuthUser(username, password string) (*models.User, error) {
	var user models.User
	err := um.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(password + um.cfg.PasswordSalt))
	hashed_passwd := fmt.Sprintf("%x", hash)
	if user.Password != string(hashed_passwd) {
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
		hash := sha256.Sum256([]byte(password + um.cfg.PasswordSalt))
		hashed_passwd := fmt.Sprintf("%x", hash)
		user.Password = string(hashed_passwd)
	}
	return um.DB.Save(&user).Error
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

// 查询用户数量
func (um *DaoManager) GetUserCount(queryName string, excludeGroupID uint) (int64, error) {
	var userCount int64
	var result *gorm.DB
	// select count(*) from users where id not in (select user_id from user_groups where group_id=14);
	if queryName != "" {
		result = um.DB.Where("username like ? and id not in (?)", "%"+queryName+"%", um.DB.Table("user_groups").Where("group_id=?", excludeGroupID).Select("user_id")).Model(&models.User{}).Count(&userCount)
	} else {
		result = um.DB.Where("id not in (?)", um.DB.Table("user_groups").Where("group_id=?", excludeGroupID).Select("user_id")).Model(&models.User{}).Count(&userCount)
	}
	if result.Error != nil {
		return 0, errors.New("查询用户数量失败 " + result.Error.Error())
	}
	return userCount, nil
}

// 列出用户
func (um *DaoManager) ListUsers(page int, pageSize int, queryName string, excludeGroupID uint) ([]*models.User, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	var userList []*models.User
	var result *gorm.DB
	if queryName != "" {
		// select * from users where id not in (select user_id from user_groups where group_id=14); 排除用户组id
		result = um.DB.Where("username like ? and id not in (?)", "%"+queryName+"%", um.DB.Table("user_groups").Where("group_id=?", excludeGroupID).Select("user_id")).Limit(pageSize).Offset((page - 1) * pageSize).Find(&userList)
	} else {
		//result = um.DB.Limit(pageSize).Offset((page - 1) * pageSize).Find(&userList)
		result = um.DB.Where("id not in (?)", um.DB.Table("user_groups").Where("group_id=?", excludeGroupID).Select("user_id")).Limit(pageSize).Offset((page - 1) * pageSize).Find(&userList)
	}
	if result.Error != nil {
		return nil, errors.New("列出用户失败 " + result.Error.Error())
	}
	return userList, nil
}
