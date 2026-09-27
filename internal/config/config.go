package config

import (
	"encoding/json"
	"errors"
	"os"
)

type Config struct {
	Listen            string `json:"listen"`
	MysqlAddr         string `json:"mysql_addr"`
	MysqlUser         string `json:"mysql_user"`
	MysqlPass         string `json:"mysql_pass"`
	MysqlDB           string `json:"mysql_db"`
	SQLiteDB          string `json:"sqlite_db"`
	PasswordSalt      string `json:"passwd_salt"`
	SessionSecret     string `json:"session_secret"`
	WorkingDir        string `json:"working_dir"`
	InternalAPIListen string `json:"internal_api_listen"`
	// ClientPageListen 面向已连接 VPN 客户端的自助页面监听地址（如 "10.0.1.1:8088"）。
	// 为空表示不启用该页面。页面依据 HTTP 来源 IP 识别客户端，非 VPN 客户端访问会返回错误。
	ClientPageListen string `json:"client_page_listen"`
	// MaxLogSizeKB 单个服务器日志(openvpn.log/auth.log 及其归档)的容量上限，单位 KB。
	// 达到该值时定时任务会轮换并删除最早归档，回收至该上限的 80%。<=0 关闭日志轮换。
	MaxLogSizeKB int64 `json:"max_log_size_kb"`
	// AllowEditResource 是否允许通过接口修改/重置脚本等资源(如 auth.sh、client_online.sh、
	// 配置模板等)。默认 false：所有 resource/write 与 resource/delete 请求都会被拒绝，
	// 以避免脚本资源被随意改写带来的安全隐患。需要在线编辑资源时必须在 config.json 中
	// 显式设置 "allow_edit_resource": true。
	AllowEditResource bool `json:"allow_edit_resource"`
	// StopInstancesOnExit 面板进程退出（SIGINT/SIGTERM）前是否停止所有 OpenVPN 实例。
	// 未配置（nil）时默认 true，保持旧版本行为：退出前停止所有实例。
	// 显式设为 false 时：退出/崩溃都不再停止实例，实例脱离面板成为孤儿进程继续运行，
	// 由 systemd 等托管进程回收；面板（重新）启动时会依据数据库中的进程记录重新接管
	// 这些仍在运行的实例，从而做到面板重启期间客户端无感知。
	StopInstancesOnExit *bool `json:"stop_instances_on_exit"`
	// MaxMemoryEvents 内存事件数量上限（含服务器事件与证书事件）。
	// 为 0 时事件写入数据库（默认，行为不变）；不为 0 时改为启用「内存模式」：
	//   - 服务器事件与证书事件只存内存，最多保留该数目条最新事件；
	//   - 在线会话记录、已下发 ACL 记录、用户达量限速周期等日常运行数据也只存内存，
	//     使日常运行期间数据库几乎只读（仅显式运维操作才写库），保护嵌入式设备闪存；
	//   - 强制 stop_instances_on_exit=true（面板退出时停止所有实例），以保证重启后状态干净。
	// 代价：进程重启后上述内存数据丢失（客户端重连会自然重建），终身总流量亦在内存、重启后不保留。
	MaxMemoryEvents int `json:"max_memory_events"`
	// TrustedProxies 为反向代理服务器 IP/CIDR 白名单（如 ["127.0.0.1","10.0.0.0/8"]）。
	// 只有来自这些地址的请求，其 X-Forwarded-For / X-Real-IP 才会被采信为客户端真实 IP；
	// 未配置（空）时禁用代理信任，客户端 IP 一律取 TCP 来源地址，防止伪造来源。
	TrustedProxies []string `json:"trusted_proxies"`
}

// ShouldStopInstancesOnExit 返回面板退出前是否应停止所有实例。未配置时默认 true。
//
// 启用内存事件/运行时数据（max_memory_events > 0）时强制返回 true：此时在线会话、
// 已下发 ACL 等数据仅存于内存，面板退出若不停止实例，重启后无法正确接管（会话记录与
// ACL 记录已丢失，可能导致残留防火墙规则）。强制停止实例可确保重启后状态干净。
func (c *Config) ShouldStopInstancesOnExit() bool {
	if c.MaxMemoryEvents > 0 {
		return true
	}
	return c.StopInstancesOnExit == nil || *c.StopInstancesOnExit
}

func ReadConfig(path string) (*Config, error) {
	config := Config{}
	configFile, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("读取配置文件失败 " + err.Error())
	}
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		return nil, errors.New("解析配置文件失败 " + err.Error())
	}
	return &config, nil

}
