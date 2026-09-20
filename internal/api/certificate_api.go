package api

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"openvpn-pannel/internal/certutil"
	"openvpn-pannel/internal/dao"
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"

	"github.com/gin-gonic/gin"
)

// certListItem 列表返回项，不含证书/私钥内容，仅带 has_key 标记。
type certListItem struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Type         uint   `json:"type"`
	ParentID     uint   `json:"parent_id"`
	KeyType      string `json:"key_type"`
	HasKey       bool   `json:"has_key"`
	SerialNumber string `json:"serial_number"`
	Subject      string `json:"subject"`
	Issuer       string `json:"issuer"`
	NotBefore    int64  `json:"not_before"`
	NotAfter     int64  `json:"not_after"`
	Description  string `json:"description"`
	CreatedAt    int64  `json:"created_at"`
}

func toCertListItem(cert *models.Certificate) certListItem {
	return certListItem{
		ID:           cert.ID,
		Name:         cert.Name,
		Type:         cert.Type,
		ParentID:     cert.ParentID,
		KeyType:      cert.KeyType,
		HasKey:       cert.HasKey(),
		SerialNumber: cert.SerialNumber,
		Subject:      cert.Subject,
		Issuer:       cert.Issuer,
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		Description:  cert.Description,
		CreatedAt:    cert.CreatedAt,
	}
}

func (a *App) certPairToModel(name string, certType uint, parentID uint, pair *certutil.CertPair, description string) *models.Certificate {
	return &models.Certificate{
		Name:         name,
		Type:         certType,
		ParentID:     parentID,
		Cert:         pair.CertPEM,
		Key:          pair.KeyPEM,
		KeyType:      pair.KeyType,
		SerialNumber: pair.SerialNumber,
		Subject:      pair.Subject,
		Issuer:       pair.Issuer,
		NotBefore:    pair.NotBefore,
		NotAfter:     pair.NotAfter,
		Description:  description,
		CreatedAt:    time.Now().Unix(),
	}
}

func certKeyOptions(keyType string, rsaBits int, ecCurve string) certutil.KeyOptions {
	return certutil.KeyOptions{KeyType: keyType, RSABits: rsaBits, ECCurve: ecCurve}
}

// ListCertificateHandler 列出证书（支持分页与过滤）。
// 过滤参数：type（单个证书类型）、types（逗号分隔的多个类型）、parent_id（某 CA 的子证书）、has_key=1（仅含私钥）。
// 分页参数：page（从 1 开始，默认 1）、page_size（默认 0 表示不分页，返回全部）。
func (a *App) ListCertificateHandler(c *gin.Context, user *models.User) {
	q := dao.CertificateListQuery{}
	if v := c.Query("type"); v != "" {
		certType, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			c.JSON(400, gin.H{"result": "failed", "error": "参数 type 必须是 int 类型"})
			return
		}
		q.FilterType = true
		q.CertType = uint(certType)
	}
	if v := c.Query("types"); v != "" {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			certType, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				c.JSON(400, gin.H{"result": "failed", "error": "参数 types 必须是逗号分隔的 int 列表"})
				return
			}
			q.CertTypes = append(q.CertTypes, uint(certType))
		}
		q.FilterTypes = len(q.CertTypes) > 0
	}
	if v := c.Query("parent_id"); v != "" {
		pid, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			c.JSON(400, gin.H{"result": "failed", "error": "参数 parent_id 必须是 int 类型"})
			return
		}
		q.FilterParent = true
		q.ParentID = uint(pid)
	}
	if v := c.Query("has_key"); v == "1" || v == "true" {
		q.OnlyWithKey = true
	}
	q.Page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	q.PageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "0"))

	list, total, err := a.daoManager.ListCertificateQuery(q)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	items := make([]certListItem, 0, len(list))
	for _, cert := range list {
		items = append(items, toCertListItem(cert))
	}
	c.JSON(200, gin.H{
		"result":    "success",
		"error":     nil,
		"data":      items,
		"total":     total,
		"page":      q.Page,
		"page_size": q.PageSize,
	})
}

// certDetailInfo 证书详情响应：不含证书/私钥本体，仅返回展示所需的详细信息。
type certDetailInfo struct {
	ID              uint     `json:"id"`
	Name            string   `json:"name"`
	Type            uint     `json:"type"`
	ParentID        uint     `json:"parent_id"`
	KeyType         string   `json:"key_type"`
	HasKey          bool     `json:"has_key"`
	SerialNumber    string   `json:"serial_number"`
	Subject         string   `json:"subject"`
	Issuer          string   `json:"issuer"`
	CommonName      string   `json:"common_name"`
	NotBefore       int64    `json:"not_before"`
	NotAfter        int64    `json:"not_after"`
	KeyUsage        []string `json:"key_usage"`
	ExtKeyUsage     []string `json:"ext_key_usage"`
	CertSHA256      string   `json:"cert_sha256"`
	PublicKeySHA256 string   `json:"public_key_sha256"`
	Description     string   `json:"description"`
	CreatedAt       int64    `json:"created_at"`
}

// formatFingerprint 把小写十六进制指纹格式化为冒号分隔的大写形式。
func formatFingerprint(rawHex string) string {
	if rawHex == "" {
		return ""
	}
	upper := strings.ToUpper(rawHex)
	parts := make([]string, 0, len(upper)/2)
	for i := 0; i+2 <= len(upper); i += 2 {
		parts = append(parts, upper[i:i+2])
	}
	return strings.Join(parts, ":")
}

// logCertificateEvent 记录证书操作审计事件（certificate_events 表），事件内容包含证书 CN 与 SHA-256 指纹。
// server 非空时表示该事件与某服务器相关（如服务器引用证书）。
func (a *App) logCertificateEvent(c *gin.Context, eventType int, action string, cert *models.Certificate, server *models.Server) {
	cn := ""
	if parsed, err := certutil.ParseCertificate(cert.Cert); err == nil {
		cn = parsed.Subject.CommonName
	}
	fp := ""
	if raw, err := certutil.FingerprintSHA256(cert.Cert); err == nil {
		fp = formatFingerprint(raw)
	}
	event := &models.CertificateEvent{
		EventType:  eventType,
		EventTime:  uint64(time.Now().Unix()),
		RealIPAddr: c.ClientIP(),
		EventData:  fmt.Sprintf("操作=%s 证书名称=%s CN=%s 指纹=%s", action, cert.Name, cn, fp),
		CertID:     cert.ID,
		CertName:   cert.Name,
	}
	if server != nil {
		event.ServerID = server.ID
		event.ServerName = server.Name
		event.EventData += fmt.Sprintf(" 服务器=%s(id=%d)", server.Name, server.ID)
	}
	if err := a.daoManager.CreateCertificateEvent(event); err != nil {
		log.Printf("记录证书审计事件失败: %v", err)
	}
}

func certDetailFromModel(cert *models.Certificate) certDetailInfo {
	info := certDetailInfo{
		ID:           cert.ID,
		Name:         cert.Name,
		Type:         cert.Type,
		ParentID:     cert.ParentID,
		KeyType:      cert.KeyType,
		HasKey:       cert.HasKey(),
		SerialNumber: cert.SerialNumber,
		Subject:      cert.Subject,
		Issuer:       cert.Issuer,
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		Description:  cert.Description,
		CreatedAt:    cert.CreatedAt,
	}
	if parsed, err := certutil.ParseCertificate(cert.Cert); err == nil {
		info.CommonName = parsed.Subject.CommonName
		info.KeyUsage = certutil.KeyUsageNames(parsed)
		info.ExtKeyUsage = certutil.ExtKeyUsageNames(parsed)
	}
	if fp, err := certutil.FingerprintSHA256(cert.Cert); err == nil {
		info.CertSHA256 = formatFingerprint(fp)
	}
	if fp, err := certutil.PublicKeySHA256(cert.Cert); err == nil {
		info.PublicKeySHA256 = formatFingerprint(fp)
	}
	return info
}

// GetCertificateHandler 获取证书详细信息（不含证书与私钥本体）。
func (a *App) GetCertificateHandler(c *gin.Context, user *models.User) {
	id, err := strconv.ParseUint(c.DefaultQuery("id", ""), 10, 32)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": "参数 id 必须是 int 类型"})
		return
	}
	cert, err := a.daoManager.GetCertificateByID(uint(id))
	if err != nil {
		c.JSON(404, gin.H{"result": "failed", "error": "证书不存在"})
		return
	}
	c.JSON(200, gin.H{
		"result": "success",
		"error":  nil,
		"data":   certDetailFromModel(cert),
	})
}

// sanitizeFilePart 清理下载文件名中的单个片段。
func sanitizeFilePart(s, fallback string) string {
	s = unsafeFileNameChars.ReplaceAllString(strings.TrimSpace(s), "_")
	s = strings.Trim(s, "_")
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		s = fallback
	}
	return s
}

// certDownloadFileName 生成下载文件名：<名称>-<CN>-<哈希>.<ext>。
func certDownloadFileName(cert *models.Certificate, ext string) string {
	cn := ""
	if parsed, err := certutil.ParseCertificate(cert.Cert); err == nil {
		cn = parsed.Subject.CommonName
	}
	hash := "nohash"
	if fp, err := certutil.FingerprintSHA256(cert.Cert); err == nil && fp != "" {
		if len(fp) > 16 {
			fp = fp[:16]
		}
		hash = fp
	}
	return fmt.Sprintf("%s-%s-%s.%s",
		sanitizeFilePart(cert.Name, "cert"),
		sanitizeFilePart(cn, "ca"),
		hash,
		ext,
	)
}

// downloadCertFile 输出证书或私钥文件下载。
func (a *App) downloadCertFile(c *gin.Context, wantKey bool) {
	id, err := strconv.ParseUint(c.DefaultQuery("id", ""), 10, 32)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": "参数 id 必须是 int 类型"})
		return
	}
	cert, err := a.daoManager.GetCertificateByID(uint(id))
	if err != nil {
		c.JSON(404, gin.H{"result": "failed", "error": "证书不存在"})
		return
	}
	ext := "cert"
	content := cert.Cert
	eventType := models.CERT_EVENT_TYPE_DOWNLOAD_CERT
	action := "下载证书"
	if wantKey {
		ext = "key"
		eventType = models.CERT_EVENT_TYPE_DOWNLOAD_KEY
		action = "下载私钥"
		if !cert.HasKey() {
			c.JSON(404, gin.H{"result": "failed", "error": "该证书没有私钥"})
			return
		}
		content = cert.Key
	}
	fileName := certDownloadFileName(cert, ext)
	a.logCertificateEvent(c, eventType, action, cert, nil)
	c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	c.Data(200, "application/x-pem-file", []byte(content))
}

// DownloadCertificateHandler 下载证书 PEM。
func (a *App) DownloadCertificateHandler(c *gin.Context, user *models.User) {
	a.downloadCertFile(c, false)
}

// DownloadCertificateKeyHandler 下载私钥 PEM。
func (a *App) DownloadCertificateKeyHandler(c *gin.Context, user *models.User) {
	a.downloadCertFile(c, true)
}

// ParseCertificateHandler 解析一段 PEM 证书（不保存），用于前端展示“手动填写”证书的信息。
// 可选传入私钥，用于判断证书与私钥是否匹配。
func (a *App) ParseCertificateHandler(c *gin.Context, user *models.User) {
	type Param struct {
		Cert string `json:"cert" binding:"required,max=65536"`
		Key  string `json:"key" binding:"max=65536"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	parsed, err := certutil.ParseCertificate(param.Cert)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": "证书解析失败: " + err.Error()})
		return
	}

	certType := "未指定"
	if certutil.IsCACertificate(parsed) {
		certType = "CA 证书"
	} else {
		serverAuth, clientAuth := certutil.CertKeyUsage(parsed)
		switch models.CertTypeFromEKU(serverAuth, clientAuth) {
		case models.CERT_TYPE_SERVER:
			certType = "服务器证书"
		case models.CERT_TYPE_CLIENT:
			certType = "客户端证书"
		default:
			certType = "未指定"
		}
	}

	resp := gin.H{
		"subject":       certutil.FormatName(parsed.Subject),
		"issuer":        certutil.FormatName(parsed.Issuer),
		"serial_number": parsed.SerialNumber.Text(16),
		"not_before":    parsed.NotBefore.Unix(),
		"not_after":     parsed.NotAfter.Unix(),
		"is_ca":         certutil.IsCACertificate(parsed),
		"cert_type":     certType,
	}
	if strings.TrimSpace(param.Key) != "" {
		if err := certutil.KeyMatchesCert(param.Cert, param.Key); err != nil {
			resp["key_matches"] = false
		} else {
			resp["key_matches"] = true
			if keyType, err := certutil.PrivateKeyType(param.Key); err == nil {
				resp["key_type"] = keyType
			}
		}
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": resp})
}

// GenerateCAHandler 生成自签名 CA 证书。
func (a *App) GenerateCAHandler(c *gin.Context, user *models.User) {
	type Param struct {
		Name               string `json:"name" binding:"required,min=1,max=100"`
		CommonName         string `json:"common_name" binding:"required,min=1,max=100"`
		Org                string `json:"org" binding:"max=100"`
		OrganizationalUnit string `json:"organizational_unit" binding:"max=100"`
		Country            string `json:"country" binding:"max=10"`
		Province           string `json:"province" binding:"max=100"`
		Locality           string `json:"locality" binding:"max=100"`
		EmailAddress       string `json:"email_address" binding:"max=200"`
		Days               int    `json:"days"`
		KeyType            string `json:"key_type"`
		RSABits            int    `json:"rsa_bits"`
		ECCurve            string `json:"ec_curve"`
		Description        string `json:"description" binding:"max=500"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	pair, err := certutil.GenerateCA(certutil.CertOptions{
		CommonName:         param.CommonName,
		Org:                param.Org,
		OrganizationalUnit: param.OrganizationalUnit,
		Country:            param.Country,
		Province:           param.Province,
		Locality:           param.Locality,
		EmailAddress:       param.EmailAddress,
		Days:               param.Days,
		Key:                certKeyOptions(param.KeyType, param.RSABits, param.ECCurve),
	})
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "生成 CA 失败: " + err.Error()})
		return
	}
	cert := a.certPairToModel(param.Name, models.CERT_TYPE_CA, 0, pair, param.Description)
	if err := a.daoManager.CreateCertificate(cert); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	a.logCertificateEvent(c, models.CERT_EVENT_TYPE_CREATE, "生成CA", cert, nil)
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": toCertListItem(cert)})
}

// SignCertificateHandler 使用上级 CA 签发服务器证书或客户端证书。
func (a *App) SignCertificateHandler(c *gin.Context, user *models.User) {
	type Param struct {
		CAID               uint   `json:"ca_id" binding:"required"`
		Name               string `json:"name" binding:"required,min=1,max=100"`
		CommonName         string `json:"common_name" binding:"required,min=1,max=100"`
		Org                string `json:"org" binding:"max=100"`
		OrganizationalUnit string `json:"organizational_unit" binding:"max=100"`
		Country            string `json:"country" binding:"max=10"`
		Province           string `json:"province" binding:"max=100"`
		Locality           string `json:"locality" binding:"max=100"`
		EmailAddress       string `json:"email_address" binding:"max=200"`
		Days               int    `json:"days"`
		CertType           uint   `json:"cert_type" binding:"required"`
		KeyType            string `json:"key_type"`
		RSABits            int    `json:"rsa_bits"`
		ECCurve            string `json:"ec_curve"`
		Description        string `json:"description" binding:"max=500"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if param.CertType != models.CERT_TYPE_SERVER && param.CertType != models.CERT_TYPE_CLIENT && param.CertType != models.CERT_TYPE_UNSPECIFIED {
		c.JSON(400, gin.H{"result": "failed", "error": "证书类型必须是服务器证书、客户端证书或未指定"})
		return
	}
	ca, err := a.daoManager.GetCertificateByID(param.CAID)
	if err != nil {
		c.JSON(404, gin.H{"result": "failed", "error": "上级 CA 不存在"})
		return
	}
	if ca.Type != models.CERT_TYPE_CA {
		c.JSON(400, gin.H{"result": "failed", "error": "指定的证书不是 CA"})
		return
	}
	if !ca.HasKey() {
		c.JSON(400, gin.H{"result": "failed", "error": "该 CA 没有私钥，无法签发证书"})
		return
	}
	pair, err := certutil.SignCert(ca.Cert, ca.Key, certutil.CertOptions{
		CommonName:         param.CommonName,
		Org:                param.Org,
		OrganizationalUnit: param.OrganizationalUnit,
		Country:            param.Country,
		Province:           param.Province,
		Locality:           param.Locality,
		EmailAddress:       param.EmailAddress,
		Days:               param.Days,
		ServerAuth:         param.CertType == models.CERT_TYPE_SERVER,
		ClientAuth:         param.CertType == models.CERT_TYPE_CLIENT,
		Key:                certKeyOptions(param.KeyType, param.RSABits, param.ECCurve),
	})
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "签发证书失败: " + err.Error()})
		return
	}
	cert := a.certPairToModel(param.Name, param.CertType, param.CAID, pair, param.Description)
	if err := a.daoManager.CreateCertificate(cert); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	a.logCertificateEvent(c, models.CERT_EVENT_TYPE_SIGN, "签发证书", cert, nil)
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": toCertListItem(cert)})
}

// ImportCertificateHandler 导入证书（私钥可选）。
// 证书类型由证书的扩展密钥用法自动识别（CA / 服务器 / 客户端 / 未指定）；
// 导入的非 CA 证书必须由指定 CA 签发。
func (a *App) ImportCertificateHandler(c *gin.Context, user *models.User) {
	type Param struct {
		Name        string `json:"name" binding:"required,min=1,max=100"`
		ParentID    uint   `json:"parent_id"`
		Cert        string `json:"cert" binding:"required"`
		Key         string `json:"key"`
		Description string `json:"description" binding:"max=500"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	parsed, err := certutil.ParseCertificate(param.Cert)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": "证书解析失败: " + err.Error()})
		return
	}

	var certType uint
	var parentID uint
	if certutil.IsCACertificate(parsed) {
		certType = models.CERT_TYPE_CA
		parentID = 0
	} else {
		if param.ParentID == 0 {
			c.JSON(400, gin.H{"result": "failed", "error": "导入服务器/客户端证书必须指定上级 CA"})
			return
		}
		ca, err := a.daoManager.GetCertificateByID(param.ParentID)
		if err != nil {
			c.JSON(404, gin.H{"result": "failed", "error": "上级 CA 不存在"})
			return
		}
		if ca.Type != models.CERT_TYPE_CA {
			c.JSON(400, gin.H{"result": "failed", "error": "指定的上级证书不是 CA"})
			return
		}
		if err := certutil.ValidateSignedBy(param.Cert, ca.Cert); err != nil {
			c.JSON(400, gin.H{"result": "failed", "error": "该证书不是此 CA 签发，无法导入: " + err.Error()})
			return
		}
		serverAuth, clientAuth := certutil.CertKeyUsage(parsed)
		certType = models.CertTypeFromEKU(serverAuth, clientAuth)
		parentID = param.ParentID
	}

	keyType := ""
	if strings.TrimSpace(param.Key) != "" {
		if err := certutil.KeyMatchesCert(param.Cert, param.Key); err != nil {
			c.JSON(400, gin.H{"result": "failed", "error": "证书与私钥不匹配: " + err.Error()})
			return
		}
		keyType, err = certutil.PrivateKeyType(param.Key)
		if err != nil {
			c.JSON(400, gin.H{"result": "failed", "error": "私钥解析失败: " + err.Error()})
			return
		}
	}

	cert := &models.Certificate{
		Name:         param.Name,
		Type:         certType,
		ParentID:     parentID,
		Cert:         strings.TrimSpace(param.Cert) + "\n",
		Key:          strings.TrimSpace(param.Key),
		KeyType:      keyType,
		SerialNumber: parsed.SerialNumber.Text(16),
		Subject:      certutil.FormatName(parsed.Subject),
		Issuer:       certutil.FormatName(parsed.Issuer),
		NotBefore:    parsed.NotBefore.Unix(),
		NotAfter:     parsed.NotAfter.Unix(),
		Description:  param.Description,
		CreatedAt:    time.Now().Unix(),
	}
	if err := a.daoManager.CreateCertificate(cert); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	a.logCertificateEvent(c, models.CERT_EVENT_TYPE_IMPORT, "导入证书", cert, nil)
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": toCertListItem(cert)})
}

// DeleteCertificateHandler 删除证书。
// - CA：若其下仍有服务器证书，拒绝删除；删除时会一并清理其下的客户端证书，避免留下悬挂引用。
// - 任意证书：若有服务器正在使用该证书 id，拒绝删除。
func (a *App) DeleteCertificateHandler(c *gin.Context, user *models.User) {
	type Param struct {
		ID uint `json:"id" binding:"required"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	cert, err := a.daoManager.GetCertificateByID(param.ID)
	if err != nil {
		c.JSON(404, gin.H{"result": "failed", "error": "证书不存在"})
		return
	}

	var childList []*models.Certificate
	if cert.Type == models.CERT_TYPE_CA {
		// 该 CA 下若有服务器证书，禁止删除
		serverCount, err := a.daoManager.CountChildCertificateByType(cert.ID, models.CERT_TYPE_SERVER)
		if err != nil {
			c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
			return
		}
		if serverCount > 0 {
			c.JSON(400, gin.H{"result": "failed", "error": "该 CA 下仍有服务器证书，无法删除"})
			return
		}
		childList, err = a.daoManager.ListChildCertificate(cert.ID)
		if err != nil {
			c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
			return
		}
		for _, child := range childList {
			ref, err := a.daoManager.CountServerReferencingCertificate(child.ID)
			if err != nil {
				c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
				return
			}
			if ref > 0 {
				c.JSON(400, gin.H{"result": "failed", "error": "该 CA 下的证书正被服务器使用，无法删除"})
				return
			}
		}
	}

	// 是否有服务器引用该证书
	refCount, err := a.daoManager.CountServerReferencingCertificate(cert.ID)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if refCount > 0 {
		c.JSON(400, gin.H{"result": "failed", "error": "该证书正被服务器使用，无法删除"})
		return
	}

	// 删除 CA 时一并删除其下剩余的子证书（客户端证书）
	for _, child := range childList {
		if err := a.daoManager.DeleteCertificate(child.ID); err != nil {
			c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
			return
		}
		a.logCertificateEvent(c, models.CERT_EVENT_TYPE_DELETE, "删除证书(随CA级联)", child, nil)
	}
	if err := a.daoManager.DeleteCertificate(cert.ID); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	a.logCertificateEvent(c, models.CERT_EVENT_TYPE_DELETE, "删除证书", cert, nil)
	c.JSON(200, gin.H{"result": "success", "error": nil})
}

// GenerateDHHandler 生成 DH 参数（RFC 3526 标准 MODP 组）。
func (a *App) GenerateDHHandler(c *gin.Context, user *models.User) {
	type Param struct {
		Bits int `json:"bits"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	if param.Bits == 0 {
		param.Bits = 2048
	}
	dh, err := certutil.GenerateDHParams(param.Bits)
	if err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": gin.H{"dh": dh}})
}

// GenerateTLSAuthHandler 调用 openvpn 生成 ta.key。
func (a *App) GenerateTLSAuthHandler(c *gin.Context, user *models.User) {
	resourceMap := a.PrepareResourceMap([]string{ovpnserver.RESOURCE_ID_MISC_CONFIG})
	miscConfig, err := ovpnserver.ParseMiscConfig(resourceMap)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "读取其他配置失败: " + err.Error()})
		return
	}
	openvpnPath := miscConfig.OpenVPNPath
	if strings.TrimSpace(openvpnPath) == "" {
		openvpnPath = "openvpn"
	}
	if _, err := exec.LookPath(openvpnPath); err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "未找到 openvpn 命令，无法生成 ta.key: " + err.Error()})
		return
	}

	tmpDir, err := os.MkdirTemp("", "ovpn-ta-")
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	defer os.RemoveAll(tmpDir)
	outFile := filepath.Join(tmpDir, "ta.key")

	// OpenVPN 2.5+ 使用 --genkey secret，旧版本使用 --genkey --secret。
	cmd := exec.Command(openvpnPath, "--genkey", "secret", outFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		cmd = exec.Command(openvpnPath, "--genkey", "--secret", outFile)
		out, err = cmd.CombinedOutput()
	}
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "生成 ta.key 失败: " + err.Error() + " 输出: " + string(out)})
		return
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "读取生成的 ta.key 失败: " + err.Error()})
		return
	}
	if !strings.Contains(string(data), "BEGIN OpenVPN Static key") {
		c.JSON(500, gin.H{"result": "failed", "error": "生成的 ta.key 内容无效"})
		return
	}
	c.JSON(200, gin.H{"result": "success", "error": nil, "data": gin.H{"tls_auth": string(data)}})
}

// resolveCertReference 解析服务器 CA/证书/私钥字段中的证书引用，形如 cert-stor:<id>/cert 或 cert-stor:<id>/key。
// 非引用值原样返回。
func (a *App) resolveCertReference(value string) string {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, models.CERT_REF_PREFIX) {
		return value
	}
	rest := strings.TrimPrefix(trimmed, models.CERT_REF_PREFIX)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		log.Printf("证书引用格式无效: %s", value)
		return value
	}
	id, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		log.Printf("证书引用 ID 无效: %s", value)
		return value
	}
	cert, err := a.daoManager.GetCertificateByID(uint(id))
	if err != nil {
		log.Printf("证书引用不存在: %s", value)
		return ""
	}
	switch parts[1] {
	case "cert":
		return cert.Cert
	case "key":
		return cert.Key
	default:
		log.Printf("证书引用部分无效: %s", value)
		return value
	}
}

// resolvedServerModel 返回 CA/证书/私钥字段已解析为实际 PEM 的服务器副本，用于写入文件。
func (a *App) resolvedServerModel(server *models.Server) *models.Server {
	resolved := *server
	resolved.CA = a.resolveCertReference(server.CA)
	resolved.Cert = a.resolveCertReference(server.Cert)
	resolved.Key = a.resolveCertReference(server.Key)
	return &resolved
}

// certRefIDs 返回若干字段中所有 cert-stor 引用的证书 id（去重，保持出现顺序）。
func certRefIDs(values ...string) []uint {
	seen := make(map[uint]bool)
	var ids []uint
	for _, value := range values {
		if id, ok := certRefID(value); ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// validateCertReferences 校验服务器 CA/Cert/Key 字段中的 cert-stor:<id>/cert|key 引用是否合法且存在。
// 非引用值（手动填写的 PEM）直接跳过。
func (a *App) validateCertReferences(values ...string) error {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if !strings.HasPrefix(trimmed, models.CERT_REF_PREFIX) {
			continue
		}
		rest := strings.TrimPrefix(trimmed, models.CERT_REF_PREFIX)
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 || (parts[1] != "cert" && parts[1] != "key") {
			return fmt.Errorf("证书引用格式无效: %s（应为 cert-stor:<id>/cert 或 cert-stor:<id>/key）", trimmed)
		}
		id, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil || id == 0 {
			return fmt.Errorf("证书引用 ID 无效: %s", trimmed)
		}
		if _, err := a.daoManager.GetCertificateByID(uint(id)); err != nil {
			return fmt.Errorf("证书引用不存在: 引用的证书 id=%d 不存在，请重新选择", id)
		}
	}
	return nil
}
