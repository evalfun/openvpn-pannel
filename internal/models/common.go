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
	// 资源表历史结构升级必须在 AutoMigrate 之前处理：旧表只有单主键 id，
	// AutoMigrate 无法为已有数据加出 NOT NULL 的 set_id 列（SQLite 会直接报错）。
	if err := migrateResourceSetSchema(db); err != nil {
		return err
	}
	err := db.AutoMigrate(&Server{}, &ServerRoute{}, &ConnectedClientInfoRecord{},
		&ClientConfig{}, &User{}, &Group{}, &UserGroup{}, &AddedServerACLRecord{},
		&GroupACL{}, &ServerPermission{}, &AppResourceRecord{}, &AppResourceSetState{}, &ServerEvent{}, &Certificate{}, &CertificateEvent{},
		&RateLimitPlan{}, &RateLimitRule{}, &ServerProcessRecord{})
	if err != nil {
		return err
	}
	return migrateMFAFromTOTP(db)
}

// migrateResourceSetSchema 处理 app_resource_records 从“单主键 id”升级到“复合主键 (set_id, id)”。
// 由于 AutoMigrate 不能给已有行补上 NOT NULL 且无默认值的 set_id 列，这里在检测到旧结构时
// 采用“保留旧表数据 -> 重建新表 -> 回填默认资源集 -> 换名”的方式完成迁移。
// 幂等：新结构（已含 set_id 列）下不会做任何事。
func migrateResourceSetSchema(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&AppResourceRecord{}) {
		return nil // 全新库，交给 AutoMigrate 建表
	}
	if m.HasColumn(&AppResourceRecord{}, "set_id") {
		return nil // 已是新结构
	}
	// 旧结构：id 为主键，无 set_id。重建为新结构并把历史数据归入默认资源集。
	const defaultSetID = "linux-iptables"
	switch db.Dialector.Name() {
	case "sqlite":
		stmts := []string{
			"ALTER TABLE app_resource_records RENAME TO app_resource_records_old",
			`CREATE TABLE app_resource_records (
				set_id varchar(50) NOT NULL,
				id varchar(50) NOT NULL,
				content text NOT NULL,
				PRIMARY KEY (set_id, id)
			)`,
			`INSERT INTO app_resource_records (set_id, id, content)
				SELECT '` + defaultSetID + `', id, content FROM app_resource_records_old`,
			"DROP TABLE app_resource_records_old",
		}
		for _, s := range stmts {
			if err := db.Exec(s).Error; err != nil {
				return fmt.Errorf("迁移资源表失败: %w", err)
			}
		}
	case "mysql":
		if err := db.Exec(
			"ALTER TABLE app_resource_records ADD COLUMN set_id varchar(50) NOT NULL DEFAULT '" + defaultSetID + "'",
		).Error; err != nil {
			return fmt.Errorf("迁移资源表失败: %w", err)
		}
		// 改为复合主键；MySQL 需先删旧主键再加新主键。
		if err := db.Exec("ALTER TABLE app_resource_records DROP PRIMARY KEY").Error; err != nil {
			return fmt.Errorf("迁移资源表失败: %w", err)
		}
		if err := db.Exec("ALTER TABLE app_resource_records ADD PRIMARY KEY (set_id, id)").Error; err != nil {
			return fmt.Errorf("迁移资源表失败: %w", err)
		}
		if err := db.Exec("ALTER TABLE app_resource_records MODIFY set_id varchar(50) NOT NULL").Error; err != nil {
			return fmt.Errorf("迁移资源表失败: %w", err)
		}
	}
	return nil
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
