package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lingnc/aicli/internal/config"
	"golang.org/x/term"
)

// Confirm 请求用户确认是否执行命令
// 返回: (是否执行, 是否加入白名单, error)
func Confirm(category Category, cfg *config.Config) (bool, bool, error) {
	// 检查 stdin 是否为终端
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "非终端环境，跳过确认")
		return false, false, nil
	}

	// 打印确认提示
	fmt.Fprintf(os.Stderr, "[a/y/N] ")

	fd := int(os.Stdin.Fd())

	// 设置终端为原始模式
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false, false, err
	}
	defer term.Restore(fd, oldState)

	// 信号处理：捕获 SIGINT/SIGTERM，恢复终端后退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		term.Restore(fd, oldState)
		os.Exit(130)
	}()
	defer signal.Stop(sigCh)

	// 读取用户输入
	buf := make([]byte, 1)
	os.Stdin.Read(buf)
	fmt.Fprintln(os.Stderr)

	// 判断用户输入
	switch buf[0] {
	case 'y', 'Y':
		return true, false, nil
	case 'a', 'A':
		return true, true, nil
	default:
		return false, false, nil
	}
}

// Execute 执行命令并返回结果
func Execute(command string) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()

	// 错误处理
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", 1, err
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", exitErr.ExitCode(), nil
		}
		return "", 1, err
	}

	return "", 0, nil
}

// ExtractBaseName 从命令中提取基础命令名
func ExtractBaseName(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
