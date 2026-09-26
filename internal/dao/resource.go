package dao

import (
	"fmt"
	"openvpn-pannel/internal/models"
)

// ===== 资源集状态 =====

// GetActiveResourceSetID 返回当前启用的资源集 ID；无记录时返回 fallback。
func (um *DaoManager) GetActiveResourceSetID(fallback string) (string, error) {
	var state models.AppResourceSetState
	err := um.DB.First(&state, 1).Error
	if err != nil {
		return fallback, err
	}
	if state.ActiveSetID == "" {
		return fallback, nil
	}
	return state.ActiveSetID, nil
}

// SetActiveResourceSetID 写入当前启用的资源集 ID（单行，固定 ID=1）。
func (um *DaoManager) SetActiveResourceSetID(setID string) error {
	state := models.AppResourceSetState{ID: 1, ActiveSetID: setID}
	// 覆盖写入：先删后建，避免依赖 upsert 语义差异。
	if err := um.DB.Where("id = ?", 1).Delete(&models.AppResourceSetState{}).Error; err != nil {
		return err
	}
	if err := um.DB.Create(&state).Error; err != nil {
		return fmt.Errorf("写入当前资源集失败 %s", err.Error())
	}
	return nil
}

// ===== 资源集内容 =====

// GetResourceBySet 读取指定资源集内的某个资源；不存在返回错误。
func (um *DaoManager) GetResourceBySet(setID, resourceID string) (*models.AppResourceRecord, error) {
	resourceModel := models.AppResourceRecord{}
	err := um.DB.Where("set_id = ? AND id = ?", setID, resourceID).First(&resourceModel).Error
	if err != nil {
		return nil, err
	}
	return &resourceModel, nil
}

// GetResourceByIDListInSet 读取指定资源集内的多个资源。
func (um *DaoManager) GetResourceByIDListInSet(setID string, resourceIDList []string) ([]*models.AppResourceRecord, error) {
	var resourceModelList []*models.AppResourceRecord
	err := um.DB.Where("set_id = ? AND id in (?)", setID, resourceIDList).Find(&resourceModelList).Error
	return resourceModelList, err
}

// CountResourceBySet 统计指定资源集内的资源数量（用于判断是否需要 seed）。
func (um *DaoManager) CountResourceBySet(setID string) (int64, error) {
	var count int64
	err := um.DB.Model(&models.AppResourceRecord{}).Where("set_id = ?", setID).Count(&count).Error
	return count, err
}

// WriteResourceBySet 写入指定资源集内的某个资源（覆盖）。
func (um *DaoManager) WriteResourceBySet(setID, resourceID, content string) error {
	if err := um.DB.Where("set_id = ? AND id = ?", setID, resourceID).Delete(&models.AppResourceRecord{}).Error; err != nil {
		return err
	}
	resourceModel := models.AppResourceRecord{
		SetID:   setID,
		ID:      resourceID,
		Content: content,
	}
	if err := um.DB.Create(&resourceModel).Error; err != nil {
		return fmt.Errorf("写入新数据失败 %s", err.Error())
	}
	return nil
}

// DeleteResourceBySet 删除指定资源集内的某个资源（删除后回落到该资源集内置默认）。
func (um *DaoManager) DeleteResourceBySet(setID, resourceID string) error {
	return um.DB.Where("set_id = ? AND id = ?", setID, resourceID).Delete(&models.AppResourceRecord{}).Error
}
