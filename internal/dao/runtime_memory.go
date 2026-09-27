package dao

import (
	"strconv"
	"sync"
	"time"

	"openvpn-pannel/internal/models"
)

// runtimeMemoryStore 运行时内存存储：在 config.max_memory_events > 0 时启用，
// 把「日常运行会频繁变化、且属于会话态」的数据保存在内存中，避免频繁写库磨损
// 嵌入式设备的闪存。目标是在日常运行（用户上下线、周期采集、达量限速踢下线）中
// 数据库只读，只有显式运维操作（增删用户、改密码、管理服务器/证书/方案/资源等）才写库。
//
// 覆盖范围：
//   - connected_client_info_records（在线会话记录）
//   - added_server_acl_records（已下发的 ACL 记录）
//   - 用户的达量限速周期流量（rate_limit_cycle_*）
//   - 用户的终身总流量（upload_traffic / download_traffic，客户端下线时累加）
//
// 注意：这些数据在进程重启后会丢失（面板重启时因强制 stop_instances_on_exit=true，
// 实例会被停止，客户端重新连接时会重建会话记录与 ACL）。
type runtimeMemoryStore struct {
	mu sync.RWMutex

	// clients 以 "serverID|virtualIPAddr" 为键保存在线会话记录。
	clients map[string]*models.ConnectedClientInfoRecord
	// clientOrder 记录插入顺序，保证遍历顺序稳定（便于测试与状态展示）。
	clientOrder []string

	// acls 以 "serverID|virtualIPAddr" 为键保存该会话已下发的 ACL 记录。
	acls map[string][]*models.AddedServerACLRecord
	// aclOrder 记录插入顺序。
	aclOrder []string

	// cycles 以 userID 为键保存达量限速周期状态。
	cycles map[uint]*cycleState

	// lifetime 以 userID 为键保存用户终身总流量（upload/download 字节）。
	lifetime map[uint]*lifetimeState

	nextACLID uint
}

// cycleState 单个用户的达量限速周期状态。
type cycleState struct {
	start    uint64
	upload   uint64
	download uint64
}

// lifetimeState 单个用户的终身总流量。
type lifetimeState struct {
	upload   uint64
	download uint64
}

func newRuntimeMemoryStore() *runtimeMemoryStore {
	return &runtimeMemoryStore{
		clients:  make(map[string]*models.ConnectedClientInfoRecord),
		acls:     make(map[string][]*models.AddedServerACLRecord),
		cycles:   make(map[uint]*cycleState),
		lifetime: make(map[uint]*lifetimeState),
	}
}

func clientKey(serverID uint, virtualIPAddr string) string {
	return strconv.FormatUint(uint64(serverID), 10) + "|" + virtualIPAddr
}

// ===== 在线会话记录 =====

// upsertClient 写入或覆盖一条在线会话记录。若已存在同 (serverID, virtualIPAddr)
// 的记录则覆盖（内存模式下不关心唯一约束，直接后写覆盖）。
func (s *runtimeMemoryStore) upsertClient(rec *models.ConnectedClientInfoRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := clientKey(rec.ServerID, rec.VirtualIPAddr)
	stored := *rec
	if _, ok := s.clients[key]; !ok {
		s.clientOrder = append(s.clientOrder, key)
	}
	s.clients[key] = &stored
}

func (s *runtimeMemoryStore) deleteClient(serverID uint, virtualIPAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteClientLocked(serverID, virtualIPAddr)
}

func (s *runtimeMemoryStore) deleteClientLocked(serverID uint, virtualIPAddr string) {
	key := clientKey(serverID, virtualIPAddr)
	if _, ok := s.clients[key]; !ok {
		return
	}
	delete(s.clients, key)
	for i, k := range s.clientOrder {
		if k == key {
			s.clientOrder = append(s.clientOrder[:i], s.clientOrder[i+1:]...)
			break
		}
	}
}

func (s *runtimeMemoryStore) deleteClientsByServer(serverID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.clientOrder[:0]
	for _, key := range s.clientOrder {
		if s.clients[key].ServerID == serverID {
			delete(s.clients, key)
			continue
		}
		kept = append(kept, key)
	}
	s.clientOrder = kept
}

// listClients 返回副本切片（可选按 serverID 过滤，serverID=0 表示不过滤）。
func (s *runtimeMemoryStore) listClients(serverID uint) []*models.ConnectedClientInfoRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.ConnectedClientInfoRecord, 0, len(s.clientOrder))
	for _, key := range s.clientOrder {
		rec := s.clients[key]
		if serverID != 0 && rec.ServerID != serverID {
			continue
		}
		cp := *rec
		result = append(result, &cp)
	}
	return result
}

func (s *runtimeMemoryStore) listClientsByVirtualIP(virtualIPAddr string) []*models.ConnectedClientInfoRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.ConnectedClientInfoRecord, 0)
	for _, key := range s.clientOrder {
		rec := s.clients[key]
		if rec.VirtualIPAddr != virtualIPAddr && rec.VirtualIP6Addr != virtualIPAddr {
			continue
		}
		cp := *rec
		result = append(result, &cp)
	}
	return result
}

func (s *runtimeMemoryStore) listClientsByUsername(username string) []*models.ConnectedClientInfoRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.ConnectedClientInfoRecord, 0)
	for _, key := range s.clientOrder {
		rec := s.clients[key]
		if rec.Username != username {
			continue
		}
		cp := *rec
		result = append(result, &cp)
	}
	return result
}

// resetTrafficByUsernames 把指定用户名的在线会话流量清零（供管理端重置流量使用）。
func (s *runtimeMemoryStore) resetTrafficByUsernames(usernames map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range s.clientOrder {
		rec := s.clients[key]
		if !usernames[rec.Username] {
			continue
		}
		rec.ByteReceived = 0
		rec.ByteSent = 0
	}
}

func (s *runtimeMemoryStore) getClient(serverID uint, virtualIPAddr string) (*models.ConnectedClientInfoRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.clients[clientKey(serverID, virtualIPAddr)]
	if !ok {
		return nil, false
	}
	cp := *rec
	return &cp, true
}

func (s *runtimeMemoryStore) countClients() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.clients))
}

// updateClient 对指定会话记录应用 mutate；若记录不存在则静默忽略（与 DB Updates 语义一致）。
func (s *runtimeMemoryStore) updateClient(serverID uint, virtualIPAddr string, mutate func(rec *models.ConnectedClientInfoRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.clients[clientKey(serverID, virtualIPAddr)]
	if !ok {
		return
	}
	mutate(rec)
}

// ===== 已下发 ACL 记录 =====

// saveACL 覆盖写入某 (serverID, virtualIPAddr) 的 ACL 记录集合。
func (s *runtimeMemoryStore) saveACL(serverID uint, virtualIPAddr string, list []*models.AddedServerACLRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := clientKey(serverID, virtualIPAddr)
	copied := make([]*models.AddedServerACLRecord, 0, len(list))
	for _, acl := range list {
		s.nextACLID++
		cp := *acl
		if cp.ID == 0 {
			cp.ID = s.nextACLID
		}
		copied = append(copied, &cp)
	}
	if _, ok := s.acls[key]; !ok {
		s.aclOrder = append(s.aclOrder, key)
	}
	s.acls[key] = copied
}

func (s *runtimeMemoryStore) deleteACL(serverID uint, virtualIPAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := clientKey(serverID, virtualIPAddr)
	if _, ok := s.acls[key]; !ok {
		return
	}
	delete(s.acls, key)
	for i, k := range s.aclOrder {
		if k == key {
			s.aclOrder = append(s.aclOrder[:i], s.aclOrder[i+1:]...)
			break
		}
	}
}

func (s *runtimeMemoryStore) deleteACLByServer(serverID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.aclOrder[:0]
	for _, key := range s.aclOrder {
		recs := s.acls[key]
		if len(recs) > 0 && recs[0].ServerID == serverID {
			delete(s.acls, key)
			continue
		}
		kept = append(kept, key)
	}
	s.aclOrder = kept
}

func (s *runtimeMemoryStore) listACL(serverID uint, virtualIPAddr string) []*models.AddedServerACLRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := clientKey(serverID, virtualIPAddr)
	recs := s.acls[key]
	result := make([]*models.AddedServerACLRecord, 0, len(recs))
	for _, acl := range recs {
		cp := *acl
		result = append(result, &cp)
	}
	return result
}

func (s *runtimeMemoryStore) listACLByServer(serverID uint) []*models.AddedServerACLRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.AddedServerACLRecord, 0)
	for _, key := range s.aclOrder {
		for _, acl := range s.acls[key] {
			if acl.ServerID != serverID {
				continue
			}
			cp := *acl
			result = append(result, &cp)
		}
	}
	return result
}

// ===== 达量限速周期 =====

// cycleOf 返回用户周期的副本；不存在时返回 nil。
func (s *runtimeMemoryStore) cycleOf(userID uint) *cycleState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cycles[userID]
	if !ok {
		return nil
	}
	cp := *c
	return &cp
}

func (s *runtimeMemoryStore) setCycle(userID uint, start, upload, download uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cycles[userID] = &cycleState{start: start, upload: upload, download: download}
}

// addCycle 累加周期流量；若该用户尚无记录则先以当前时间初始化（start=now）。
func (s *runtimeMemoryStore) addCycle(userID uint, uploadBytes, downloadBytes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cycles[userID]
	if !ok {
		c = &cycleState{start: uint64(time.Now().Unix())}
		s.cycles[userID] = c
	}
	c.upload += uploadBytes
	c.download += downloadBytes
}

func (s *runtimeMemoryStore) resetCycle(userID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cycles[userID] = &cycleState{start: uint64(time.Now().Unix())}
}

func (s *runtimeMemoryStore) deleteCycle(userID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cycles, userID)
}

// ===== 用户终身总流量 =====

// lifetimeOf 返回用户终身总流量副本；不存在时返回 nil。
func (s *runtimeMemoryStore) lifetimeOf(userID uint) *lifetimeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.lifetime[userID]
	if !ok {
		return nil
	}
	cp := *l
	return &cp
}

// addLifetime 累加用户终身总流量。
func (s *runtimeMemoryStore) addLifetime(userID uint, uploadBytes, downloadBytes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lifetime[userID]
	if !ok {
		l = &lifetimeState{}
		s.lifetime[userID] = l
	}
	l.upload += uploadBytes
	l.download += downloadBytes
}

// setLifetime 覆盖设置用户终身总流量（供管理端重置流量使用）。
func (s *runtimeMemoryStore) setLifetime(userID uint, uploadBytes, downloadBytes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lifetime[userID] = &lifetimeState{upload: uploadBytes, download: downloadBytes}
}

// resetLifetimeByUserIDs 把指定用户的终身总流量清零（供管理端批量重置使用）。
// 若内存中尚无记录，则以 0 建立记录（覆盖数据库中的旧值）。
func (s *runtimeMemoryStore) resetLifetimeByUserIDs(userIDs []uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range userIDs {
		s.lifetime[id] = &lifetimeState{}
	}
}

// deleteLifetime 删除用户的终身流量内存记录（供删除用户时清理）。
func (s *runtimeMemoryStore) deleteLifetime(userID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.lifetime, userID)
}
