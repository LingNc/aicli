package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	debug          bool
	debugLogConsole bool
	mu             sync.Mutex
	logFile        io.WriteCloser // debug 模式持有，非 debug 为 nil
	logPath        string
)

// Init 初始化日志系统。
//   debugFlag: 是否启用调试模式（写入日志文件）
//   showDebug: 调试模式下，debug 级别日志是否同时输出到控制台
//   cmdName: 子命令名（如 "main", "setup"），用于日志文件名
//   fullCmd: 完整命令行，写入日志首行
// 非 debug 模式不创建文件，行为和当前代码一致（直接写 stderr/stdout）。
func Init(debugFlag bool, showDebug bool, cmdName string, fullCmd string) error {
	debug = debugFlag
	debugLogConsole = showDebug

	if !debug {
		return nil
	}

	// 解析日志目录
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取 home 目录失败: %w", err)
	}
	logDir := filepath.Join(home, ".aicli", "logs")

	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 构造文件名: ai_YYYY-MM-DD_HH-MM-SS_<cmdName>.log，同名追加 _1, _2
	ts := time.Now().Format("2006-01-02_15-04-05")
	const maxNameLen = 64
	const fixedOverhead = 3 + 19 + 1 + 4 // ai_ + timestamp + _ + .log = 27
	available := max(maxNameLen-fixedOverhead, 1)
	cmdPart := truncateToBytes(cmdName, available)
	baseName := "ai_" + ts + "_" + cmdPart
	candidate := filepath.Join(logDir, baseName+".log")
	path := candidate
	for i := 1; ; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			logFile = f
			logPath = path
			ts := time.Now().Format("2006-01-02 15:04:05")
			fmt.Fprintf(logFile, "[%s] [CMD] %s\n", ts, fullCmd)
			return nil
		}
		if !os.IsExist(err) {
			return fmt.Errorf("创建日志文件失败: %w", err)
		}
		path = filepath.Join(logDir, fmt.Sprintf("%s_%d.log", baseName, i))
	}
}

// Close 关闭日志文件。如果是 debug 模式，返回日志文件路径；否则返回空字符串。
func Close() string {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	return logPath
}

// IsDebug 返回是否处于调试模式。
func IsDebug() bool {
	return debug
}

// SetDebug 设置调试模式（保留以兼容现有调用）。
func SetDebug(enabled bool) {
	debug = enabled
}

// writeFile 写入日志文件（带时间戳和级别）
func writeFile(level, format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(logFile, "[%s] [%s] %s\n", ts, level, msg)
}

// Debug 输出调试信息。仅在调试模式下写入。
func Debug(format string, args ...interface{}) {
	if !debug {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if debugLogConsole {
		fmt.Fprintf(os.Stderr, "\r[DEBUG] %s\r\n", msg)
	}
	writeFile("DEBUG", "%s", msg)
}

// Info 输出信息到 stderr，始终显示。
// 调试模式下同时写入日志文件（带时间戳）。
func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\r%s\r\n", msg)
	writeFile("INFO", "%s", msg)
}

// Warn 输出警告到 stderr，始终显示。
// 调试模式下同时写入日志文件（带时间戳）。
func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\r%s\r\n", msg)
	writeFile("WARN", "%s", msg)
}

// Error 输出错误到 stderr，始终显示。
// 调试模式下同时写入日志文件（带时间戳）。
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\r%s\r\n", msg)
	writeFile("ERROR", "%s", msg)
}

// Fatal 输出错误并退出（exit code 1）。
func Fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\r%s\r\n", msg)
	writeFile("FATAL", "%s", msg)
	os.Exit(1)
}

// Print 输出到 stdout。用于 help 文本、shell 状态消息、流式命令展示。
// 调试模式下同时写入日志文件（带时间戳），不包含 ANSI 转义序列。
func Print(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		ts := time.Now().Format("2006-01-02 15:04:05")
		fmt.Fprintf(logFile, "[%s] [PRINT] %s\n", ts, msg)
	}
}

// PrintRaw 输出原始内容到 stdout，不加换行、不转义。
func PrintRaw(s string) {
	fmt.Print(s)
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		// PrintRaw 不加换行也不带时间戳，按追加方式写入
		fmt.Fprint(logFile, s)
	}
}

// ClearStderrLine 清除 stderr 当前行（\033[2K\r）。仅控制台，不写日志。
func ClearStderrLine() {
	fmt.Fprint(os.Stderr, "\033[2K\r")
}

// Bell 响铃（\a）。仅控制台，不写日志。
func Bell() {
	fmt.Fprint(os.Stderr, "\a")
}

// truncateToBytes 按 UTF-8 字符边界截断字符串至 maxBytes 字节。
func truncateToBytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		rLen := utf8.RuneLen(r)
		if b.Len()+rLen > maxBytes {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
