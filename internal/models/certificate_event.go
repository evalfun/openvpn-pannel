package models

// 证书事件类型（独立于服务器事件，存放于 certificate_events 表）
const (
	CERT_EVENT_TYPE_CREATE           = 1 // 生成 CA
	CERT_EVENT_TYPE_IMPORT           = 2 // 导入证书
	CERT_EVENT_TYPE_SIGN             = 3 // 签发证书
	CERT_EVENT_TYPE_DELETE           = 4 // 删除证书
	CERT_EVENT_TYPE_DOWNLOAD_CERT    = 5 // 下载证书
	CERT_EVENT_TYPE_DOWNLOAD_KEY     = 6 // 下载私钥
	CERT_EVENT_TYPE_SERVER_REFERENCE = 7 // 服务器引用证书
	CERT_EVENT_TYPE_CLIENT_REFERENCE = 8 // 导出客户端配置时引用证书
	CERT_EVENT_TYPE_CLEAR            = 9 // 清空证书事件（清空后记录一条，用于审计）
)

// CertificateEvent 证书操作审计事件（独立表，不属于任何服务器）。
type CertificateEvent struct {
	ID         uint   `gorm:"primarykey" json:"id"`
	EventType  int    `gorm:"not null;index" json:"event_type"`
	EventTime  uint64 `gorm:"not null;index" json:"event_time"`
	RealIPAddr string `gorm:"not null" json:"real_ip_addr"`
	EventData  string `gorm:"not null" json:"event_data"`
	CertID     uint   `gorm:"not null;default:0;index" json:"cert_id"`
	CertName   string `gorm:"not null;default:''" json:"cert_name"`
	ServerID   uint   `gorm:"not null;default:0;index" json:"server_id"`
	ServerName string `gorm:"not null;default:''" json:"server_name"`
	// OperatorUserID/OperatorUsername 记录执行该操作的面板用户（审计用）。
	OperatorUserID   uint   `gorm:"not null;default:0;index" json:"operator_user_id"`
	OperatorUsername string `gorm:"not null;default:''" json:"operator_username"`
}
