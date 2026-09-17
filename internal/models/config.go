package models

// 保存配置文件模板和一些资源配置
// 目前id有下面几种：
// config 服务器配置文件模板
// auth.sh 认证脚本
// client_offline.sh 用户下线脚本
// client_online.sh 用户上线脚本
type AppResourceRecord struct {
	ID      string `gorm:"uniqueIndex;not null;type:varchar(50)" json:"id" binding:"required,max=30"`
	Content string `gorm:"not null" json:"content" binding:"required,max=65535"`
}
