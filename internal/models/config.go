package models

// 保存配置文件模板和一些资源配置。
// 资源按“资源集”(set_id) 分组，每个资源集包含全部资源文件：
// 服务器配置模板 config、认证脚本 auth.sh、上线/下线脚本、限速脚本等。
// 复合主键为 (set_id, id)。
type AppResourceRecord struct {
	SetID   string `gorm:"primaryKey;type:varchar(50);not null" json:"set_id" binding:"required,max=50"`
	ID      string `gorm:"primaryKey;type:varchar(50);not null" json:"id" binding:"required,max=30"`
	Content string `gorm:"not null" json:"content" binding:"required,max=65535"`
}

// AppResourceSetState 记录当前启用的资源集（单行表，固定 ID=1）。
type AppResourceSetState struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	ActiveSetID string `gorm:"type:varchar(50);not null" json:"active_set_id"`
}
