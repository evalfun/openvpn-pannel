package models

import "strings"

const (
	CERT_TYPE_CA     = 1
	CERT_TYPE_SERVER = 2
	CERT_TYPE_CLIENT = 3
	// CERT_TYPE_UNSPECIFIED 表示未能依据扩展密钥用法判定用途的证书（既无 serverAuth 也无 clientAuth，
	// 或同时包含两者）。OpenVPN 的 --remote-cert-tls client|server 依据 RFC 3280 的密钥用法区分用途。
	CERT_TYPE_UNSPECIFIED = 4
)

// CertTypeFromEKU 依据扩展密钥用法判定的角色映射为证书类型。
// roleServer/roleClient 为 true 时对应服务器/客户端；两者相同（同为 true 或同为 false）视为未指定。
func CertTypeFromEKU(hasServerAuth, hasClientAuth bool) uint {
	switch {
	case hasServerAuth && !hasClientAuth:
		return CERT_TYPE_SERVER
	case hasClientAuth && !hasServerAuth:
		return CERT_TYPE_CLIENT
	default:
		return CERT_TYPE_UNSPECIFIED
	}
}

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
