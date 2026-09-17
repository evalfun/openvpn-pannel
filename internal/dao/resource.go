package dao

import (
	"fmt"
	"openvpn-pannel/internal/models"
)

func (um *DaoManager) GetResourceByIDList(resourceIDList []string) ([]*models.AppResourceRecord, error) {
	var resourceModelList []*models.AppResourceRecord
	err := um.DB.Where("id in (?)", resourceIDList).Find(&resourceModelList).Error
	return resourceModelList, err
}
func (um *DaoManager) GetResourceByID(resourceID string) (*models.AppResourceRecord, error) {
	resourceModel := models.AppResourceRecord{
		ID: resourceID,
	}
	err := um.DB.First(&resourceModel).Error
	if err != nil {
		return nil, err
	}
	return &resourceModel, nil
}

func (um *DaoManager) WriteResource(resourceID string, content string) error {

	err := um.DB.Where("id = ?", resourceID).Delete(&models.AppResourceRecord{}).Error

	resourceModel := models.AppResourceRecord{
		ID:      resourceID,
		Content: content,
	}
	fmt.Println(resourceID)
	err = um.DB.Create(resourceModel).Error
	if err != nil {
		return fmt.Errorf("写入新数据失败 %s", err.Error())
	}
	return nil
}

func (um *DaoManager) DeleteResource(resourceID string) error {
	err := um.DB.Where("id = ?", resourceID).Delete(&models.AppResourceRecord{}).Error
	return err
}
