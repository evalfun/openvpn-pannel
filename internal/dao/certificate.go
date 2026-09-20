package dao

import (
	"fmt"
	"openvpn-pannel/internal/models"
)

func (um *DaoManager) CreateCertificate(cert *models.Certificate) error {
	err := um.DB.Create(cert).Error
	if err != nil {
		return fmt.Errorf("创建证书失败: %s", err.Error())
	}
	return nil
}

func (um *DaoManager) UpdateCertificate(cert *models.Certificate) error {
	err := um.DB.Save(cert).Error
	if err != nil {
		return fmt.Errorf("更新证书失败: %s", err.Error())
	}
	return nil
}

func (um *DaoManager) GetCertificateByID(id uint) (*models.Certificate, error) {
	cert := models.Certificate{ID: id}
	err := um.DB.First(&cert).Error
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

func (um *DaoManager) ListCertificateByType(certType uint) ([]*models.Certificate, error) {
	var list []*models.Certificate
	err := um.DB.Where("type = ?", certType).Order("id asc").Find(&list).Error
	return list, err
}

func (um *DaoManager) ListAllCertificate() ([]*models.Certificate, error) {
	var list []*models.Certificate
	err := um.DB.Order("id asc").Find(&list).Error
	return list, err
}

// CertificateListQuery 证书分页查询条件。
type CertificateListQuery struct {
	FilterType   bool
	CertType     uint
	FilterTypes  bool
	CertTypes    []uint
	FilterParent bool
	ParentID     uint
	OnlyWithKey  bool
	Page         int
	PageSize     int
}

// ListCertificateQuery 按条件分页查询证书，同时返回总数。PageSize<=0 表示不分页。
func (um *DaoManager) ListCertificateQuery(q CertificateListQuery) ([]*models.Certificate, int64, error) {
	db := um.DB.Model(&models.Certificate{})
	if q.FilterType {
		db = db.Where("type = ?", q.CertType)
	}
	if q.FilterTypes && len(q.CertTypes) > 0 {
		db = db.Where("type IN ?", q.CertTypes)
	}
	if q.FilterParent {
		db = db.Where("parent_id = ?", q.ParentID)
	}
	if q.OnlyWithKey {
		// key 是 MySQL 保留字，需加引用符
		db = db.Where("`key` <> ''")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	db = db.Order("id asc")
	if q.PageSize > 0 {
		page := q.Page
		if page < 1 {
			page = 1
		}
		db = db.Limit(q.PageSize).Offset((page - 1) * q.PageSize)
	}
	var list []*models.Certificate
	if err := db.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (um *DaoManager) ListChildCertificate(parentID uint) ([]*models.Certificate, error) {
	var list []*models.Certificate
	err := um.DB.Where("parent_id = ?", parentID).Order("id asc").Find(&list).Error
	return list, err
}

func (um *DaoManager) CountChildCertificate(parentID uint) (int64, error) {
	var count int64
	err := um.DB.Model(&models.Certificate{}).Where("parent_id = ?", parentID).Count(&count).Error
	return count, err
}

// CountChildCertificateByType 统计某 CA 下指定类型的子证书数量。
func (um *DaoManager) CountChildCertificateByType(parentID uint, certType uint) (int64, error) {
	var count int64
	err := um.DB.Model(&models.Certificate{}).
		Where("parent_id = ? AND type = ?", parentID, certType).
		Count(&count).Error
	return count, err
}

func (um *DaoManager) DeleteCertificate(id uint) error {
	err := um.DB.Where("id = ?", id).Delete(&models.Certificate{}).Error
	if err != nil {
		return fmt.Errorf("删除证书失败: %s", err.Error())
	}
	return nil
}

// CountServerReferencingCertificate 统计引用指定证书记录的服务器数量。
func (um *DaoManager) CountServerReferencingCertificate(id uint) (int64, error) {
	pattern := fmt.Sprintf("%s%d/%%", models.CERT_REF_PREFIX, id)
	var count int64
	err := um.DB.Model(&models.Server{}).
		Where("`ca` LIKE ? OR `cert` LIKE ? OR `key` LIKE ?", pattern, pattern, pattern).
		Count(&count).Error
	return count, err
}
