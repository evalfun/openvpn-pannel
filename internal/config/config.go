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
	// TrustedProxies 为反向代理服务器 IP/CIDR 白名单（如 ["127.0.0.1","10.0.0.0/8"]）。
	// 只有来自这些地址的请求，其 X-Forwarded-For / X-Real-IP 才会被采信为客户端真实 IP；
	// 未配置（空）时禁用代理信任，客户端 IP 一律取 TCP 来源地址，防止伪造来源。
	TrustedProxies []string `json:"trusted_proxies"`
}

// ShouldStopInstancesOnExit 返回面板退出前是否应停止所有实例。未配置时默认 true。
func (c *Config) ShouldStopInstancesOnExit() bool {
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
