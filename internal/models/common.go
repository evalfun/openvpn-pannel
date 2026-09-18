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
	return db.AutoMigrate(&Server{}, &ServerRoute{}, &ConnectedClientInfoRecord{},
		&ClientConfig{}, &User{}, &Group{}, &UserGroup{}, &AddedServerACLRecord{},
		&GroupACL{}, &ServerPermission{}, &AppResourceRecord{}, &ServerEvent{}, &Certificate{}, &CertificateEvent{})
}
