package dao

import (
	"openvpn-pannel/internal/models"
)

// CountCertificatesByType 统计指定用途的证书数量。
func (um *DaoManager) CountCertificatesByType(certType uint) (int64, error) {
	var count int64
	err := um.DB.Model(&models.Certificate{}).Where("type = ?", certType).Count(&count).Error
	return count, err
}

// CountServerEvents 统计事件总数。
func (um *DaoManager) CountServerEvents() (int64, error) {
	var count int64
	err := um.DB.Model(&models.ServerEvent{}).Count(&count).Error
	return count, err
}

// ListRecentServerEvents 返回最近的事件（按时间倒序）。
func (um *DaoManager) ListRecentServerEvents(limit int) ([]*models.ServerEvent, error) {
	if limit <= 0 {
		limit = 10
	}
	var list []*models.ServerEvent
	err := um.DB.Order("event_time desc, id desc").Limit(limit).Find(&list).Error
	return list, err
}

// CountServers 统计服务器总数。
func (um *DaoManager) CountServers() (int64, error) {
	var count int64
	err := um.DB.Model(&models.Server{}).Count(&count).Error
	return count, err
}

// CountConnectedClientInfoRecords 统计当前在线客户端记录数。
func (um *DaoManager) CountConnectedClientInfoRecords() (int64, error) {
	var count int64
	err := um.DB.Model(&models.ConnectedClientInfoRecord{}).Count(&count).Error
	return count, err
}
