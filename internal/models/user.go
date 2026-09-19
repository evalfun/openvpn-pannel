package models

// 用户限速策略类型。
// 用户上线时先看用户策略，策略为“依据活跃用户组”时再取活跃用户组的限速值。
const (
	// RATE_LIMIT_TYPE_NONE 不设置任何限速策略（不限速）
	RATE_LIMIT_TYPE_NONE = 0
	// RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN 依据活跃用户组的最低速率
	RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN = 1
	// RATE_LIMIT_TYPE_ACTIVE_GROUP_MAX 依据活跃用户组的最高速率
	RATE_LIMIT_TYPE_ACTIVE_GROUP_MAX = 2
	// RATE_LIMIT_TYPE_FIXED 固定限速（使用用户自身的 upload_limit_kb / download_limit_kb）
	RATE_LIMIT_TYPE_FIXED = 3
)

// 限速单位为 KB/s（千字节每秒，1024 字节）。
// 约定：上传 = 服务器 -> 客户端；下载 = 客户端 -> 服务器。
type User struct {
	ID              uint   `gorm:"primarykey"`
	Username        string `gorm:"uniqueIndex;not null;type:varchar(50)" json:"username"`
	Password        string `gorm:"not null" json:"-"`
	Description     string `gorm:"not null" json:"description"`
	UploadTraffic   uint64 `gorm:"not null;default:0" json:"upload_traffic"`
	DownloadTraffic uint64 `gorm:"not null;default:0" json:"download_traffic"`
	// RateLimitType 用户限速策略，默认 1（依据活跃用户组的最低速率）。
	// 取值见 RATE_LIMIT_TYPE_* 常量。
	RateLimitType uint `gorm:"not null;default:1" json:"rate_limit_type"`
	// UploadLimitKB 固定限速时的上传限速(KB/s)，仅 RateLimitType=3 时生效，0=不限速。
	UploadLimitKB uint64 `gorm:"not null;default:0" json:"upload_limit_kb"`
	// DownloadLimitKB 固定限速时的下载限速(KB/s)，仅 RateLimitType=3 时生效，0=不限速。
	DownloadLimitKB uint64 `gorm:"not null;default:0" json:"download_limit_kb"`
	// RateLimitPlanID 关联的达量限速方案 id，0=未关联。
	RateLimitPlanID uint `gorm:"not null;default:0" json:"rate_limit_plan_id"`
	// RateLimitCycleStart 当前达量限速周期的起始时间(unix 秒)。
	RateLimitCycleStart uint64 `gorm:"not null;default:0" json:"rate_limit_cycle_start"`
	// RateLimitCycleUpload 当前周期内已完成会话累计的上传流量(字节)。上传=服务器->客户端。
	RateLimitCycleUpload uint64 `gorm:"not null;default:0" json:"rate_limit_cycle_upload"`
	// RateLimitCycleDownload 当前周期内已完成会话累计的下载流量(字节)。下载=客户端->服务器。
	RateLimitCycleDownload uint64 `gorm:"not null;default:0" json:"rate_limit_cycle_download"`
}

type Group struct {
	ID          uint   `gorm:"primarykey"`
	Name        string `json:"name" gorm:"unique;not null;uniqueIndex;type:varchar(50)"`
	Description string `json:"description"`
	// UploadLimitKB 用户组上传限速(KB/s)，0=不限速。上传=服务器->客户端。
	UploadLimitKB uint64 `gorm:"not null;default:0" json:"upload_limit_kb"`
	// DownloadLimitKB 用户组下载限速(KB/s)，0=不限速。下载=客户端->服务器。
	DownloadLimitKB uint64 `gorm:"not null;default:0" json:"download_limit_kb"`
}

type GroupACL struct {
	ID      uint   `json:"id" gorm:"primarykey"`
	GroupID uint   `json:"-" gorm:"not null;index"`
	Type    uint   `json:"type" gorm:"not null"`
	Value   string `json:"value"  gorm:"not null"`
}

type UserGroup struct {
	ID      uint `gorm:"primarykey"`
	UserID  uint `json:"user_id" gorm:"not null;index"`
	GroupID uint `json:"group_id" gorm:"not null;index"`
}
