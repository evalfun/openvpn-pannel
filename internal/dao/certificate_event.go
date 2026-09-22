package dao

import (
	"openvpn-pannel/internal/models"

	"gorm.io/gorm"
)

type CertificateEventListResponse struct {
	EventList []*models.CertificateEvent `json:"event_list"`
	Total     int64                      `json:"total"`
}

// CreateCertificateEvent 记录一条证书事件。
func (um *DaoManager) CreateCertificateEvent(event *models.CertificateEvent) error {
	return um.DB.Create(event).Error
}

// GetCertificateEventList 查询证书事件（支持类型、关键字、时间范围与分页）。
func (um *DaoManager) GetCertificateEventList(eventTypeList []int, query string, startTime, endTime int64, page, pageSize int) (*CertificateEventListResponse, error) {
	if page <= 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	buildQuery := func() *gorm.DB {
		db := um.DB.Model(&models.CertificateEvent{})
		if len(eventTypeList) != 0 {
			db = db.Where("event_type in ?", eventTypeList)
		}
		if query != "" {
			like := "%" + query + "%"
			db = db.Where("(real_ip_addr like ? or event_data like ? or cert_name like ? or server_name like ? or operator_username like ?)", like, like, like, like, like)
		}
		if startTime != 0 && endTime != 0 {
			db = db.Where("event_time >= ? and event_time <= ?", startTime, endTime)
		}
		return db
	}

	response := &CertificateEventListResponse{Total: 0}
	if err := buildQuery().Count(&response.Total).Error; err != nil {
		return nil, err
	}
	var eventList []*models.CertificateEvent
	if err := buildQuery().Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error; err != nil {
		return nil, err
	}
	response.EventList = eventList
	return response, nil
}

// ClearCertificateEvent 清空所有证书事件。
func (um *DaoManager) ClearCertificateEvent() error {
	return um.DB.Where("1 = 1").Delete(&models.CertificateEvent{}).Error
}
