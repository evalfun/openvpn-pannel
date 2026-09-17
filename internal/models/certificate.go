package models

import "strings"

const (
	CERT_TYPE_CA     = 1
	CERT_TYPE_SERVER = 2
	CERT_TYPE_CLIENT = 3
)

const (
	CERT_KEY_TYPE_RSA = "rsa"
	CERT_KEY_TYPE_EC  = "ec"
)

// 服务器 CA/证书/私钥字段引用证书存储时的前缀，格式为 cert-stor:<id>/cert 或 cert-stor:<id>/key
const CERT_REF_PREFIX = "cert-stor:"

// Certificate 证书存储记录。
// Type 为 CERT_TYPE_CA 时 ParentID 为 0；服务器/客户端证书的 ParentID 指向签发它的 CA 记录 ID。
type Certificate struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	Name         string `gorm:"not null;index" json:"name"`
	Type         uint   `gorm:"not null;index" json:"type"`
	ParentID     uint   `gorm:"not null;index" json:"parent_id"`
	Cert         string `gorm:"not null;type:text" json:"cert"`
	Key          string `gorm:"not null;type:text" json:"key"`
	KeyType      string `gorm:"not null" json:"key_type"`
	SerialNumber string `gorm:"not null" json:"serial_number"`
	Subject      string `gorm:"not null" json:"subject"`
	Issuer       string `gorm:"not null" json:"issuer"`
	NotBefore    int64  `gorm:"not null" json:"not_before"`
	NotAfter     int64  `gorm:"not null" json:"not_after"`
	Description  string `gorm:"not null" json:"description"`
	CreatedAt    int64  `gorm:"not null" json:"created_at"`
}

func (cert *Certificate) HasKey() bool {
	return strings.TrimSpace(cert.Key) != ""
}
