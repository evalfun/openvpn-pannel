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
	// MaxLogSizeKB 单个服务器日志(openvpn.log/auth.log 及其归档)的容量上限，单位 KB。
	// 达到该值时定时任务会轮换并删除最早归档，回收至该上限的 80%。<=0 关闭日志轮换。
	MaxLogSizeKB int64 `json:"max_log_size_kb"`
	// AllowEditResource 是否允许通过接口修改/重置脚本等资源(如 auth.sh、client_online.sh、
	// 配置模板等)。默认 false：所有 resource/write 与 resource/delete 请求都会被拒绝，
	// 以避免脚本资源被随意改写带来的安全隐患。需要在线编辑资源时必须在 config.json 中
	// 显式设置 "allow_edit_resource": true。
	AllowEditResource bool `json:"allow_edit_resource"`
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
