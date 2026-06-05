package prompt

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// System 是发给 LLM 的系统提示词（静态，不含动态信息，利于缓存命中）
const System = `你是 aicli，一个运行在用户机器上的终端 AI 助手，你的配置文件位于 ~/.aicli/config.yaml，日志目录位于 ~/.aicli/logs/。你的命令将被执行，必须严格按以下格式输出，不得有任何额外文字：
#$ 命令
#@ 分类
#& 简短说明

格式要求（严格遵守）：
- 命令以 '#$ ' 开头，命令不能以 # 或空格开头。多个命令用 && 连接写一行。
- 分类必须以 '#@ ' 开头：ro(只读)、rw(修改)、rm(删除)、sudo,ro、sudo,rw、sudo,rm
- 简短说明以 '#& ' 开头 ≤15 字
- #$ , #@ 和 #& 必须顶格，前面无空格
- 不要添加 markdown 代码块、解释或其他格式
- 需要给输出有颜色的命令添加颜色标识

接受输入:
1. 系统消息: "#( 系统消息 #)"
2. 用户消息: "user(用户文本)"`

// BuildSystemInfo 收集当前系统信息，构建 system info
func BuildSystemInfo() string {
	var info strings.Builder

	// 操作系统
	osName := runtime.GOOS
	if runtime.GOOS == "linux" {
		// 尝试读取 /etc/os-release 获取发行版名
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				if after, ok :=strings.CutPrefix(line, "PRETTY_NAME="); ok  {
					osName = strings.Trim(after, "\"")
					break
				}
			}
		}
	} else if runtime.GOOS == "darwin" {
		osName = "macOS"
	} else if runtime.GOOS == "windows" {
		osName = "Windows"
	}
	fmt.Fprintf(&info, "OS: %s %s", osName, runtime.GOARCH)

	// Shell
	shell := os.Getenv("SHELL")
	if shell == "" && runtime.GOOS == "windows" {
		shell = os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd"
		}
	}
	if shell != "" {
		fmt.Fprintf(&info, "\nShell: %s", filepath.Base(shell))
	}

	// 当前目录
	if cwd, err := os.Getwd(); err == nil {
		fmt.Fprintf(&info, "\nCWD: %s", cwd)
	}

	// 用户
	if u, err := user.Current(); err == nil {
		fmt.Fprintf(&info, "\nUser: %s", u.Username)
	}

	// 主机名
	if hostname, err := os.Hostname(); err == nil {
		fmt.Fprintf(&info, "\nHost: %s", hostname)
	}

	return info.String()
}