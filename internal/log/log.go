package log

import (
	"fmt"
	"os"
)

// debug 全局调试开关
var debug bool

// SetDebug 设置调试模式
func SetDebug(enabled bool) {
	debug = enabled
}

// IsDebug 返回调试模式状态
func IsDebug() bool {
	return debug
}

// Debug 输出调试日志（仅 debug 模式）
func Debug(format string, args ...interface{}) {
	if debug {
		fmt.Fprintf(os.Stderr, "\r[DEBUG] "+format+"\n", args...)
	}
}

// Info 输出普通日志到 stderr
func Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "\r"+format+"\n", args...)
}
