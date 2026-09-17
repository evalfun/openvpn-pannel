package ovpnserver

import (
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

// 归档文件名后缀时间戳，精确到纳秒，避免同一秒内多次轮换相互覆盖
const logArchiveTimeLayout = "20060102-150405.000000000"

// copyTruncate 把 src 的全部内容复制到 dst(新归档)，然后把 src 截断为 0。
// 采用“复制+截断”而非“重命名”，是为了兼容被进程持有文件描述符的日志
// （如 openvpn 持续写 openvpn.log）：截断后该 fd 仍可继续在原路径追加，
// 只是位置被清零，日志不会写到已改名的旧 inode 上。
func copyTruncate(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Truncate(src, 0)
}

type logFileInfo struct {
	path   string
	name   string
	size   int64
	mod    time.Time
	active bool
}

// RotateLogsInDir 在 dir 目录下对若干活动日志做容量轮换，返回 (生成归档数, 删除文件数, err)。
//   - activeLogs: 活动日志文件名(如 openvpn.log / auth.log)；其历史归档须以 "<活动名>.<时间戳>" 命名。
//   - maxBytes: 该服务器日志总量的上限(达到即触发)；<=0 视为关闭功能，直接返回。
//
// 触发后：
//  1. 对每个非空活动日志 copytruncate 生成时间戳归档(活动文件清零)；
//  2. 反复删除“最早”的归档，使活动+归档总量 <= maxBytes 的 80%，但始终保留最新一个归档以保住最近日志。
func RotateLogsInDir(dir string, activeLogs []string, maxBytes int64) (int, int, error) {
	if maxBytes <= 0 {
		return 0, 0, nil
	}
	if !strings.HasSuffix(dir, "/") {
		dir = dir + "/"
	}
	targetBytes := maxBytes * 80 / 100

	collect := func() ([]logFileInfo, int64) {
		var files []logFileInfo
		var total int64
		seen := map[string]bool{}
		add := func(name string, active bool) {
			if name == "" || seen[name] {
				return
			}
			seen[name] = true
			st, err := os.Stat(dir + name)
			if err != nil || st.IsDir() {
				return
			}
			files = append(files, logFileInfo{path: dir + name, name: name, size: st.Size(), mod: st.ModTime(), active: active})
			total += st.Size()
		}
		for _, a := range activeLogs {
			add(a, true)
		}
		if dents, err := os.ReadDir(dir); err == nil {
			for _, d := range dents {
				if d.IsDir() {
					continue
				}
				n := d.Name()
				for _, a := range activeLogs {
					if a != "" && strings.HasPrefix(n, a+".") {
						add(n, false)
						break
					}
				}
			}
		}
		return files, total
	}

	files, total := collect()
	if total < maxBytes {
		return 0, 0, nil
	}
	totalBefore := total

	// (1) 轮换：把活动日志 copytruncate 成带时间戳的归档
	stamp := time.Now().Format(logArchiveTimeLayout)
	rotated := 0
	for _, a := range activeLogs {
		if a == "" {
			continue
		}
		st, err := os.Stat(dir + a)
		if err != nil || st.Size() == 0 {
			continue
		}
		if err := copyTruncate(dir+a, dir+a+"."+stamp); err != nil {
			log.Printf("日志轮换失败 %s: %v", dir+a, err)
			continue
		}
		rotated++
	}

	// (2) 重新统计并按“最早优先”删除归档，回收至 <= 80%
	files, total = collect()
	var archives []logFileInfo
	for _, f := range files {
		if !f.active {
			archives = append(archives, f)
		}
	}
	// 旧 -> 新（mtime 相同则按名字，保证确定性）
	sort.Slice(archives, func(i, j int) bool {
		if archives[i].mod.Equal(archives[j].mod) {
			return archives[i].name < archives[j].name
		}
		return archives[i].mod.Before(archives[j].mod)
	})

	deleted := 0
	// 至少保留最新的一个归档(archives 末尾)，避免把最近日志全删光
	for i := 0; i < len(archives)-1 && total > targetBytes; i++ {
		if err := os.Remove(archives[i].path); err != nil {
			log.Printf("删除归档日志失败 %s: %v", archives[i].path, err)
			continue
		}
		total -= archives[i].size
		deleted++
	}

	if rotated > 0 || deleted > 0 {
		log.Printf("日志轮换: dir=%s 触发前总量=%dB 阈值=%dB 生成归档=%d 删除=%d 轮换后总量=%dB",
			dir, totalBefore, maxBytes, rotated, deleted, total)
	}
	return rotated, deleted, nil
}

// RotateLogsFromMisc 供上层定时任务调用：从 misc 资源解析出本服务器的活动日志文件名，执行轮换。
func (ins *OpenVPNServerInstance) RotateLogsFromMisc(resourceMap map[string]string, maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}
	misc, err := getMiscConfig(resourceMap)
	if err != nil {
		return err
	}
	activeLogs := []string{misc.ServerLog, misc.ScriptLog}
	_, _, err = RotateLogsInDir(ins.workingDir, activeLogs, maxBytes)
	return err
}
