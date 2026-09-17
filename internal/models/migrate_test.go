package models

import (
	"path/filepath"
	"testing"

	"openvpn-pannel/internal/config"
)

// 升级场景：老的 users 表没有 rate_limit_type 等列，迁移后老用户应回填为默认策略
// (1 = 依据活跃用户组的最低速率)，而不是 0(不限速)。
func TestMigrateBackfillsRateLimitDefaults(t *testing.T) {
	cfg := &config.Config{SQLiteDB: filepath.Join(t.TempDir(), "old.db")}
	db, err := ConnectDB(cfg)
	if err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}

	// 模拟旧版本的表结构（缺少限速相关列）
	oldSchema := `CREATE TABLE users (
		id integer PRIMARY KEY AUTOINCREMENT,
		username text NOT NULL,
		password text NOT NULL,
		description text NOT NULL,
		upload_traffic integer NOT NULL DEFAULT 0,
		download_traffic integer NOT NULL DEFAULT 0
	)`
	if err := db.Exec(oldSchema).Error; err != nil {
		t.Fatalf("create old users table: %v", err)
	}
	if err := db.Exec(`INSERT INTO users (username, password, description) VALUES ('olduser', 'x', '')`).Error; err != nil {
		t.Fatalf("insert old user: %v", err)
	}

	if err := MigrateDB(db); err != nil {
		t.Fatalf("MigrateDB: %v", err)
	}

	var row struct {
		RateLimitType   uint
		UploadLimitKB   uint64
		DownloadLimitKB uint64
	}
	if err := db.Table("users").Select("rate_limit_type, upload_limit_kb, download_limit_kb").
		Where("username = ?", "olduser").Scan(&row).Error; err != nil {
		t.Fatalf("query migrated user: %v", err)
	}
	if row.RateLimitType != RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN {
		t.Fatalf("RateLimitType = %d, want %d", row.RateLimitType, RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN)
	}
	if row.UploadLimitKB != 0 || row.DownloadLimitKB != 0 {
		t.Fatalf("limits = (%d,%d), want (0,0)", row.UploadLimitKB, row.DownloadLimitKB)
	}
}

// 升级场景：老的 groups 表迁移后限速应默认 0(不限速)。
func TestMigrateGroupLimitDefaults(t *testing.T) {
	cfg := &config.Config{SQLiteDB: filepath.Join(t.TempDir(), "old2.db")}
	db, err := ConnectDB(cfg)
	if err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}
	oldSchema := `CREATE TABLE groups (
		id integer PRIMARY KEY AUTOINCREMENT,
		name text NOT NULL,
		description text
	)`
	if err := db.Exec(oldSchema).Error; err != nil {
		t.Fatalf("create old groups table: %v", err)
	}
	if err := db.Exec(`INSERT INTO groups (name, description) VALUES ('oldgroup', '')`).Error; err != nil {
		t.Fatalf("insert old group: %v", err)
	}
	if err := MigrateDB(db); err != nil {
		t.Fatalf("MigrateDB: %v", err)
	}
	var row struct {
		UploadLimitKB   uint64
		DownloadLimitKB uint64
	}
	if err := db.Table("groups").Select("upload_limit_kb, download_limit_kb").
		Where("name = ?", "oldgroup").Scan(&row).Error; err != nil {
		t.Fatalf("query migrated group: %v", err)
	}
	if row.UploadLimitKB != 0 || row.DownloadLimitKB != 0 {
		t.Fatalf("group limits = (%d,%d), want (0,0)", row.UploadLimitKB, row.DownloadLimitKB)
	}
}
