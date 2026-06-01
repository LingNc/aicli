package prompt

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// System 是发给 LLM 的系统 prompt 模板
// %s 会被替换为系统信息
const System = `你是终端助手，用户需要你执行命令。你的命令将被执行，必须严格按以下格式输出，不得有任何额外文字：
#$ 命令
#@ 分类
#& 简短说明

格式要求（严格遵守）：
- 命令以 '#$ ' 开头，命令不能以 # 或空格开头。多个命令用 && 连接写一行。
- 分类以 '#@ ' 开头：ro(只读)、rw(修改)、rm(删除)、sudo,ro、sudo,rw、sudo,rm
- 简短说明以 '#& ' 开头 ≤30 字
- #$ , #@ 和 #& 必须顶格，前面无空格
- 不要添加 markdown 代码块、解释或其他格式

当前系统环境:
%s`

// BuildSystemInfo 收集当前系统信息，构建 system prompt
func BuildSystemInfo() string {
	var info strings.Builder

	// 操作系统
	osName := runtime.GOOS
	if runtime.GOOS == "linux" {
		// 尝试读取 /etc/os-release 获取发行版名
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					osName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					break
				}
			}
		}
	} else if runtime.GOOS == "darwin" {
		osName = "macOS"
	} else if runtime.GOOS == "windows" {
		osName = "Windows"
	}
	info.WriteString(fmt.Sprintf("系统: %s", osName))

	// 架构
	info.WriteString(fmt.Sprintf(" %s", runtime.GOARCH))

	// Shell
	shell := os.Getenv("SHELL")
	if shell == "" && runtime.GOOS == "windows" {
		shell = os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd"
		}
	}
	if shell != "" {
		info.WriteString(fmt.Sprintf("\nShell: %s", filepath.Base(shell)))
	}

	// 当前目录
	if cwd, err := os.Getwd(); err == nil {
		info.WriteString(fmt.Sprintf("\n目录: %s", cwd))
	}

	// 用户
	if u, err := user.Current(); err == nil {
		info.WriteString(fmt.Sprintf("\n用户: %s", u.Username))
	}

	// 主机名
	if hostname, err := os.Hostname(); err == nil {
		info.WriteString(fmt.Sprintf("\n主机: %s", hostname))
	}

	return info.String()
}