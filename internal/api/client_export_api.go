package api

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"openvpn-pannel/internal/certutil"
	"openvpn-pannel/internal/models"
	ovpnserver "openvpn-pannel/internal/ovpn_server"

	"github.com/gin-gonic/gin"
)

var unsafeFileNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeFileName(name string) string {
	name = unsafeFileNameChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = "client"
	}
	return name
}

// certRefID 判断一个服务器证书字段是否为 cert-stor:<id>/... 引用，是则返回 id。
func certRefID(value string) (uint, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, models.CERT_REF_PREFIX) {
		return 0, false
	}
	rest := strings.TrimPrefix(trimmed, models.CERT_REF_PREFIX)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return 0, false
	}
	id, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

// ExportClientConfigHandler 渲染并下载客户端配置文件，同时保存该服务器导出用的地址与端口。
// 两种模式：
//   - 服务器 CA 引用证书管理（cert-stor:）：必须选择由该 CA 签发的客户端证书（cert_id）。
//   - 服务器 CA 为手动填写：由请求直接提供客户端证书/私钥（client_cert/client_key），均可留空。
func (a *App) ExportClientConfigHandler(c *gin.Context, user *models.User) {
	type Param struct {
		ServerID    uint   `json:"server_id" binding:"required"`
		CertID      uint   `json:"cert_id"`
		ClientCert  string `json:"client_cert" binding:"max=65536"`
		ClientKey   string `json:"client_key" binding:"max=65536"`
		ExtraConfig string `json:"extra_config" binding:"max=16384"`
		Host        string `json:"host" binding:"required,max=255"`
		Port        uint32 `json:"port" binding:"required"`
		Save        *bool  `json:"save"`
	}
	var param Param
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(400, gin.H{"result": "failed", "error": err.Error()})
		return
	}
	server, err := a.daoManager.GetOpenVPNServerByID(param.ServerID)
	if err != nil {
		c.JSON(404, gin.H{"result": "failed", "error": "服务器不存在"})
		return
	}

	caID, caIsRef := certRefID(server.CA)

	var clientCertPEM, clientKeyPEM, clientCertName string
	switch {
	case param.CertID != 0:
		clientCert, err := a.daoManager.GetCertificateByID(param.CertID)
		if err != nil {
			c.JSON(404, gin.H{"result": "failed", "error": "客户端证书不存在"})
			return
		}
		if clientCert.Type != models.CERT_TYPE_CLIENT {
			c.JSON(400, gin.H{"result": "failed", "error": "所选证书不是客户端证书"})
			return
		}
		// 服务器 CA 为证书管理引用时，客户端证书必须由该 CA 签发
		if caIsRef && clientCert.ParentID != caID {
			c.JSON(400, gin.H{"result": "failed", "error": "所选客户端证书不是该服务器 CA 签发的，无法导出"})
			return
		}
		clientCertPEM = clientCert.Cert
		clientKeyPEM = clientCert.Key
		clientCertName = clientCert.Name
	case caIsRef:
		c.JSON(400, gin.H{"result": "failed", "error": "请选择由该服务器 CA 签发的客户端证书"})
		return
	default:
		// 服务器 CA 为手动填写：使用请求携带的证书/私钥，允许留空由用户后续自行补全
		clientCertPEM = strings.TrimSpace(param.ClientCert)
		clientKeyPEM = strings.TrimSpace(param.ClientKey)
		if clientCertPEM != "" {
			parsed, err := certutil.ParseCertificate(clientCertPEM)
			if err != nil {
				c.JSON(400, gin.H{"result": "failed", "error": "客户端证书解析失败: " + err.Error()})
				return
			}
			clientCertName = parsed.Subject.CommonName
			clientCertPEM += "\n"
			if clientKeyPEM != "" {
				if err := certutil.KeyMatchesCert(clientCertPEM, clientKeyPEM); err != nil {
					c.JSON(400, gin.H{"result": "failed", "error": "客户端证书与私钥不匹配: " + err.Error()})
					return
				}
			}
		} else if clientKeyPEM != "" {
			c.JSON(400, gin.H{"result": "failed", "error": "填写客户端私钥时必须同时填写客户端证书"})
			return
		}
	}

	resourceMap := a.PrepareResourceMap([]string{
		ovpnserver.RESOURCE_ID_CLIENT_CONFIG,
		ovpnserver.RESOURCE_ID_MISC_CONFIG,
	})
	template := resourceMap[ovpnserver.RESOURCE_ID_CLIENT_CONFIG]
	if strings.TrimSpace(template) == "" {
		template = ovpnserver.GetDefaultResource(ovpnserver.RESOURCE_ID_CLIENT_CONFIG)
	}

	renderParam := &ovpnserver.OpenvpnClientTemplateParam{
		Server:     server,
		CA:         a.resolveCertReference(server.CA),
		Cert:       clientCertPEM,
		Key:        clientKeyPEM,
		TLSAuthKey: server.TLSAuthKey,
		RemoteHost: strings.TrimSpace(param.Host),
		RemotePort: param.Port,
	}
	configContent, err := ovpnserver.RenderOpenvpnClientConfig(template, renderParam)
	if err != nil {
		c.JSON(500, gin.H{"result": "failed", "error": "渲染客户端配置失败: " + err.Error()})
		return
	}

	// 客户端附加配置：追加到配置文件末尾
	extraConfig := strings.TrimSpace(param.ExtraConfig)
	if extraConfig != "" {
		configContent = strings.TrimRight(configContent, "\n") + "\n\n" + extraConfig + "\n"
	}

	if param.Save == nil || *param.Save {
		if err := a.daoManager.UpdateServerExportSetting(server.ID, strings.TrimSpace(param.Host), param.Port, strings.TrimSpace(param.ExtraConfig)); err != nil {
			// 保存失败不阻断导出，仅记录日志
			log.Printf("保存服务器导出设置失败 server %d: %v", server.ID, err)
		}
	}

	name := sanitizeFileName(server.Name)
	if clientCertName != "" {
		name += "-" + sanitizeFileName(clientCertName)
	}
	fileName := fmt.Sprintf("%s.ovpn", name)
	c.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)
	c.Data(200, "application/x-openvpn-profile; charset=utf-8", []byte(configContent))
}
