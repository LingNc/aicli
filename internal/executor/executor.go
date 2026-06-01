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
	fmt.Fprintf(os.Stderr, "-> 请确认[a/y/N] ")

	fd := int(os.Stdin.Fd())

	// 设置终端为原始模式
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false, false, err
	}
	defer func() {
		term.Restore(fd, oldState)
	}()

	// 信号处理：捕获 SIGINT/SIGTERM，恢复终端后退出
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			term.Restore(fd, oldState)
			os.Exit(130)
		case <-done:
			// 正常返回，goroutine 退出
		}
	}()
	defer func() {
		close(done)
		signal.Stop(sigCh)
	}()

	// 读取用户输入
	buf := make([]byte, 1)
	os.Stdin.Read(buf)

	// 判断用户输入
	switch buf[0] {
	case 'y', 'Y':
		// 清空当前行：\033[2K 清除整行，\r 回到行首
		fmt.Fprint(os.Stderr, "\033[2K\r")
		return true, false, nil
	case 'a', 'A':
		// 清空当前行：\033[2K 清除整行，\r 回到行首
		fmt.Fprint(os.Stderr, "\033[2K\r")
		return true, true, nil
	default:
		// 清空当前行：\033[2K 清除整行，\r 回到行首
		fmt.Fprint(os.Stderr, "\033[2K\r")
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
