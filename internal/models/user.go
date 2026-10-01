package models

import (
	"errors"
	"time"
)

// 用户不可认证/在线的原因。有效期到期与手动禁用都会导致用户无法通过认证，
// 在线会话也会被状态采集线程踢下线。
var (
	// ErrUserDisabled 账号已被管理员禁用。
	ErrUserDisabled = errors.New("账号已被禁用")
	// ErrUserExpired 账号已超过有效期。
	ErrUserExpired = errors.New("账号已过期")
)

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

// 多因素认证（MFA）类型。
// 0 表示未启用；当前仅实现 TOTP，未来可扩展手机号、邮箱等类型。
const (
	MFA_TYPE_NONE = 0
	MFA_TYPE_TOTP = 1
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
	// MFAType 用户启用的多因素认证类型，0=未启用；当前支持 1=TOTP。
	// 取值见 MFA_TYPE_* 常量，便于未来扩展手机号/邮箱等认证方式。
	MFAType uint `gorm:"not null;default:0" json:"mfa_type"`
	// MFAData 对应 MFAType 的认证数据（如 TOTP 的 Base32 密钥）。MFAType=0 时为空，
	// 不通过接口明文返回。
	MFAData string `gorm:"not null;default:''" json:"-"`
	// ExpireAt 用户有效期截止时间(unix 秒)，0=永不过期。到期后无法通过认证，
	// 已在线会话会被状态采集线程自动踢下线。
	ExpireAt uint64 `gorm:"not null;default:0" json:"expire_at"`
	// Disabled 是否被管理员禁用。禁用后无法通过认证，已在线会话会被自动踢下线。
	Disabled bool `gorm:"not null;default:false" json:"disabled"`
}

// CheckAvailable 判断用户当前是否允许认证与保持在线：
// 未被禁用且未超过有效期时返回 nil，否则返回具体原因（ErrUserDisabled / ErrUserExpired）。
// now 方便调用方在批量检查时复用同一时间点，也便于测试。
func (u *User) CheckAvailable(now time.Time) error {
	if u == nil {
		return errors.New("用户不存在")
	}
	if u.Disabled {
		return ErrUserDisabled
	}
	if u.ExpireAt > 0 && uint64(now.Unix()) >= u.ExpireAt {
		return ErrUserExpired
	}
	return nil
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
