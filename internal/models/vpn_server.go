package models

type Server struct {
	ID          uint   `gorm:"primarykey" json:"id"`
	Name        string `gorm:"unique;not null" json:"name"`
	Local       string `gorm:"not null" json:"local"`
	Port        uint32 `gorm:"not null" json:"port"`
	Proto       string `gorm:"not null" json:"proto"`
	Dev         string `gorm:"not null" json:"dev"`
	CA          string `gorm:"not null" json:"ca"`
	Cert        string `gorm:"not null" json:"cert"`
	Key         string `gorm:"not null" json:"key"`
	DH          string `gorm:"not null" json:"dh"`
	DataCipher  string `gorm:"not null" json:"data_cipher"`
	Topology    string `gorm:"not null" json:"topology"`
	ServerCIDR  string `gorm:"not null" json:"server_cidr"`
	DuplicateCN bool   `gorm:"not null" json:"duplicate_cn"`
	Keepalive   string `gorm:"not null" json:"keepalive"`
	TLSAuthKey  string `gorm:"not null" json:"tls_auth_key"`
	OtherConfig string `gorm:"not null" json:"other_config"`
	AutoStart   bool   `gorm:"not null" json:"auto_start"`
	// 客户端配置导出时使用的服务器地址与端口，按服务器实例保存，下次导出自动回填。
	ExportHost string `gorm:"not null;default:''" json:"export_host"`
	ExportPort uint32 `gorm:"not null;default:0" json:"export_port"`
	// 客户端附加配置：导出时追加到 .ovpn 末尾，按服务器实例保存，下次导出自动回填。
	ExportExtraConfig string `gorm:"not null;default:''" json:"export_extra_config"`
}

type ServerRoute struct {
	ID       uint   `gorm:"primarykey" json:"id"`
	ServerID uint   `gorm:"not null" json:"server_id"`
	Network  string `gorm:"not null" json:"network"`
}

type ClientConfig struct {
	ID             uint   `gorm:"primarykey" json:"id"`
	ServerID       uint   `gorm:"not null" json:"server_id"`
	ClientCertName string `gorm:"not null" json:"client_cert_name"`
	Config         string `gorm:"not null" json:"config"`
}

/*
客户端配置文件
route 10.9.0.0 255.255.255.252
learn-address ./script
push "redirect-gateway def1 bypass-dhcp"
push "dhcp-option DNS 208.67.222.222"
push "dhcp-option DNS 208.67.220.220"
*/

const (
	SERVER_PERM_OBJ_TYPE_USER  = 1
	SERVER_PERM_OBJ_TYPE_GROUP = 2
	SERVER_PERM_ACTION_PERMIT  = 1
	SERVER_PERM_ACTION_DENY    = 0
)

type ServerPermission struct {
	ID       uint `gorm:"primarykey" json:"id"`
	ServerID uint `gorm:"not null;index" json:"server_id" binding:"required"`
	ObjType  uint `gorm:"not null" json:"obj_type" binding:"required"`
	ObjID    uint `gorm:"not null" json:"obj_id" binding:"required"`
	Action   uint `gorm:"not null" json:"action"`
}

const (
	SERVER_EVENT_TYPE_CLIENT_ONLINE        = 1
	SERVER_EVENT_TYPE_CLIENT_OFFLINE       = 2
	SERVER_EVENT_TYPE_CLIENT_AUTH_FAIL     = 3
	SERVER_EVENT_TYPE_CLIENT_AUTH_SUCCESS  = 4
	SERVER_EVENT_TYPE_SERVER_START_SUCCESS = 5
	SERVER_EVENT_TYPE_SERVER_EXIT_SUCCESS  = 6
	SERVER_EVENT_TYPE_SERVER_START_FAIL    = 7
	SERVER_EVENT_TYPE_SERVER_EXIT_FAIL     = 8

	SERVER_EVENT_TYPE_ADD_ACL = 9
	SERVER_EVENT_TYPE_DEL_ACL = 10
)

type ServerEvent struct {
	ID         uint   `gorm:"primarykey" json:"id"`
	ServerID   uint   `gorm:"not null;index" json:"server_id"`
	EventType  int    `gorm:"not null" json:"event_type"`
	EventTime  uint64 `gorm:"not null" json:"event_time"`
	RealIPAddr string `gorm:"not null" json:"real_ip_addr"`
	EventData  string `gorm:"not null" json:"event_data"`
}

type AddedServerACLRecord struct {
	ID            uint   `gorm:"primarykey" json:"id"`
	ServerID      uint   `gorm:"not null;index" json:"server_id"`
	VirtualIPAddr string `gorm:"not null;index" json:"real_ip_addr"`
	ACLType       uint   `gorm:"not null" json:"acl_type"`
	ACLValue      string `gorm:"not null" json:"acl_value"`
}
type ConnectedClientInfoRecord struct {
	VirtualIPAddr string `gorm:"not null;index" json:"virtual_ip_addr"`
	ServerID      uint   `gorm:"not null;index" json:"server_id"`
	Username      string `gorm:"not null" json:"username"`
	ByteReceived  uint64 `gorm:"not null;default:0" json:"byte_received"`
	ByteSent      uint64 `gorm:"not null;default:0" json:"byte_sent"`
	// UploadLimitKB/DownloadLimitKB 记录该会话最近一次实际下发的限速(KB/s)，0=不限速。
	// 达量限速运行时据此判断生效限速是否变化，只对变化的客户端重新设置 tc。
	UploadLimitKB   uint64 `gorm:"not null;default:0" json:"upload_limit_kb"`
	DownloadLimitKB uint64 `gorm:"not null;default:0" json:"download_limit_kb"`
}
