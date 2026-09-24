package models

import "time"

// ServerProcessRecord 记录面板启动的 OpenVPN 实例进程 PID。
//
// 仅在 config.stop_instances_on_exit=false（退出时不停止实例）时写入：
//   - 启动实例成功后写入/更新该服务器的 PID；
//   - 停止实例成功后删除该记录；
//   - 面板启动时依据该记录重新接管仍在运行的进程（见 App.RecoverRunningServers）。
//
// 这样面板自身崩溃被 systemd 等重新拉起时，OpenVPN 子进程不会中断，客户端无感知。
type ServerProcessRecord struct {
	ID        uint `gorm:"primarykey"`
	ServerID  uint `gorm:"uniqueIndex;not null"`
	PID       int  `gorm:"column:pid;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
