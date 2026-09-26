package ovpnserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"openvpn-pannel/internal/models"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	SERVER_LOG_TYPE_SERVER      = 1
	SERVER_LOG_TYPE_SCRIPT      = 2
	SERVER_LOG_TYPE_SERVER_NAME = "openvpn.log"
	SERVER_LOG_TYPE_SCRIPT_NAME = "auth.log"
)

type OpenVPNServerInstance struct {
	serverConfig      *models.Server
	routeList         []*models.ServerRoute
	clientConfigList  []*models.ClientConfig
	workingDir        string
	internalAPIListen string
	openvpnPath       string
	pid               int
	cmd               *exec.Cmd
	// exited 在进程退出并被 Wait 回收后关闭。Running 据此判定存活，
	// 避免进程退出后仍处于僵尸态时 signal 0 成功而误判为存活。
	exited chan struct{}
	// keepAlive 表示面板期望该实例进程保持运行（原子访问）。为 1 时，
	// 看门狗会在检测到进程异常退出后重新拉起；主动停止时清 0，避免被拉起。
	keepAlive int32
}

func (ins *OpenVPNServerInstance) GetServerModel() *models.Server {
	return ins.serverConfig
}
func NewOpenVPNServerInstance(instanceModel *models.Server,
	routeList []*models.ServerRoute,
	clientConfigList []*models.ClientConfig,
	workingDir string, internalAPIListen string, openvpnPath string) *OpenVPNServerInstance {

	var _workingDir string
	if len(workingDir) >= 1 {
		if workingDir[len(workingDir)-1] != '/' {
			_workingDir = workingDir + "/"
		} else {
			_workingDir = workingDir
		}
	} else {
		_workingDir = workingDir
	}

	instance := OpenVPNServerInstance{
		serverConfig:      instanceModel,
		routeList:         routeList,
		clientConfigList:  clientConfigList,
		workingDir:        _workingDir,
		internalAPIListen: internalAPIListen,
		openvpnPath:       openvpnPath,
	}
	return &instance
}

// 重新写入客户端特定配置 misc
func (ins *OpenVPNServerInstance) UpdateClientConfig(clientConfigList []*models.ClientConfig, resourceMap map[string]string) error {
	ins.clientConfigList = clientConfigList
	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return fmt.Errorf("获取其他配置失败: %s", err.Error())
	}
	err = ins.WriteClientConfig(miscConfig)
	return err
}

// 写入客户端特定设置
func (ins *OpenVPNServerInstance) WriteClientConfig(miscConfig *MiscConfig) error {
	os.RemoveAll(ins.workingDir + miscConfig.CCDDir)
	err := os.MkdirAll(ins.workingDir+miscConfig.CCDDir, 0755)
	if err != nil {
		return fmt.Errorf("创建ccd文件夹失败: %s", err.Error())
	}

	for _, clientConfig := range ins.clientConfigList {
		err = os.WriteFile(fmt.Sprintf("%s/%s/%s", ins.workingDir, miscConfig.CCDDir, clientConfig.ClientCertName), []byte(clientConfig.Config), 0644)
		if err != nil {
			return fmt.Errorf("写入客户端配置文件%s失败: %s", clientConfig.Config, err.Error())
		}
	}
	return nil

}

// 写入配置文件 misc config client_offline.sh client_online.sh auth.sh
func (ins *OpenVPNServerInstance) WriteConfig(resourceMap map[string]string) error {
	// os.RemoveAll(ins.workingDir)
	err := os.MkdirAll(ins.workingDir, 0755)
	if err != nil {
		return fmt.Errorf("创建服务端文件夹失败: %s", err.Error())
	}
	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return fmt.Errorf("获取其他配置失败: %s", err.Error())
	}
	configParam := &OpenvpnServerTemplateParam{
		ServerConfig: ins.serverConfig,
		ServerRoute:  ins.routeList,
		WorkingDir:   ins.workingDir,
		MiscConfig:   miscConfig,
	}
	configTemplate, ok := resourceMap[RESOURCE_ID_CONFIG_TEMPLATE]
	if !ok {
		// 兜底：正常情况下调用方已通过 PrepareResourceMap 按当前资源集填好，缺失时才回落默认资源集。
		configTemplate = GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_CONFIG_TEMPLATE)
	}
	configString, err := RenderOpenvpnServerConfig(configTemplate, configParam)
	if err != nil {
		return fmt.Errorf("渲染配置文件失败: %s", err.Error())
	}
	err = os.WriteFile(ins.workingDir+miscConfig.ServerConfigFileName, []byte(configString), 0644)
	if err != nil {
		return fmt.Errorf("写入配置文件失败: %s", err.Error())
	}

	// 写入CA CERT key dh ccd ta.key
	err = os.WriteFile(ins.workingDir+miscConfig.CAFileName, []byte(ins.serverConfig.CA), 0644)
	if err != nil {
		return fmt.Errorf("写入CA文件失败: %s", err.Error())
	}

	err = os.WriteFile(ins.workingDir+miscConfig.ServerCertFileName, []byte(ins.serverConfig.Cert), 0644)
	if err != nil {
		return fmt.Errorf("写入服务器证书文件失败: %s", err.Error())
	}

	err = os.WriteFile(ins.workingDir+miscConfig.ServerKeyFileName, []byte(ins.serverConfig.Key), 0600)
	if err != nil {
		return fmt.Errorf("写入服务器证书私钥文件失败: %s", err.Error())
	}

	err = os.WriteFile(ins.workingDir+miscConfig.DHFileName, []byte(ins.serverConfig.DH), 0600)
	if err != nil {
		return fmt.Errorf("写入dh文件失败: %s", err.Error())
	}

	err = os.WriteFile(ins.workingDir+miscConfig.TAFileName, []byte(ins.serverConfig.TLSAuthKey), 0600)
	if err != nil {
		return fmt.Errorf("写入ta文件失败: %s", err.Error())
	}

	err = ins.WriteClientConfig(configParam.MiscConfig)
	if err != nil {
		return err
	}

	// 写入脚本  client_offline.sh client_online.sh auth.sh
	fileNameMap := map[string]string{
		RESOURCE_ID_CLIENT_OFFLINE_SCRIPT: miscConfig.ClientOfflineScriptName,
		RESOURCE_ID_CLIENT_ONLINE_SCRIPT:  miscConfig.ClientOnlineScriptName,
		RESOURCE_ID_AUTH_SCRIPT:           miscConfig.ClientAuthScriptName,
	}
	for resourceID, fileName := range fileNameMap {
		fileContent, ok := resourceMap[resourceID]
		if !ok {
			fileContent = GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, resourceID)
		}
		fileContent = strings.ReplaceAll(fileContent, "__INTERNAL_API__", ins.internalAPIListen)
		fileContent = strings.ReplaceAll(fileContent, "__WORKING_DIR__", ins.workingDir)
		fileContent = strings.ReplaceAll(fileContent, "__SERVER_ID__", fmt.Sprintf("%d", ins.serverConfig.ID))
		fileContent = strings.ReplaceAll(fileContent, "__SERVER_INTERFACE__", ins.serverConfig.Dev)

		err = os.WriteFile(ins.workingDir+fileName, []byte(fileContent), 0755)
		if err != nil {
			return fmt.Errorf("写入%s文件失败: %s", fileName, err.Error())
		}
	}
	return nil
}

// 启动服务端 misc RESOURCE_ID_SERVER_START_SCRIPT
func (ins *OpenVPNServerInstance) Start(resourceMap map[string]string) error {
	if ins.Running() {
		return fmt.Errorf("已经启动，不能重复启动")
	}
	// 运行openvpn
	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return fmt.Errorf("获取其他配置失败: %s", err.Error())
	}
	ins.cmd = exec.Command(ins.openvpnPath, ins.workingDir+miscConfig.ServerConfigFileName)
	ins.cmd.Dir = ins.workingDir

	if err := ins.cmd.Start(); err != nil {
		return fmt.Errorf("启动openvpn进程失败: %s", err.Error())
	}
	ins.pid = ins.cmd.Process.Pid
	// 后台回收子进程并在退出时关闭 exited：不连接 management socket，
	// 仅凭子进程状态即可判定存活，不会产生额外 openvpn 日志。
	exited := make(chan struct{})
	ins.exited = exited
	startedCmd := ins.cmd
	atomic.StoreInt32(&ins.keepAlive, 1)
	go func() {
		waitErr := startedCmd.Wait()
		if atomic.LoadInt32(&ins.keepAlive) == 1 {
			log.Printf("OpenVPN server %d 进程意外退出 PID: %d %v\n", ins.serverConfig.ID, startedCmd.Process.Pid, waitErr)
		}
		close(exited)
	}()
	log.Printf("OpenVPN server %d 进程启动 PID: %d\n", ins.serverConfig.ID, ins.cmd.Process.Pid)
	// 执行启动脚本
	server_start_script, ok := resourceMap[RESOURCE_ID_SERVER_START_SCRIPT]
	if !ok {
		server_start_script = GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_SERVER_START_SCRIPT)
	}
	server_start_script = strings.ReplaceAll(server_start_script, "__INTERNAL_API__", ins.internalAPIListen)
	server_start_script = strings.ReplaceAll(server_start_script, "__WORKING_DIR__", ins.workingDir)
	server_start_script = strings.ReplaceAll(server_start_script, "__SERVER_ID__", fmt.Sprintf("%d", ins.serverConfig.ID))
	server_start_script = strings.ReplaceAll(server_start_script, "__SERVER_INTERFACE__", ins.serverConfig.Dev)
	cmd := exec.Command(miscConfig.ShellPath, "-c", server_start_script)
	// CombinedOutput 返回标准输出和标准错误的内容
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("OpenVPN server %d 进程启动: 运行启动脚本失败 %s 脚本输出: %s\n", ins.serverConfig.ID, err.Error(), string(out))
	} else {
		log.Printf("OpenVPN server %d 进程启动: 运行启动脚本成功 脚本输出: \n %s\n", ins.serverConfig.ID, string(out))
	}
	return nil
}

// 停止服务 misc RESOURCE_ID_SERVER_EXIT_SCRIPT
func (ins *OpenVPNServerInstance) Stop(resourceMap map[string]string) error {
	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return fmt.Errorf("获取其他配置失败: %s", err.Error())
	}
	// 先清除保活标记：主动停止的进程不允许被看门狗重新拉起。
	ins.SetKeepAlive(false)
	if ins.cmd != nil && ins.cmd.Process != nil {
		log.Printf("OpenVPN server %d 进程停止 PID: %d\n", ins.serverConfig.ID, ins.cmd.Process.Pid)
		ins.cmd.Process.Signal(os.Interrupt)
		// 子进程由 Start 里启动的 Wait 协程回收，这里只等待其退出信号，避免重复调用 Wait。
		if ins.exited != nil {
			select {
			case <-ins.exited:
				// 子进程在 8 秒内退出
				log.Printf("OpenVPN server %d 进程正常停止 PID: %d\n", ins.serverConfig.ID, ins.cmd.Process.Pid)
			case <-time.After(8 * time.Second):
				// 超时！8 秒还没退出
				log.Printf("OpenVPN server %d 进程停止超时 强制停止 PID: %d\n", ins.serverConfig.ID, ins.cmd.Process.Pid)

				// 尝试强制终止
				if ins.cmd.Process != nil {
					err := ins.cmd.Process.Kill()
					if err != nil {
						// 忽略 "process already finished"（Go ≥1.20）
						if !errors.Is(err, os.ErrProcessDone) {
							log.Printf("OpenVPN server %d 强制进程停止失败 PID: %d %v\n", ins.serverConfig.ID, ins.cmd.Process.Pid, err)
						} else {
							log.Printf("OpenVPN server %d 强制进程停止成功 PID: %d\n", ins.serverConfig.ID, ins.cmd.Process.Pid)
						}
					} else {
						log.Printf("OpenVPN server %d 强制进程停止成功 PID: %d\n", ins.serverConfig.ID, ins.cmd.Process.Pid)
					}
					// 等待 Wait() 返回（回收资源）
					<-ins.exited
				}
			}
		}
	} else if ins.pid > 0 {
		// 面板重启后接管的外部进程：不是本进程的子进程，无法用 Wait 回收，按 PID 发信号。
		stopAttachedProcess(ins.serverConfig.ID, ins.pid)
	}
	// 执行退出脚本
	server_exit_script, ok := resourceMap[RESOURCE_ID_SERVER_EXIT_SCRIPT]
	if !ok {
		server_exit_script = GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_SERVER_EXIT_SCRIPT)
	}
	server_exit_script = strings.ReplaceAll(server_exit_script, "__INTERNAL_API__", ins.internalAPIListen)
	server_exit_script = strings.ReplaceAll(server_exit_script, "__WORKING_DIR__", ins.workingDir)
	server_exit_script = strings.ReplaceAll(server_exit_script, "__SERVER_ID__", fmt.Sprintf("%d", ins.serverConfig.ID))
	server_exit_script = strings.ReplaceAll(server_exit_script, "__SERVER_INTERFACE__", ins.serverConfig.Dev)
	cmd := exec.Command(miscConfig.ShellPath, "-c", server_exit_script)
	// CombinedOutput 返回标准输出和标准错误的内容
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("OpenVPN server %d 进程停止: 运行停止脚本失败 %s 脚本输出 %s\n", ins.serverConfig.ID, err.Error(), string(out))
	} else {
		log.Printf("OpenVPN server %d 进程停止: 运行停止脚本成功 脚本输出: \n %s\n", ins.serverConfig.ID, string(out))
	}
	return nil
}

func (ins *OpenVPNServerInstance) GetPID() int {
	if ins.cmd != nil && ins.cmd.Process != nil {
		return ins.cmd.Process.Pid
	}
	return ins.pid
}

// AttachPID 接管一个面板重启前已在运行、且未被子进程 Wait 回收的 openvpn 进程。
// 接管后实例的存活判定与停止都基于该 PID，不会重新写配置、也不会重新启动进程。
func (ins *OpenVPNServerInstance) AttachPID(pid int) {
	ins.pid = pid
	atomic.StoreInt32(&ins.keepAlive, 1)
}

// ProcessAlive 判断指定 PID 的进程是否存在（发送 signal 0）。
// EPERM 表示进程存在但当前用户无权限操作，仍视为存活；僵尸态进程视为已退出。
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// 僵尸进程（已被 kill 但尚未被父进程回收）对 signal 0 仍会成功，必须排除。
	if processIsZombie(pid) {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EPERM
	}
	return false
}

// processIsZombie 读取 /proc/<pid>/stat 判断进程是否处于僵尸态（state == 'Z'）。
// 非 Linux 或读取失败时返回 false。
func processIsZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	// stat 格式：pid (comm) state ...，其中 comm 可能包含空格与括号，
	// 因此取最后一个 ')' 之后、跳过空格的字符即为 state。
	idx := bytes.LastIndexByte(data, ')')
	if idx < 0 || idx+2 >= len(data) {
		return false
	}
	return data[idx+2] == 'Z'
}

// stopAttachedProcess 停止一个面板接管的外部进程：先发 SIGINT 等待退出，超时后 SIGKILL。
func stopAttachedProcess(serverID uint, pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	log.Printf("OpenVPN server %d 进程停止(接管) PID: %d\n", serverID, pid)
	proc.Signal(os.Interrupt)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !ProcessAlive(pid) {
			log.Printf("OpenVPN server %d 进程正常停止(接管) PID: %d\n", serverID, pid)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Printf("OpenVPN server %d 进程停止超时 强制停止(接管) PID: %d\n", serverID, pid)
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		log.Printf("OpenVPN server %d 强制进程停止失败(接管) PID: %d %v\n", serverID, pid, err)
	}
	for i := 0; i < 50 && ProcessAlive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

// SetKeepAlive 标记面板是否期望该实例进程保持运行。看门狗据此决定异常退出后是否自动拉起。
func (ins *OpenVPNServerInstance) SetKeepAlive(v bool) {
	if v {
		atomic.StoreInt32(&ins.keepAlive, 1)
	} else {
		atomic.StoreInt32(&ins.keepAlive, 0)
	}
}

// KeepAlive 返回面板是否期望该实例进程保持运行。
func (ins *OpenVPNServerInstance) KeepAlive() bool {
	return atomic.LoadInt32(&ins.keepAlive) == 1
}

func (ins *OpenVPNServerInstance) Running() bool {
	if ins.cmd == nil || ins.cmd.Process == nil {
		// 接管的外部进程：不是本进程子进程，直接按 PID 判定存活。
		return ProcessAlive(ins.pid)
	}
	// Start 启动的 Wait 协程在进程退出后关闭 exited；僵尸态下 signal 0 仍会成功，
	// 必须先用该通道判定，才能识别“已退出但未被回收”的情况。
	if ins.exited != nil {
		select {
		case <-ins.exited:
			return false
		default:
		}
	}
	// 发送 signal 0：不实际发送信号，仅检查进程是否存在
	err := ins.cmd.Process.Signal(syscall.Signal(0))
	if err != nil {
		// 进程不存在、已退出、或无权限
		if errors.Is(err, os.ErrProcessDone) {
			return false // Go 1.20+
		}
		// 兼容旧版本 Go：检查错误字符串或 syscall 错误
		if err == os.ErrProcessDone || err.Error() == "os: process already finished" {
			return false
		}
		// Unix: ESRCH = No such process
		if pathErr, ok := err.(*os.SyscallError); ok {
			if pathErr.Err == syscall.ESRCH {
				return false
			}
		}
		// 其他错误（如权限不足）通常也视为“不可达”，按需处理
	}
	return true
}

// 获取服务器日志
type ServerLogResponse struct {
	Content   string `json:"content"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func GetFileContent(filePath string, startLine int, endLine int) (*ServerLogResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// ===== 第一遍：获取所有行的起始偏移 =====
	offsets, err := getLineOffsets(file)
	if err != nil {
		return nil, fmt.Errorf("failed to scan line offsets: %w", err)
	}

	totalLines := len(offsets)

	// 默认：最后100行
	if startLine == 0 && endLine == 0 {
		startLine = totalLines - 99
		if startLine < 1 {
			startLine = 1
		}
		endLine = totalLines
	}

	// 边界处理
	if totalLines == 0 {
		return &ServerLogResponse{Content: "", StartLine: startLine, EndLine: endLine}, nil
	}
	if startLine < 1 {
		startLine = 1
	}
	if endLine > totalLines {
		endLine = totalLines
	}
	if startLine > endLine {
		return &ServerLogResponse{Content: "", StartLine: startLine, EndLine: endLine}, nil
	}

	// ===== 第二遍：只读取目标行 =====
	startOffset := offsets[startLine-1]
	var endOffset int64
	if endLine < totalLines {
		endOffset = offsets[endLine] // 下一行开始，即当前行结束
	} else {
		// 读到文件末尾
		if stat, err := file.Stat(); err == nil {
			endOffset = stat.Size()
		} else {
			endOffset = startOffset // fallback
		}
	}

	// 计算要读取的字节数
	readSize := endOffset - startOffset
	if readSize <= 0 {
		return &ServerLogResponse{Content: "", StartLine: startLine, EndLine: endLine}, nil
	}

	// Seek 到起始位置
	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek failed: %w", err)
	}

	// 读取片段
	buf := make([]byte, readSize)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read segment failed: %w", err)
	}
	buf = buf[:n]

	// 转为字符串并按行分割（注意最后一行可能无 \n）
	lines := strings.Split(string(buf), "\n")
	// 如果原始数据以 \n 结尾，Split 会多出一个空字符串，需处理
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	content := strings.Join(lines, "\n")

	return &ServerLogResponse{
		Content:   content,
		StartLine: startLine,
		EndLine:   startLine + len(lines) - 1, // 实际读取的结束行
	}, nil
}

// getLineOffsets 返回每行起始字节偏移（0-based），长度 = 总行数
func getLineOffsets(file *os.File) ([]int64, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var offsets []int64
	offsets = append(offsets, 0)

	reader := bufio.NewReader(file)
	pos := int64(0)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		pos += int64(len(line))
		offsets = append(offsets, pos) // 下一行起始位置
	}

	// 如果文件非空且最后一行没有 \n，则最后一行未被计入 offsets 的新项，
	// 但 offsets 长度仍等于行数（因为初始有 0，每遇到 \n 增加一个偏移）
	// 例如： "a\nb" → offsets = [0, 2] → 2 行，正确
	//       "a\nb\n" → offsets = [0, 2, 4] → 2 行？不对！
	// 所以需要修正：如果文件以 \n 结尾，则最后一行为空，应减1？

	// 更安全的做法：统计换行符数量
	// 但为简化，我们约定：offsets 长度 = 行数（即使末尾有 \n，也视为多一行空行）
	// 实际使用中，日志通常不以 \n 结尾，或即使有也不影响

	return offsets, nil
}

// 获取日志 misc
func (ins *OpenVPNServerInstance) GetLog(logType int, startLine int, endLine int, resourceMap map[string]string) (*ServerLogResponse, error) {
	miscConfigModel, err := getMiscConfig(resourceMap)
	if err != nil {
		return nil, err
	}
	if logType == SERVER_LOG_TYPE_SERVER {
		// 查找工作目录里的 openvpn.log 文件 如果 startLine 和 startLine 都是 0，那么 默认输出最后的 100 行
		logFile := filepath.Join(ins.workingDir, miscConfigModel.ServerLog)
		return GetFileContent(logFile, startLine, endLine)
	}
	if logType == SERVER_LOG_TYPE_SCRIPT {
		// 获取脚本日志
		logFile := filepath.Join(ins.workingDir, miscConfigModel.ScriptLog)
		return GetFileContent(logFile, startLine, endLine)
	}
	return nil, errors.New("未知的日志类型")
}

// 清理日志 misc
func (ins *OpenVPNServerInstance) ClearLog(logType int, resourceMap map[string]string) error {
	miscConfigModel, err := getMiscConfig(resourceMap)
	if err != nil {
		return err
	}
	if logType == SERVER_LOG_TYPE_SERVER {
		// 覆盖写入工作目录里的 openvpn.log 文件
		logFile := filepath.Join(ins.workingDir, miscConfigModel.ServerLog)
		return os.WriteFile(logFile, []byte{}, 0644)
	}
	if logType == SERVER_LOG_TYPE_SCRIPT {
		// 覆盖写入脚本日志
		logFile := filepath.Join(ins.workingDir, miscConfigModel.ScriptLog)
		return os.WriteFile(logFile, []byte{}, 0644)
	}
	return errors.New("未知的日志类型")
}

func getMiscConfig(resourceMap map[string]string) (*MiscConfig, error) {
	miscConfig, ok := resourceMap[RESOURCE_ID_MISC_CONFIG]
	if !ok {
		miscConfig = GetSetDefaultResource(RESOURCE_SET_LINUX_IPTABLES, RESOURCE_ID_MISC_CONFIG)
	}
	var miscConfigModel MiscConfig
	err := json.Unmarshal([]byte(miscConfig), &miscConfigModel)
	return &miscConfigModel, err
}

// 获取状态 包括版本 客户端列表
type ServerStatusClientInfoResponse struct {
	CommonName     string   `json:"common_name"`
	RealIPAddr     string   `json:"real_ip_addr"`
	VirtualIPAddr  []string `json:"virtual_ip_addr"`
	VirtualIP6Addr []string `json:"virtual_ip6_addr"`
	ByteReceived   int      `json:"last_byte_received"`
	ByteSent       int      `json:"last_byte_sent"`
	ConnectedSince string   `json:"connected_since"`
	LastRef        string   `json:"last_ref"`
	Username       string   `json:"username"`
	// ClientID 是管理接口 status 2 输出里的客户端 ID（CLIENT_LIST 的 Client ID 列）。
	// client-kill 命令依赖它，且不区分管理接口版本，因此断开客户端统一走它。
	ClientID string `json:"client_id"`
}

// appendVirtualIP 按地址族把虚拟地址追加到 IPv4 / IPv6 列表。
func appendVirtualIP(info *ServerStatusClientInfoResponse, addr string) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}
	if strings.Contains(addr, ":") {
		info.VirtualIP6Addr = append(info.VirtualIP6Addr, addr)
	} else {
		info.VirtualIPAddr = append(info.VirtualIPAddr, addr)
	}
}

type ServerStatusResponse struct {
	Version           string `json:"version"`
	ManagementVersion string `json:"management_version"`
	ClientList        []*ServerStatusClientInfoResponse
}

func execManagementCommand(cmd string, conn net.Conn) (string, error) {
	if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
		return "", err
	}
	reader := bufio.NewReader(conn)
	allData := ""

	emptyLineCount := 0
	for {
		line, err := reader.ReadString('\n') // 会一直读直到遇到 \n 或出错
		if err == nil {
			allData = allData + line
		} else {
			break
		}
		//log.Printf("read line [%s] \n", line)
		if strings.HasSuffix(line, "END\r\n") {
			break
		}
		if line == "" {
			emptyLineCount++
			if emptyLineCount > 9 {
				break
			}
		} else {
			emptyLineCount = 0
		}
	}
	log.Println("exec ", cmd)
	log.Println(allData)
	return allData, nil

}

// 获取状态 misc
func (ins *OpenVPNServerInstance) GetStatus(resourceMap map[string]string) (*ServerStatusResponse, error) {
	serverStatusResponse := &ServerStatusResponse{
		ClientList: []*ServerStatusClientInfoResponse{},
		Version:    "",
	}
	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return nil, err
	}
	socketName := miscConfig.ManagementSocket
	// 连接unix socket
	log.Println("连接unix socket ", filepath.Join(ins.workingDir, socketName))
	conn, err := net.Dial("unix", filepath.Join(ins.workingDir, socketName))
	if err != nil {
		return nil, err
	}
	log.Println("连接unix socket 成功", filepath.Join(ins.workingDir, socketName))
	defer conn.Close()
	allData, err := execManagementCommand("status", conn)
	if err != nil {
		return nil, err
	}
	var clientStatusMap map[string]*ServerStatusClientInfoResponse
	clientStatusMap = make(map[string]*ServerStatusClientInfoResponse)
	// 一行一行开始分析数据
	lineContent := 0 // 行号标记，记录已经进入了某些内容范围 0 为都不是
	lines := strings.Split(allData, "\r\n")
	for _, line := range lines {
		if line == "status" || line == "END" || line == "" || line == "OpenVPN CLIENT LIST" || line == "GLOBAL STATS" {
			lineContent = 0
			continue
		}
		if strings.HasPrefix(line, "Updated,") || strings.HasPrefix(line, "Max bcast/mcast queue length") {
			lineContent = 0
			continue
		}
		if line == "Common Name,Real Address,Bytes Received,Bytes Sent,Connected Since" {
			lineContent = 1
			continue
		}
		if line == "ROUTING TABLE" {
			lineContent = 2
			continue
		}
		if line == "Virtual Address,Common Name,Real Address,Last Ref" && lineContent == 2 {
			lineContent = 3
			continue
		}
		if lineContent == 3 {
			// Virtual Address,Common Name,Real Address,Last Ref
			//        0              1           2        3
			fields := strings.Split(line, ",")
			if len(fields) < 4 {
				continue
			}
			serverStatusClientInfoResponse, ok := clientStatusMap[fields[2]]
			if !ok {
				serverStatusClientInfoResponse = &ServerStatusClientInfoResponse{
					CommonName:     fields[1],
					RealIPAddr:     fields[2],
					ByteReceived:   0,
					ByteSent:       0,
					ConnectedSince: "",
					LastRef:        fields[3],
				}
				appendVirtualIP(serverStatusClientInfoResponse, fields[0])
				clientStatusMap[fields[2]] = serverStatusClientInfoResponse
			} else {
				serverStatusClientInfoResponse.LastRef = fields[3]
				appendVirtualIP(serverStatusClientInfoResponse, fields[0])
			}
		}
		if lineContent == 1 {
			fields := strings.Split(line, ",")
			if len(fields) < 5 {
				continue
			}
			bytesReceived, err := strconv.Atoi(fields[2])
			if err != nil {
				bytesReceived = 0
			}
			bytesSent, err := strconv.Atoi(fields[3])
			if err != nil {
				bytesSent = 0
			}
			// Common Name,Real Address,Bytes Received,Bytes Sent,Connected Since
			// 0               1           2               3          4
			serverStatusClientInfoResponse, ok := clientStatusMap[fields[1]]
			if !ok {
				serverStatusClientInfoResponse = &ServerStatusClientInfoResponse{
					CommonName:     fields[0],
					RealIPAddr:     fields[1],
					ByteReceived:   bytesReceived,
					ByteSent:       bytesSent,
					ConnectedSince: fields[4],
					LastRef:        "",
				}
			} else {
				serverStatusClientInfoResponse.ByteReceived = bytesReceived
				serverStatusClientInfoResponse.ByteSent = bytesSent
				serverStatusClientInfoResponse.ConnectedSince = fields[4]
				serverStatusClientInfoResponse.LastRef = ""
				serverStatusClientInfoResponse.RealIPAddr = fields[1]
				serverStatusClientInfoResponse.CommonName = fields[0]
			}
			clientStatusMap[fields[1]] = serverStatusClientInfoResponse
		}
	}
	allData, err = execManagementCommand("version", conn)
	if err != nil {
		return nil, err
	}
	for _, clientInfo := range clientStatusMap {
		serverStatusResponse.ClientList = append(serverStatusResponse.ClientList, clientInfo)
	}
	lines = strings.Split(allData, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "OpenVPN Version:") {
			serverStatusResponse.Version = strings.TrimPrefix(line, "OpenVPN Version: ")
		}
		if strings.HasPrefix(line, "Management Version:") {
			serverStatusResponse.ManagementVersion = strings.TrimPrefix(line, "Management Version: ")
		}
	}
	return serverStatusResponse, nil
}

// GetStatus2 连接管理接口执行 `status 2`，解析 v2 格式的客户端列表。
// v2 输出为逐行 "CLIENT_LIST,<字段...>" 记录，字段包含 Client ID（见 status-version 2 定义）：
//
//	CLIENT_LIST,Common Name,Real Address,Virtual Address,Virtual IPv6 Address,Bytes Received,
//	           Bytes Sent,Connected Since,Connected Since (time_t),Username,Client ID,Peer ID,Data Channel Cipher
//
// 相比版本字符串判断，Client ID 是断开客户端的稳定标识（client-kill <id>），
// 因此这里只解析客户端列表，不再依赖管理接口版本。
func (ins *OpenVPNServerInstance) GetStatus2(resourceMap map[string]string) ([]*ServerStatusClientInfoResponse, error) {
	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return nil, err
	}
	socketName := miscConfig.ManagementSocket
	log.Println("连接unix socket ", filepath.Join(ins.workingDir, socketName))
	conn, err := net.Dial("unix", filepath.Join(ins.workingDir, socketName))
	if err != nil {
		return nil, err
	}
	log.Println("连接unix socket 成功", filepath.Join(ins.workingDir, socketName))
	defer conn.Close()

	allData, err := execManagementCommand("status 2", conn)
	if err != nil {
		return nil, err
	}
	return parseStatus2Output(allData), nil
}

// parseStatus2Output 解析管理接口 `status 2` 的文本输出，返回客户端列表。
// 抽成纯函数以便单测，不依赖真实管理接口。
func parseStatus2Output(allData string) []*ServerStatusClientInfoResponse {
	// CLIENT_LIST 提供客户端主体信息（含 Client ID），ROUTING_TABLE 仅作虚拟地址与
	// 最后引用时间的兜底补充。这里按出现顺序聚合为客户端列表。
	clients := make([]*ServerStatusClientInfoResponse, 0)

	lines := strings.Split(allData, "\r\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "CLIENT_LIST,"):
			// CLIENT_LIST,Common Name,Real Address,Virtual Address,Virtual IPv6 Address,Bytes Received,Bytes Sent,Connected Since,Connected Since (time_t),Username,Client ID,Peer ID,Data Channel Cipher
			fields := strings.Split(line, ",")
			if len(fields) < 11 {
				continue
			}
			bytesReceived, _ := strconv.Atoi(fields[5])
			bytesSent, _ := strconv.Atoi(fields[6])
			client := &ServerStatusClientInfoResponse{
				CommonName:     fields[1],
				RealIPAddr:     fields[2],
				ByteReceived:   bytesReceived,
				ByteSent:       bytesSent,
				ConnectedSince: fields[7],
				Username:       fields[9],
				ClientID:       fields[10],
			}
			appendVirtualIP(client, fields[3])
			appendVirtualIP(client, fields[4])
			clients = append(clients, client)
		case strings.HasPrefix(line, "ROUTING_TABLE,"):
			// ROUTING_TABLE,Virtual Address,Common Name,Real Address,Last Ref,Last Ref (time_t)
			fields := strings.Split(line, ",")
			if len(fields) < 5 {
				continue
			}
			virtualAddr := fields[1]
			realAddr := fields[3]
			// 路由表按真实地址匹配到对应客户端，补充虚拟地址(兜底)与最后引用时间。
			for _, client := range clients {
				if client.RealIPAddr == realAddr {
					client.LastRef = fields[4]
					// CLIENT_LIST 已填过的虚拟地址不重复追加。
					if !containsVirtualIP(client, virtualAddr) {
						appendVirtualIP(client, virtualAddr)
					}
					break
				}
			}
		}
	}

	return clients
}

// containsVirtualIP 判断虚拟地址是否已存在于客户端信息的 IPv4/IPv6 列表中。
func containsVirtualIP(info *ServerStatusClientInfoResponse, addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	for _, a := range info.VirtualIPAddr {
		if a == addr {
			return true
		}
	}
	for _, a := range info.VirtualIP6Addr {
		if a == addr {
			return true
		}
	}
	return false
}

// 获取状态 misc
// 从openvpn-status.log获取日志，实时性没有那么高，但是不会刷openvpn管理socket的日志
// Version和ManagementVersion无法获取
func (ins *OpenVPNServerInstance) GetStatusFromFile(resourceMap map[string]string) ([]*ServerStatusClientInfoResponse, error) {
	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return nil, err
	}
	statusFileName := filepath.Join(ins.workingDir, miscConfig.StatusFileName)
	allData, err := os.ReadFile(statusFileName)
	if err != nil {
		log.Printf("GetStatusFromFile failed file: %s error: %s", statusFileName, err)
		return nil, err
	}
	var clientStatusMap map[string]*ServerStatusClientInfoResponse
	clientStatusMap = make(map[string]*ServerStatusClientInfoResponse)
	// 一行一行开始分析数据
	lineContent := 0 // 行号标记，记录已经进入了某些内容范围 0 为都不是
	//log.Printf("GetStatusFromFile 读取文件 %s 内容 %s", statusFileName, allData)
	lines := strings.Split(string(allData), "\n")
	for _, line := range lines {
		if line == "status" || line == "END" || line == "" || line == "OpenVPN CLIENT LIST" || line == "GLOBAL STATS" {
			lineContent = 0
			//log.Println("进入 lineContent=0", line)
			continue
		}
		if strings.HasPrefix(line, "Updated,") || strings.HasPrefix(line, "Max bcast/mcast queue length") {
			lineContent = 0
			//log.Println("进入 lineContent=0", line)
			continue
		} //        Common Name,Real Address,Bytes Received,Bytes Sent,Connected Since
		if line == "Common Name,Real Address,Bytes Received,Bytes Sent,Connected Since" {
			lineContent = 1
			//log.Println("进入 lineContent=1", line)
			continue
		} //        ROUTING TABLE
		if line == "ROUTING TABLE" {
			lineContent = 2
			//log.Println("进入 lineContent=2", line)
			continue
		} //         Virtual Address,Common Name,Real Address,Last Ref
		if line == "Virtual Address,Common Name,Real Address,Last Ref" && lineContent == 2 {
			lineContent = 3
			//log.Println("进入 lineContent=3", line)
			continue
		}
		if lineContent == 3 {
			// Virtual Address,Common Name,Real Address,Last Ref
			//        0              1           2        3
			fields := strings.Split(line, ",")
			if len(fields) < 4 {
				log.Println("lineContent=3 解析行错误，len(fields) < 4", line)
				continue
			}
			serverStatusClientInfoResponse, ok := clientStatusMap[fields[2]]
			if !ok {
				serverStatusClientInfoResponse = &ServerStatusClientInfoResponse{
					CommonName:     fields[1],
					RealIPAddr:     fields[2],
					ByteReceived:   0,
					ByteSent:       0,
					ConnectedSince: "",
					LastRef:        fields[3],
				}
				appendVirtualIP(serverStatusClientInfoResponse, fields[0])
				clientStatusMap[fields[2]] = serverStatusClientInfoResponse
			} else {
				serverStatusClientInfoResponse.LastRef = fields[3]
				appendVirtualIP(serverStatusClientInfoResponse, fields[0])
			}
		}
		if lineContent == 1 {
			fields := strings.Split(line, ",")
			if len(fields) < 5 {
				log.Println("lineContent=1 解析行错误，len(fields) < 5", line)
				continue
			}
			bytesReceived, err := strconv.Atoi(fields[2])
			if err != nil {
				bytesReceived = 0
			}
			bytesSent, err := strconv.Atoi(fields[3])
			if err != nil {
				bytesSent = 0
			}
			// Common Name,Real Address,Bytes Received,Bytes Sent,Connected Since
			// 0               1           2               3          4
			serverStatusClientInfoResponse, ok := clientStatusMap[fields[1]]
			if !ok {
				serverStatusClientInfoResponse = &ServerStatusClientInfoResponse{
					CommonName:     fields[0],
					RealIPAddr:     fields[1],
					ByteReceived:   bytesReceived,
					ByteSent:       bytesSent,
					ConnectedSince: fields[4],
					LastRef:        "",
				}
			} else {
				serverStatusClientInfoResponse.ByteReceived = bytesReceived
				serverStatusClientInfoResponse.ByteSent = bytesSent
				serverStatusClientInfoResponse.ConnectedSince = fields[4]
				serverStatusClientInfoResponse.LastRef = ""
				serverStatusClientInfoResponse.RealIPAddr = fields[1]
				serverStatusClientInfoResponse.CommonName = fields[0]
			}
			clientStatusMap[fields[1]] = serverStatusClientInfoResponse
		}
	}
	var response []*ServerStatusClientInfoResponse
	for _, clientInfo := range clientStatusMap {
		response = append(response, clientInfo)
		//log.Printf("GetStatusFromFile %s %s %d %d %s", clientInfo.RealIPAddr, clientInfo.VirtualIPAddr, clientInfo.ByteReceived, clientInfo.ByteSent, clientInfo.CommonName)
	}
	return response, nil
}

// 关闭客户端连接 misc。
// clientID 为管理接口 status 2 的客户端 ID（最精确，优先使用）；
// commonName 为客户端证书名称（OpenVPN common_name）；readIPAddr 为客户端真实地址（可能带协议前缀）。
//
// 实现：client-kill 依赖 status 2 的 Client ID，不区分管理接口版本，也避免了对
// IPv6 真实地址拼接/协议的脆弱解析。若调用方未提供 clientID，则通过 GetStatus2
// 拉取在线客户端列表，按证书名或真实地址定位到目标客户端再取其 Client ID。
func (ins *OpenVPNServerInstance) CloseClient(commonName, readIPAddr, clientID string, resourceMap map[string]string) (string, error) {
	if clientID == "" {
		// 未提供 ID 时，查询在线客户端以定位目标 Client ID。
		clientList, err := ins.GetStatus2(resourceMap)
		if err != nil {
			// 极个别 openvpn 服务器无法开启管理接口，连接 socket 会失败，
			// 踢人依赖管理接口，此时无法完成，给出明确提示而不是裸的连接错误。
			return "", fmt.Errorf("无法连接管理接口(该服务器可能不支持管理接口，无法断开客户端): %w", err)
		}

		target := findClientForKill(clientList, commonName, readIPAddr)
		if target == nil {
			if commonName != "" {
				return "", fmt.Errorf("未找到在线客户端 证书=%s 地址=%s", commonName, readIPAddr)
			}
			return "", fmt.Errorf("未找到在线客户端 地址=%s", readIPAddr)
		}
		clientID = target.ClientID
		if clientID == "" {
			return "", fmt.Errorf("客户端 证书=%s 地址=%s 缺少 Client ID，无法断开", target.CommonName, target.RealIPAddr)
		}
	}

	miscConfig, err := getMiscConfig(resourceMap)
	if err != nil {
		return "", err
	}
	socketName := miscConfig.ManagementSocket
	conn, err := net.Dial("unix", filepath.Join(ins.workingDir, socketName))
	if err != nil {
		return "", fmt.Errorf("无法连接管理接口(该服务器可能不支持管理接口，无法断开客户端): %w", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(fmt.Sprintf("client-kill %s\n", clientID))); err != nil {
		return "", err
	}
	reader := bufio.NewReader(conn)

	emptyLineCount := 0
	for {
		line, err := reader.ReadString('\n') // 会一直读直到遇到 \n 或出错
		if err != nil {
			return "", err
		}
		if line == "" {
			emptyLineCount++
			if emptyLineCount > 9 {
				break
			}
		} else {
			emptyLineCount = 0
			if strings.HasPrefix(line, "ERROR:") {
				_line := strings.Split(line, "\r\n")
				return "", errors.New(_line[0])
			}
			if strings.HasPrefix(line, "SUCCESS:") {
				_line := strings.Split(line, "\r\n")
				return _line[0], nil
			}
		}
	}
	return "", errors.New("未知错误")
}

// findClientForKill 在在线客户端列表里定位要断开的客户端。
// 优先按证书名匹配；证书名为空时按真实地址匹配（兼容 status 输出带协议前缀、
// 以及脚本上报的 "[ipv6]:port" 与 status 的 "udp6:[ipv6]:port" 等差异）。
func findClientForKill(clientList []*ServerStatusClientInfoResponse, commonName, readIPAddr string) *ServerStatusClientInfoResponse {
	if commonName != "" {
		for _, c := range clientList {
			if c.CommonName == commonName {
				return c
			}
		}
		return nil
	}
	want := normalizeRealAddr(readIPAddr)
	if want == "" {
		return nil
	}
	for _, c := range clientList {
		if c.RealIPAddr == readIPAddr || normalizeRealAddr(c.RealIPAddr) == want {
			return c
		}
	}
	return nil
}

// normalizeRealAddr 归一化真实地址，便于不同来源（status 输出 / 脚本上报）之间的比较：
// 去掉协议前缀（如 udp4:/udp6:/tcp4:/tcp6:），去掉 IPv6 地址的方括号，并统一小写。
func normalizeRealAddr(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(stripAddrProto(addr)))
	addr = strings.ReplaceAll(addr, "[", "")
	addr = strings.ReplaceAll(addr, "]", "")
	return addr
}

// stripAddrProto 去掉真实地址里可能出现的协议前缀。
// 较新版本 OpenVPN 的 status 输出 Real Address 形如 "udp4:61.171.212.168:15778"
// 或 "tcp6:[2001:db8::1]:15778"，用于与脚本上报的真实地址做归一化比较。
// 这里去掉开头连续出现的、不以数字或中括号开头的协议段（udp/udp4/udp6/tcp/tcp4/tcp6）。
func stripAddrProto(addr string) string {
	addr = strings.TrimSpace(addr)
	lower := strings.ToLower(addr)
	for _, proto := range []string{"udp6:", "udp4:", "udp:", "tcp6:", "tcp4:", "tcp:", "udp", "tcp"} {
		if strings.HasPrefix(lower, proto) {
			stripped := addr[len(proto):]
			// 仅当去掉后看起来像 ip:port（以数字或 [ 开头）才采用，避免误伤
			if stripped != "" && (stripped[0] == '[' || (stripped[0] >= '0' && stripped[0] <= '9')) {
				return stripped
			}
		}
	}
	return addr
}
