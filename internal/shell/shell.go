package shell

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lingnc/aicli/internal/log"
	"golang.org/x/term"
)

// wrapper 是要注入到 shell rc 文件的 wrapper 函数
const wrapper = `# >>> ai shell integration >>>
[[ ":$PATH:" != *":$HOME/.local/bin:"* ]] && export PATH="$HOME/.local/bin:$PATH"
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

// installSystem 使用 sudo 安装二进制到 /usr/local/bin/
func installSystem() error {
	src, err := findSelf()
	if err != nil {
		return fmt.Errorf("获取当前程序路径失败: %w", err)
	}
	dst := "/usr/local/bin/aicli"
	if src == dst {
		log.Print("-> 已安装在 %s", dst)
		return nil
	}
	cmd := exec.Command("sudo", "install", "-m", "0755", src, dst)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("系统安装失败: %w", err)
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

// selectInstallScope 在 raw mode 下显示箭头选择界面。
// 返回: "user" (为自己安装) 或 "system" (为所有人安装)
func selectInstallScope() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "user", nil
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "user", nil
	}

	// 信号处理：Ctrl-C 时恢复终端
	done := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	go func() {
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
			fmt.Fprintf(os.Stderr, "\033[?25h") // 显示光标
			term.Restore(fd, oldState)
			fmt.Fprintf(os.Stderr, "\r\033[K")
			os.Exit(130)
		case <-done:
		}
	}()

	options := []string{
		"为所有人安装 (需要 root)",
		"为自己安装",
		"取消 [q]",
	}
	selected := 0

	// 隐藏光标
	fmt.Fprintf(os.Stderr, "\033[?25l")

	// 首次绘制
	drawMenu(options, selected)

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}

		switch {
		case buf[0] == '\x1b' && n >= 3 && buf[1] == '[':
			// 方向键
			switch buf[2] {
			case 'A': // 上
				selected = (selected - 1 + len(options)) % len(options)
				drawMenu(options, selected)
			case 'B': // 下
				selected = (selected + 1) % len(options)
				drawMenu(options, selected)
			}
		case buf[0] == '\r' || buf[0] == '\n': // Enter
			// 清空菜单
			fmt.Fprintf(os.Stderr, "\033[%dA\033[J\033[?25h", len(options)-1)
			signal.Stop(sigCh)
			close(done)
			term.Restore(fd, oldState)
			if selected == 2 { // 取消
				return "", fmt.Errorf("-> 取消")
			}
			if selected == 0 {
				return "system", nil
			}
			return "user", nil
		case buf[0] == 'q' || buf[0] == '\x03': // q 或 Ctrl-C
			fmt.Fprintf(os.Stderr, "\033[J\033[?25h")
			signal.Stop(sigCh)
			close(done)
			term.Restore(fd, oldState)
			return "", fmt.Errorf("用户取消")
		default:
			log.Bell()
		}
	}

	fmt.Fprintf(os.Stderr, "\033[?25h") // 显示光标
	signal.Stop(sigCh)
	close(done)
	term.Restore(fd, oldState)
	return "user", nil
}

// drawMenu 绘制选择菜单
func drawMenu(options []string, selected int) {
	for i, opt := range options {
		prefix := "   "
		if i == selected {
			prefix = "-> "
		}
		fmt.Fprintf(os.Stderr, "\r\033[K%s%s\n", prefix, opt)
	}
	// 光标回到第一行
	fmt.Fprintf(os.Stderr, "\033[%dA", len(options))
}

// Install 将 wrapper 函数注入到 shell rc 文件并安装二进制
func Install() error {
	// 1. 选择安装范围
	scope, err := selectInstallScope()
	if err != nil {
		return err
	}

	// 2. 按选择安装二进制
	if scope == "system" {
		if err := installSystem(); err != nil {
			return err
		}
	} else {
		installDir, err := resolveInstallDir()
		if err != nil {
			return err
		}
		if err := installBinary(installDir); err != nil {
			return err
		}
	}

	// 3. 注入 wrapper 到 shell rc 文件
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
	if strings.Contains(content, installMarker) {
		log.Print("-> wrapper 已存在于 %s", rcPath)
		return nil
	}

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