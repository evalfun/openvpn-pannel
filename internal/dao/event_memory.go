package dao

import (
	"strings"
	"sync"

	"openvpn-pannel/internal/models"
)

// eventMemoryStore 内存事件存储：在 config.max_memory_events > 0 时启用，
// 把服务器事件与证书事件保存在内存中（最多保留 max 条最新记录），避免频繁写入
// 数据库而磨损嵌入式设备的闪存。进程重启后内存事件丢失。
//
// 查询语义尽量与数据库版本保持一致：按 id 倒序、支持类型/关键字/时间范围过滤与分页。
type eventMemoryStore struct {
	mu  sync.RWMutex
	max int

	serverEvents []*models.ServerEvent
	certEvents   []*models.CertificateEvent

	// nextServerID / nextCertID 为内存事件分配自增 ID（与数据库自增 ID 语义一致，仅用于排序与前端 key）。
	nextServerID uint
	nextCertID   uint
}

func newEventMemoryStore(max int) *eventMemoryStore {
	return &eventMemoryStore{
		max:          max,
		serverEvents: make([]*models.ServerEvent, 0, max),
		certEvents:   make([]*models.CertificateEvent, 0, max),
	}
}

// appendServerEvent 追加一条服务器事件并裁剪到上限，返回带 ID 的副本。
func (s *eventMemoryStore) appendServerEvent(event *models.ServerEvent) *models.ServerEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextServerID++
	stored := *event
	stored.ID = s.nextServerID
	s.serverEvents = append(s.serverEvents, &stored)
	if len(s.serverEvents) > s.max {
		// 丢弃最旧的记录（切片前部）。
		overflow := len(s.serverEvents) - s.max
		s.serverEvents = append([]*models.ServerEvent(nil), s.serverEvents[overflow:]...)
	}
	return &stored
}

// appendCertEvent 追加一条证书事件并裁剪到上限，返回带 ID 的副本。
func (s *eventMemoryStore) appendCertEvent(event *models.CertificateEvent) *models.CertificateEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextCertID++
	stored := *event
	stored.ID = s.nextCertID
	s.certEvents = append(s.certEvents, &stored)
	if len(s.certEvents) > s.max {
		overflow := len(s.certEvents) - s.max
		s.certEvents = append([]*models.CertificateEvent(nil), s.certEvents[overflow:]...)
	}
	return &stored
}

// matchServerEvent 判断服务器事件是否满足过滤条件（与数据库查询保持一致）。
func matchServerEvent(e *models.ServerEvent, serverID uint, eventTypeList []int, query string, startTime, endTime int64) bool {
	if e.ServerID != serverID {
		return false
	}
	if len(eventTypeList) != 0 {
		found := false
		for _, t := range eventTypeList {
			if e.EventType == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if query != "" && !strings.Contains(e.RealIPAddr, query) && !strings.Contains(e.EventData, query) {
		return false
	}
	if startTime != 0 && endTime != 0 {
		if int64(e.EventTime) < startTime || int64(e.EventTime) > endTime {
			return false
		}
	}
	return true
}

// listServerEvents 按 id 倒序返回过滤后的分页结果与总数。
func (s *eventMemoryStore) listServerEvents(serverID uint, eventTypeList []int, query string, startTime, endTime int64, page, pageSize int) ([]*models.ServerEvent, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*models.ServerEvent, 0)
	// 从新到旧遍历，天然得到 id 倒序。
	for i := len(s.serverEvents) - 1; i >= 0; i-- {
		e := s.serverEvents[i]
		if matchServerEvent(e, serverID, eventTypeList, query, startTime, endTime) {
			matched = append(matched, e)
		}
	}
	total := int64(len(matched))

	start := (page - 1) * pageSize
	if start >= len(matched) {
		return []*models.ServerEvent{}, total
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	// 返回副本切片，避免调用方在锁外读到被裁剪/替换的元素。
	result := make([]*models.ServerEvent, 0, end-start)
	for _, e := range matched[start:end] {
		cp := *e
		result = append(result, &cp)
	}
	return result, total
}

// recentServerEvents 返回最近 limit 条服务器事件（按 id 倒序）。
func (s *eventMemoryStore) recentServerEvents(limit int) []*models.ServerEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}
	if limit > len(s.serverEvents) {
		limit = len(s.serverEvents)
	}
	result := make([]*models.ServerEvent, 0, limit)
	for i := len(s.serverEvents) - 1; i >= 0 && len(result) < limit; i-- {
		cp := *s.serverEvents[i]
		result = append(result, &cp)
	}
	return result
}

// countServerEvents 返回服务器事件总数。
func (s *eventMemoryStore) countServerEvents() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.serverEvents))
}

// clearServerEvents 清空指定服务器的服务器事件。
func (s *eventMemoryStore) clearServerEvents(serverID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]*models.ServerEvent, 0, len(s.serverEvents))
	for _, e := range s.serverEvents {
		if e.ServerID != serverID {
			kept = append(kept, e)
		}
	}
	s.serverEvents = kept
}

// matchCertEvent 判断证书事件是否满足过滤条件。
func matchCertEvent(e *models.CertificateEvent, eventTypeList []int, query string, startTime, endTime int64) bool {
	if len(eventTypeList) != 0 {
		found := false
		for _, t := range eventTypeList {
			if e.EventType == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if query != "" {
		if !strings.Contains(e.RealIPAddr, query) &&
			!strings.Contains(e.EventData, query) &&
			!strings.Contains(e.CertName, query) &&
			!strings.Contains(e.ServerName, query) &&
			!strings.Contains(e.OperatorUsername, query) {
			return false
		}
	}
	if startTime != 0 && endTime != 0 {
		if int64(e.EventTime) < startTime || int64(e.EventTime) > endTime {
			return false
		}
	}
	return true
}

// listCertEvents 按 id 倒序返回过滤后的分页结果与总数。
func (s *eventMemoryStore) listCertEvents(eventTypeList []int, query string, startTime, endTime int64, page, pageSize int) ([]*models.CertificateEvent, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*models.CertificateEvent, 0)
	for i := len(s.certEvents) - 1; i >= 0; i-- {
		e := s.certEvents[i]
		if matchCertEvent(e, eventTypeList, query, startTime, endTime) {
			matched = append(matched, e)
		}
	}
	total := int64(len(matched))

	start := (page - 1) * pageSize
	if start >= len(matched) {
		return []*models.CertificateEvent{}, total
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	result := make([]*models.CertificateEvent, 0, end-start)
	for _, e := range matched[start:end] {
		cp := *e
		result = append(result, &cp)
	}
	return result, total
}

// clearCertEvents 清空全部证书事件。
func (s *eventMemoryStore) clearCertEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.certEvents = s.certEvents[:0]
}
