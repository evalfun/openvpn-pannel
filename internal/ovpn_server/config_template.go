package ovpnserver

import (
	"bytes"
	"fmt"
	"openvpn-pannel/internal/models"
	"text/template"
)

type OpenvpnServerTemplateParam struct {
	ServerConfig *models.Server
	ServerRoute  []*models.ServerRoute
	WorkingDir   string
	MiscConfig   *MiscConfig
}

func RenderOpenvpnServerConfig(templ string, param *OpenvpnServerTemplateParam) (string, error) {
	t, err := template.New("config").Parse(templ)
	if err != nil {
		return "", fmt.Errorf("模板渲染失败: %s", err.Error())
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, param)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// OpenvpnClientTemplateParam 客户端配置文件模板参数（证书/密钥已内联）。
type OpenvpnClientTemplateParam struct {
	Server     *models.Server
	CA         string
	Cert       string
	Key        string
	TLSAuthKey string
	RemoteHost string
	RemotePort uint32
}

func RenderOpenvpnClientConfig(templ string, param *OpenvpnClientTemplateParam) (string, error) {
	t, err := template.New("client-config").Parse(templ)
	if err != nil {
		return "", fmt.Errorf("客户端配置模板渲染失败: %s", err.Error())
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, param)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
