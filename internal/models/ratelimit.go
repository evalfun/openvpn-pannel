package models

// 达量限速方案：以一个周期为单位统计用户流量，周期结束重置周期内流量统计。
// 一个用户最多关联一个方案（User.RateLimitPlanID），也可以不关联。
type RateLimitPlan struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	Name          string `gorm:"uniqueIndex;not null;type:varchar(100)" json:"name"`
	Description   string `gorm:"not null" json:"description"`
	PeriodSeconds uint64 `gorm:"not null;default:86400" json:"period_seconds"`
	CreatedAt     int64  `gorm:"not null;default:0" json:"created_at"`
}

// 达量限速规则：
//   - Priority 为优先级，数值越大越先匹配；
//   - UploadThresholdBytes/DownloadThresholdBytes 为上传、下载的流量阈值，规则在
//     “上传流量大于该值 或 下载流量大于该值”时触发（任一方面超标即算达到该档）；
//   - LimitUploadKB/LimitDownloadKB 为速度限制(KB/s)，两者都为 0 表示该规则不限速；
//   - AllowConnect 为是否允许连接，false 表示匹配到该规则时直接拒绝认证。
//
// 匹配方式：规则按优先级从大到小排列，取**第一条已触发**的规则；
// 若所有规则都未触发，则不做任何达量限制（默认档）。
type RateLimitRule struct {
	ID                     uint   `gorm:"primarykey" json:"id"`
	PlanID                 uint   `gorm:"not null;index" json:"plan_id"`
	Priority               int    `gorm:"not null;default:0" json:"priority"`
	UploadThresholdBytes   uint64 `gorm:"not null;default:0" json:"upload_threshold_bytes"`
	DownloadThresholdBytes uint64 `gorm:"not null;default:0" json:"download_threshold_bytes"`
	LimitUploadKB          uint64 `gorm:"not null;default:0" json:"limit_upload_kb"`
	LimitDownloadKB        uint64 `gorm:"not null;default:0" json:"limit_download_kb"`
	AllowConnect           bool   `gorm:"not null" json:"allow_connect"`
}

// MatchRateLimitRule 根据已用流量匹配限速规则。
// rules 需要按优先级降序排序。usedUpload/usedDownload 为周期内已用字节数。
// 返回第一条已触发的规则；返回 nil 表示所有规则都未触发（默认不限速且允许连接）。
func MatchRateLimitRule(rules []*RateLimitRule, usedUpload, usedDownload uint64) *RateLimitRule {
	for _, rule := range rules {
		if usedUpload > rule.UploadThresholdBytes || usedDownload > rule.DownloadThresholdBytes {
			return rule
		}
	}
	return nil
}
