package models

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"openvpn-pannel/internal/config"
)

func ConnectDB(config *config.Config) (*gorm.DB, error) {
	if config.SQLiteDB != "" {
		// 使用SQLite数据库
		db, err := gorm.Open(sqlite.Open(config.SQLiteDB), &gorm.Config{})
		return db, err
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.MysqlUser, config.MysqlPass, config.MysqlAddr, config.MysqlDB)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	return db, err
}

func MigrateDB(db *gorm.DB) error {
	err := db.AutoMigrate(&Server{}, &ServerRoute{}, &ConnectedClientInfoRecord{},
		&ClientConfig{}, &User{}, &Group{}, &UserGroup{}, &AddedServerACLRecord{},
		&GroupACL{}, &ServerPermission{}, &AppResourceRecord{}, &ServerEvent{}, &Certificate{}, &CertificateEvent{},
		&RateLimitPlan{}, &RateLimitRule{})
	if err != nil {
		return err
	}
	return migrateMFAFromTOTP(db)
}

// migrateMFAFromTOTP 把旧版本的 users.totp_enabled / totp_secret 迁移到
// mfa_type / mfa_data（MFA_TYPE_TOTP）。幂等：仅在旧列存在、且新列仍为默认值时执行。
// AutoMigrate 不会删除旧列，因此迁移后旧列会保留但不再使用。
func migrateMFAFromTOTP(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasColumn(&User{}, "totp_enabled") || !m.HasColumn(&User{}, "totp_secret") {
		return nil
	}
	return db.Exec(
		"UPDATE users SET mfa_type = ?, mfa_data = totp_secret WHERE totp_enabled = ? AND mfa_type = ?",
		MFA_TYPE_TOTP, true, MFA_TYPE_NONE,
	).Error
}
