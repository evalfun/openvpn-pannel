package dao

import (
	"errors"

	"gorm.io/gorm"

	"openvpn-pannel/internal/models"
)

// SaveServerProcess 写入/更新服务器的 OpenVPN 进程 PID 记录。
func (um *DaoManager) SaveServerProcess(serverID uint, pid int) error {
	var record models.ServerProcessRecord
	err := um.DB.Where("server_id = ?", serverID).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return um.DB.Create(&models.ServerProcessRecord{ServerID: serverID, PID: pid}).Error
		}
		return err
	}
	record.PID = pid
	return um.DB.Save(&record).Error
}

// DeleteServerProcess 删除服务器的进程 PID 记录。
func (um *DaoManager) DeleteServerProcess(serverID uint) error {
	return um.DB.Where("server_id = ?", serverID).Delete(&models.ServerProcessRecord{}).Error
}

// DeleteAllServerProcess 删除所有进程 PID 记录（退出时会停止实例的场景下清理残留）。
func (um *DaoManager) DeleteAllServerProcess() error {
	return um.DB.Where("1 = 1").Delete(&models.ServerProcessRecord{}).Error
}

// ListServerProcessRecords 列出所有进程 PID 记录。
func (um *DaoManager) ListServerProcessRecords() ([]*models.ServerProcessRecord, error) {
	var records []*models.ServerProcessRecord
	err := um.DB.Find(&records).Error
	return records, err
}
