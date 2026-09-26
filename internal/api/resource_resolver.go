package api

import (
	"log"

	ovpnserver "openvpn-pannel/internal/ovpn_server"
)

// defaultResourceSetID 内置默认资源集（普通 Linux 发行版 - iptables）。
const defaultResourceSetID = ovpnserver.RESOURCE_SET_LINUX_IPTABLES

// GetActiveResourceSetID 返回当前启用的资源集 ID。读取失败或值非法时回落到默认资源集。
func (a *App) GetActiveResourceSetID() string {
	setID, err := a.daoManager.GetActiveResourceSetID(defaultResourceSetID)
	if err != nil {
		// 无记录属正常（首次运行），不打印为错误。
		if err.Error() != "record not found" {
			log.Printf("读取当前资源集失败，使用默认资源集: %v", err)
		}
		return defaultResourceSetID
	}
	if !ovpnserver.IsValidResourceSet(setID) {
		log.Printf("当前资源集 %s 非法，使用默认资源集", setID)
		return defaultResourceSetID
	}
	return setID
}

// EnsureResourceSetSeeded 确保指定资源集在数据库中已完成种子化：
// 若该资源集一条记录都没有，则把内置默认内容批量写入，形成“可编辑的工作区”。
func (a *App) EnsureResourceSetSeeded(setID string) error {
	count, err := a.daoManager.CountResourceBySet(setID)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, item := range ovpnserver.ListResourceIDs() {
		content := ovpnserver.GetSetDefaultResource(setID, item.ID)
		if content == "" {
			continue
		}
		if err := a.daoManager.WriteResourceBySet(setID, item.ID, content); err != nil {
			return err
		}
	}
	log.Printf("资源集 %s 已从内置默认种子化", setID)
	return nil
}

// GetResourceContent 读取“指定资源集内”的某资源：优先数据库，缺失时回落该资源集内置默认。
func (a *App) GetResourceContent(setID, resourceID string) string {
	resourceModel, err := a.daoManager.GetResourceBySet(setID, resourceID)
	if err == nil {
		return resourceModel.Content
	}
	return ovpnserver.GetSetDefaultResource(setID, resourceID)
}

// GetActiveResourceContent 读取“当前启用资源集内”的某资源。
func (a *App) GetActiveResourceContent(resourceID string) string {
	return a.GetResourceContent(a.GetActiveResourceSetID(), resourceID)
}

// PrepareResourceMap 按当前启用的资源集组装资源映射：优先数据库，缺失项回落该资源集内置默认。
func (a *App) PrepareResourceMap(ResourceIDList []string) map[string]string {
	resourceMap := make(map[string]string)
	setID := a.GetActiveResourceSetID()
	// 首次使用某资源集时先完成种子化，保证后续读取与编辑都能落库。
	if err := a.EnsureResourceSetSeeded(setID); err != nil {
		log.Printf("资源集 %s 种子化失败: %v", setID, err)
	}
	resourceModelList, err := a.daoManager.GetResourceByIDListInSet(setID, ResourceIDList)
	if err == nil {
		for _, resourceModel := range resourceModelList {
			resourceMap[resourceModel.ID] = resourceModel.Content
		}
	}
	// 数据库缺失的资源回落到该资源集内置默认。
	for _, id := range ResourceIDList {
		if _, ok := resourceMap[id]; !ok {
			resourceMap[id] = ovpnserver.GetSetDefaultResource(setID, id)
		}
	}
	return resourceMap
}
