package shell

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/lingnc/aicli/internal/log"
)

// wrapper 是要注入到 shell rc 文件的 wrapper 函数
const wrapper = `# >>> ai shell integration >>>
ai() {
    local tmpfile="/tmp/ai-cmd-$$.txt"
    command aicli "$@"
    local rc=$?
    if [ -f "$tmpfile" ]; then
        local cmd
        cmd=$(cat "$tmpfile")
        rm -f "$tmpfile"
        if [ -n "$cmd" ]; then
            history -s "$cmd"
        fi
    fi
    return $rc
}
# <<< ai shell integration <<<`

// installMarker 标记开始
const installMarker = "# >>> ai shell integration >>>"

// uninstallMarker 标记结束
const uninstallMarker = "# <<< ai shell integration <<<"

// detectShell 检测当前 shell 类型
func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "bash"
	}
	if strings.Contains(shell, "zsh") {
		return "zsh"
	}
	return "bash"
}

// rcFilePath 返回对应 shell 的 rc 文件路径
func rcFilePath(shell string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("获取用户信息失败: %w", err)
	}
	switch shell {
	case "zsh":
		return filepath.Join(usr.HomeDir, ".zshrc"), nil
	default:
		return filepath.Join(usr.HomeDir, ".bashrc"), nil
	}
}

// findSelf 返回当前可执行文件的真实路径（解析 symlink）
func findSelf() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// resolveInstallDir 返回用户级安装目录 ~/.local/bin/，不存在则创建
func resolveInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建安装目录失败: %w", err)
	}
	return dir, nil
}

// checkPath 检查目录是否在 PATH 中
func checkPath(dir string) bool {
	pathEnv := os.Getenv("PATH")
	for _, p := range filepath.SplitList(pathEnv) {
		if p == dir {
			return true
		}
	}
	return false
}

// installBinary 将当前二进制复制到目标目录
func installBinary(dstDir string) error {
	src, err := findSelf()
	if err != nil {
		return fmt.Errorf("获取当前程序路径失败: %w", err)
	}
	dst := filepath.Join(dstDir, "aicli")

	// 源和目标相同则跳过
	if src == dst {
		log.Print("-> 已安装在 %s", dst)
		return nil
	}

	// 读取源文件
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("读取程序文件失败: %w", err)
	}

	// 写入临时文件再原子替换，防止写入中断导致二进制损坏
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0755); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("替换 %s 失败: %w", dst, err)
	}

	log.Print("-> 已安装到 %s", dst)
	return nil
}

// uninstallBinary 从安装目录移除二进制
func uninstallBinary() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dst := filepath.Join(home, ".local", "bin", "aicli")
	if err := os.Remove(dst); err == nil {
		log.Print("-> 已移除 %s", dst)
	}
}

// Install 将 wrapper 函数注入到 shell rc 文件并安装二进制
func Install() error {
	// 1. 安装二进制到 ~/.local/bin/
	installDir, err := resolveInstallDir()
	if err != nil {
		return err
	}
	if err := installBinary(installDir); err != nil {
		return err
	}
	if !checkPath(installDir) {
		log.Warn("-> %s 不在 PATH 中，请将以下内容加入 shell 配置文件:", installDir)
		log.Warn("   export PATH=\"$HOME/.local/bin:$PATH\"")
	}

	// 2. 注入 wrapper 到 shell rc 文件
	shell := detectShell()
	rcPath, err := rcFilePath(shell)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(rcPath)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", rcPath, err)
	}

	content := string(data)

	// 检查是否已安装
	if strings.Contains(content, installMarker) {
		log.Print("-> 已安装到 %s", rcPath)
		return nil
	}

	// 追加 wrapper
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("打开 %s 失败: %w", rcPath, err)
	}
	defer f.Close()

	if _, err := f.WriteString("\n" + wrapper + "\n"); err != nil {
		return fmt.Errorf("写入 wrapper 失败: %w", err)
	}

	log.Print("-> 已安装到 %s，请运行 source %s 或重新打开终端", rcPath, rcPath)
	return nil
}

// Uninstall 从 shell rc 文件中移除 wrapper 并卸载二进制
func Uninstall() error {
	shell := detectShell()
	rcPath, err := rcFilePath(shell)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(rcPath)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", rcPath, err)
	}

	content := string(data)

	// 检查是否已安装
	if !strings.Contains(content, installMarker) {
		log.Print("-> 未找到 ai shell 集成，跳过")
	} else {
		// 找到并删除标记包围的内容
		start := strings.Index(content, installMarker)
		end := strings.Index(content, uninstallMarker)
		if start == -1 || end == -1 {
			return fmt.Errorf("标记格式错误")
		}
		// 包括标记行本身
		end += len(uninstallMarker)

		newContent := content[:start] + content[end:]
		// 清理尾部多余空行
		newContent = strings.TrimRight(newContent, "\n")

		if err := os.WriteFile(rcPath, []byte(newContent), 0600); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", rcPath, err)
		}

		log.Print("-> 已从 %s 移除", rcPath)
	}

	// 移除二进制
	uninstallBinary()

	return nil
}