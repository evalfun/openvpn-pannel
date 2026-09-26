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

// 资源 ID（每个资源集内的文件名）。
const (
	RESOURCE_ID_CONFIG_TEMPLATE       = "config"
	RESOURCE_ID_AUTH_SCRIPT           = "auth.sh"
	RESOURCE_ID_CLIENT_OFFLINE_SCRIPT = "client_offline.sh"
	RESOURCE_ID_SERVER_START_SCRIPT   = "server_start.sh"
	RESOURCE_ID_SERVER_EXIT_SCRIPT    = "server_exit.sh"
	RESOURCE_ID_CLIENT_ONLINE_SCRIPT  = "client_online.sh"
	RESOURCE_ID_RATE_LIMIT_SCRIPT     = "ratelimit.sh"
	RESOURCE_ID_ACL_ADD_SCRIPT        = "acl_add.sh"
	RESOURCE_ID_ACL_DEL_SCRIPT        = "acl_del.sh"
	RESOURCE_ID_MISC_CONFIG           = "misc"
	RESOURCE_ID_HELP_INFO             = "help"
	RESOURCE_ID_CLIENT_CONFIG         = "client-config"
	RESOURCE_ID_CLIENT_PAGE           = "client-page.html"
)

// 预设资源集（每个资源集包含上面全部 13 个资源文件）。
const (
	RESOURCE_SET_LINUX_IPTABLES   = "linux-iptables"
	RESOURCE_SET_LINUX_NFTABLES   = "linux-nftables"
	RESOURCE_SET_OPENWRT_IPTABLES = "openwrt-iptables"
	RESOURCE_SET_OPENWRT_NFTABLES = "openwrt-nftables"
)

// ResourceSetInfo 描述一个资源集（供 API / 前端展示）。
type ResourceSetInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// resourceSetList 预置的 4 套资源集（顺序即前端展示顺序）。
var resourceSetList = []ResourceSetInfo{
	{
		ID:          RESOURCE_SET_LINUX_IPTABLES,
		Name:        "普通 Linux 发行版 - iptables",
		Description: "适用于标准 Linux 发行版，使用 iptables/ipset 实现 ACL 与限速。",
	},
	{
		ID:          RESOURCE_SET_LINUX_NFTABLES,
		Name:        "普通 Linux 发行版 - nftables",
		Description: "适用于标准 Linux 发行版，使用纯 nftables 实现 ACL（无需 ipset/iptables）。",
	},
	{
		ID:          RESOURCE_SET_OPENWRT_IPTABLES,
		Name:        "OpenWrt - iptables",
		Description: "适用于 OpenWrt/ImmortalWrt，使用 iptables/ipset，下载限速优先 ifb、回退 police。",
	},
	{
		ID:          RESOURCE_SET_OPENWRT_NFTABLES,
		Name:        "OpenWrt - nftables",
		Description: "适用于 OpenWrt/ImmortalWrt，使用纯 nftables，下载限速优先 ifb、回退 police。",
	},
}

// ListResourceSets 返回全部预设资源集。
func ListResourceSets() []ResourceSetInfo {
	return resourceSetList
}

// IsValidResourceSet 判断资源集 ID 是否为预设值。
func IsValidResourceSet(setID string) bool {
	for _, s := range resourceSetList {
		if s.ID == setID {
			return true
		}
	}
	return false
}

// ListResourceIDs 返回一个资源集应包含的全部资源 ID（含描述），供 API / 前端展示。
func ListResourceIDs() []struct {
	ID          string
	Description string
} {
	return []struct {
		ID          string
		Description string
	}{
		{RESOURCE_ID_AUTH_SCRIPT, "客户端认证脚本内容"},
		{RESOURCE_ID_CLIENT_OFFLINE_SCRIPT, "客户端下线脚本内容"},
		{RESOURCE_ID_CLIENT_ONLINE_SCRIPT, "客户端上线脚本内容"},
		{RESOURCE_ID_RATE_LIMIT_SCRIPT, "达量限速运行时更新脚本内容"},
		{RESOURCE_ID_ACL_ADD_SCRIPT, "TOTP 验证通过后动态放行用户 ACL 的脚本内容"},
		{RESOURCE_ID_ACL_DEL_SCRIPT, "登出后动态回收用户 ACL 的脚本内容"},
		{RESOURCE_ID_CONFIG_TEMPLATE, "服务器配置模板内容"},
		{RESOURCE_ID_CLIENT_CONFIG, "客户端配置文件模板内容"},
		{RESOURCE_ID_MISC_CONFIG, "其他配置内容"},
		{RESOURCE_ID_SERVER_EXIT_SCRIPT, "服务端实例停止脚本内容"},
		{RESOURCE_ID_SERVER_START_SCRIPT, "服务端实例启动脚本内容"},
		{RESOURCE_ID_HELP_INFO, "帮助信息文档内容"},
		{RESOURCE_ID_CLIENT_PAGE, "客户端自助服务页面 HTML"},
	}
}

// GetSetDefaultResource 读取指定资源集内置的某个资源文件内容。
// 资源集名与文件名组合成 bindata 键，例如 "openwrt-iptables/client_online.sh"。
func GetSetDefaultResource(setID, name string) string {
	data, err := ResourceSetAsset(setID + "/" + name)
	if err != nil {
		log.Printf("获取内置资源失败 set=%s name=%s: %s", setID, name, err.Error())
		return ""
	}
	return string(data)
}

// ParseMiscConfig 从资源映射解析其他配置（供 API 层获取 openvpn_path / shell_path 等）。
func ParseMiscConfig(resourceMap map[string]string) (*MiscConfig, error) {
	content, ok := resourceMap[RESOURCE_ID_MISC_CONFIG]
	if !ok {
		content = GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_MISC_CONFIG)
	}
	var miscConfig MiscConfig
	if err := json.Unmarshal([]byte(content), &miscConfig); err != nil {
		return nil, err
	}
	return &miscConfig, nil
}
