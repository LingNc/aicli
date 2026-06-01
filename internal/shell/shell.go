package shell

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// wrapper 是要注入到 shell rc 文件的 wrapper 函数
const wrapper = `# >>> ai shell integration >>>
ai() {
    local tmpfile="/tmp/ai-cmd-$$.txt"
    command ai "$@"
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

// Install 将 wrapper 函数注入到 shell rc 文件
func Install() error {
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
		fmt.Printf("✓ 已安装到 %s\n", rcPath)
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

	fmt.Printf("✓ 已安装到 %s，请运行 source %s 或重新打开终端\n", rcPath, rcPath)
	return nil
}

// Uninstall 从 shell rc 文件中移除 wrapper
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
		fmt.Printf("未找到 ai shell 集成，跳过\n")
		return nil
	}

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

	fmt.Printf("✓ 已从 %s 移除\n", rcPath)
	return nil
}