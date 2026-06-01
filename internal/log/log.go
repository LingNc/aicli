package log

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// debug 全局调试开关
var debug bool

// raw mode 控制
var (
	rawMu   sync.Mutex
	rawMode bool
	msgCh   = make(chan string, 200)
)

func init() {
	go flusher()
}

// flusher 后台 goroutine，负责输出日志
// raw mode 期间消息排队，恢复后按序输出
func flusher() {
	for msg := range msgCh {
		// 等待 raw mode 结束
		for {
			rawMu.Lock()
			isRaw := rawMode
			rawMu.Unlock()
			if !isRaw {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Fprint(os.Stderr, msg)
	}
}

// EnterRawMode 进入 raw mode，日志输出将排队等待
func EnterRawMode() {
	rawMu.Lock()
	rawMode = true
	rawMu.Unlock()
}

// ExitRawMode 退出 raw mode，排队的日志将自动输出
func ExitRawMode() {
	rawMu.Lock()
	rawMode = false
	rawMu.Unlock()
}

// SetDebug 设置调试模式
func SetDebug(enabled bool) {
	debug = enabled
}

// IsDebug 返回调试模式状态
func IsDebug() bool {
	return debug
}

// Debug 输出调试日志（仅 debug 模式，异步非阻塞）
func Debug(format string, args ...interface{}) {
	if debug {
		msg := "\r[DEBUG] " + fmt.Sprintf(format, args...) + "\r\n"
		msgCh <- msg
	}
}

// Info 输出普通日志到 stderr（异步非阻塞）
func Info(format string, args ...interface{}) {
	msg := "\r" + fmt.Sprintf(format, args...) + "\r\n"
	msgCh <- msg
}
