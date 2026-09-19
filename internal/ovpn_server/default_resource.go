package ovpnserver

import (
	"encoding/json"
	"log"
)

type MiscConfig struct {
	OpenVPNPath      string `json:"openvpn_path"`
	ServerLog        string `json:"server_log"`
	ScriptLog        string `json:"script_log"`
	ManagementSocket string `json:"management_socket"`

	CAFileName         string `json:"ca_file_name"`
	ServerCertFileName string `json:"server_cert_file_name"`
	ServerKeyFileName  string `json:"server_key_file_name"`
	DHFileName         string `json:"dh_file_name"`
	TAFileName         string `json:"ta_file_name"`
	CCDDir             string `json:"ccd_dir"`
	IPPFileName        string `json:"ipp_file_name"`
	StatusFileName     string `json:"status_file_name"`

	ServerConfigFileName string `json:"server_config_file_name"`

	ClientOnlineScriptName  string `json:"client_online_script_name"`
	ClientOfflineScriptName string `json:"client_offline_script_name"`
	ClientAuthScriptName    string `json:"client_auth_script_name"`

	ShellPath string `json:"shell_path"`
}

const (
	RESOURCE_ID_CONFIG_TEMPLATE       = "config"
	RESOURCE_ID_AUTH_SCRIPT           = "auth.sh"
	RESOURCE_ID_CLIENT_OFFLINE_SCRIPT = "client_offline.sh"
	RESOURCE_ID_SERVER_START_SCRIPT   = "server_start.sh"
	RESOURCE_ID_SERVER_EXIT_SCRIPT    = "server_exit.sh"
	RESOURCE_ID_CLIENT_ONLINE_SCRIPT  = "client_online.sh"
	RESOURCE_ID_RATE_LIMIT_SCRIPT     = "ratelimit.sh"
	RESOURCE_ID_MISC_CONFIG           = "misc"
	RESOURCE_ID_HELP_INFO             = "help"
	RESOURCE_ID_CLIENT_CONFIG         = "client-config"
)

func GetDefaultResource(name string) string {
	data, err := Asset(name)
	if err != nil {
		log.Println("获取内置资源失败 ", err.Error())
		return ""
	}
	return string(data)
}

// ParseMiscConfig 从资源映射解析其他配置（供 API 层获取 openvpn_path / shell_path 等）。
func ParseMiscConfig(resourceMap map[string]string) (*MiscConfig, error) {
	content, ok := resourceMap[RESOURCE_ID_MISC_CONFIG]
	if !ok {
		content = GetDefaultResource(RESOURCE_ID_MISC_CONFIG)
	}
	var miscConfig MiscConfig
	if err := json.Unmarshal([]byte(content), &miscConfig); err != nil {
		return nil, err
	}
	return &miscConfig, nil
}
