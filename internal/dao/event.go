package dao

import (
	"openvpn-pannel/internal/models"
	"time"
)

// 创建服务器事件
func (um *DaoManager) CreateEventRaw(event *models.ServerEvent) error {
	//event.EventTime = uint64(time.Now().Unix())
	return um.DB.Create(event).Error
}
func (um *DaoManager) CreateEvent(serverID uint, eventType int, realIPAddr string, eventData string) error {
	event := &models.ServerEvent{
		EventTime:  uint64(time.Now().Unix()),
		EventType:  eventType,
		RealIPAddr: realIPAddr,
		EventData:  eventData,
		ServerID:   serverID,
	}
	return um.DB.Create(event).Error
}

type EventListResponse struct {
	EventList []*models.ServerEvent `json:"event_list"`
	Total     int64                 `json:"total"`
}

func (um *DaoManager) GetEventList(serverID uint, eventTypeList []int, query string, startTime int64, endTime int64, page int, pageSize int) (*EventListResponse, error) {
	if page <= 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	var eventList []*models.ServerEvent
	response := &EventListResponse{
		EventList: eventList,
		Total:     0,
	}
	var err error
	if len(eventTypeList) == 0 && query == "" && startTime == 0 && endTime == 0 {
		err = um.DB.Where("server_id = ?", serverID).Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ?", serverID).Model(&models.ServerEvent{}).Count(&response.Total)
	} else if len(eventTypeList) != 0 && query == "" && startTime == 0 && endTime == 0 { // eventTypeList
		err = um.DB.Where("server_id = ? and event_type in (?)", serverID, eventTypeList).Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ? and event_type in (?)", serverID, eventTypeList).Model(&models.ServerEvent{}).Count(&response.Total)
	} else if len(eventTypeList) == 0 && query != "" && startTime == 0 && endTime == 0 { // query
		// ipAddr 用模糊匹配
		err = um.DB.Where("server_id = ? and (real_ip_addr like ? or event_data like ?)", serverID, "%"+query+"%", "%"+query+"%").Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ? and (real_ip_addr like ? or event_data like ?)", serverID, "%"+query+"%", "%"+query+"%").Model(&models.ServerEvent{}).Count(&response.Total)
	} else if len(eventTypeList) != 0 && query != "" && startTime == 0 && endTime == 0 { // eventTypeList ipAddr
		// ipAddr 用模糊匹配
		err = um.DB.Where("server_id = ? and event_type in (?) and  (real_ip_addr like ? or event_data like ?)", serverID, eventTypeList, "%"+query+"%", "%"+query+"%").Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ? and event_type in (?) and  (real_ip_addr like ? or event_data like ?)", serverID, eventTypeList, "%"+query+"%", "%"+query+"%").Model(&models.ServerEvent{}).Count(&response.Total)
	} else if len(eventTypeList) == 0 && query == "" && startTime != 0 && endTime != 0 { // startTime endTime
		err = um.DB.Where("server_id = ? and event_time >= ? and event_time <= ?", serverID, startTime, endTime).Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ? and event_time >= ? and event_time <= ?", serverID, startTime, endTime).Model(&models.ServerEvent{}).Count(&response.Total)
	} else if len(eventTypeList) != 0 && query == "" && startTime != 0 && endTime != 0 { // eventTypeList startTime endTime
		err = um.DB.Where("server_id = ? and event_type in (?) and event_time >= ? and event_time <= ?", serverID, eventTypeList, startTime, endTime).Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ? and event_type in (?) and event_time >= ? and event_time <= ?", serverID, eventTypeList, startTime, endTime).Model(&models.ServerEvent{}).Count(&response.Total)
	} else if len(eventTypeList) == 0 && query != "" && startTime != 0 && endTime != 0 { // ipAddr startTime endTime
		// ipAddr 用模糊匹配
		err = um.DB.Where("server_id = ? and  (real_ip_addr like ? or event_data like ?) and event_time >= ? and event_time <= ?", serverID, "%"+query+"%", "%"+query+"%", startTime, endTime).Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ? and  (real_ip_addr like ? or event_data like ?) and event_time >= ? and event_time <= ?", serverID, "%"+query+"%", "%"+query+"%", startTime, endTime).Model(&models.ServerEvent{}).Count(&response.Total)
	} else if len(eventTypeList) != 0 && query != "" && startTime != 0 && endTime != 0 { // eventTypeList ipAddr startTime endTime
		// ipAddr 用模糊匹配
		err = um.DB.Where("server_id = ? and event_type in (?) and  (real_ip_addr like ? or event_data like ?) and event_time >= ? and event_time <= ?", serverID, eventTypeList, "%"+query+"%", "%"+query+"%", startTime, endTime).Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&eventList).Error
		um.DB.Where("server_id = ? and event_type in (?) and  (real_ip_addr like ? or event_data like ?) and event_time >= ? and event_time <= ?", serverID, eventTypeList, "%"+query+"%", "%"+query+"%", startTime, endTime).Model(&models.ServerEvent{}).Count(&response.Total)
	}
	if err != nil {
		return nil, err
	}
	response.EventList = eventList
	return response, nil
}

// 清空日志
func (um *DaoManager) ClearEvent(serverID uint) error {
	return um.DB.Where("server_id = ?", serverID).Delete(&models.ServerEvent{}).Error
}
