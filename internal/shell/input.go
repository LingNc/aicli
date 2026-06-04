package shell

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lingnc/aicli/internal/log"
)

// KeyEvent 是从统一输入读取器派发的按键事件。
// Raw 包含完整的字节序列：可打印字符 1 字节，转义序列 2+ 字节（如 \033[A）。
type KeyEvent struct {
	Raw []byte
}

// InputReader 拥有从 stdin 读取所有字节的单一 goroutine。
// DSR 响应在带内被拦截并路由到待处理的光标查询。
// 普通按键事件通过 Keys() 派发。
type InputReader struct {
	keys      chan KeyEvent
	cursorReq chan chan [2]int
	done      chan struct{}
}

// NewInputReader 创建新的 InputReader。需要调用 Start() 开始读取。
func NewInputReader() *InputReader {
	return &InputReader{
		keys:      make(chan KeyEvent),
		cursorReq: make(chan chan [2]int, 1),
		done:      make(chan struct{}),
	}
}

// Keys 返回主循环消费的按键事件通道。
func (r *InputReader) Keys() <-chan KeyEvent { return r.keys }

// QueryCursorPos 请求当前光标行号。
// 非 debug 模式立即返回 0（零开销，不发送 DSR）。
// debug 模式通过读取 goroutine 发送 DSR 请求，等待最多 50ms。
func (r *InputReader) QueryCursorPos() (int, int) {
	if !log.IsDebug() {
		return 0, 0
	}
	respCh := make(chan [2]int, 1)
	select {
	case r.cursorReq <- respCh:
	default:
		return 0, 0
	}
	select {
	case pos := <-respCh:
		return pos[0], pos[1]
	case <-time.After(120 * time.Millisecond):
		return -1, -1
	}
}

// DebugLog 查询光标位置并输出调试日志，格式: cursor=(row,col) <message>
// 非 debug 模式零开销。
func (r *InputReader) DebugLog(format string, args ...any) {
	row, col := r.QueryCursorPos()
	log.Debug("cursor=(%d,%d) %s", row, col, fmt.Sprintf(format, args...))
}

// Start 启动单一 stdin 读取 goroutine。
func (r *InputReader) Start() {
	go r.readLoop()
}

// Stop 通知读取 goroutine 退出。
func (r *InputReader) Stop() {
	close(r.done)
}

// readLoop 是从 stdin 读取所有字节的单一 goroutine。
// 解析 ANSI 转义序列，将 DSR 响应路由到待处理的光标查询，
// 将按键事件派发到 Keys() 通道。
func (r *InputReader) readLoop() {
	const (
		stNormal = iota
		stEscape
		stCSI
	)
	state := stNormal
	var csiBuf []byte
	var pendingResp chan [2]int
	buf := make([]byte, 1)

	for {
		if pendingResp == nil {
			select {
			case respCh := <-r.cursorReq:
				pendingResp = respCh
				fmt.Fprintf(os.Stderr, "\033[6n")
			default:
			}
		}
		select {
		case <-r.done:
			return
		default:
		}
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return
		}
		b := buf[0]
		switch state {
		case stNormal:
			if b == 0x1b {
				state = stEscape
			} else {
				r.emit(KeyEvent{Raw: []byte{b}})
			}
		case stEscape:
			if b == '[' {
				state = stCSI
				csiBuf = csiBuf[:0]
			} else {
				r.emit(KeyEvent{Raw: []byte{0x1b, b}})
				state = stNormal
			}
		case stCSI:
			switch {
			case b >= 0x30 && b <= 0x3f:
				csiBuf = append(csiBuf, b)
			case b >= 0x40 && b <= 0x7e:
				if b == 'R' && pendingResp != nil {
					row, col := parseDSRPos(csiBuf)
					pendingResp <- [2]int{row, col}
					pendingResp = nil
				} else {
					full := buildCSISequence(csiBuf, b)
					r.emit(KeyEvent{Raw: full})
				}
				state = stNormal
			}
		}
	}
}

func (r *InputReader) emit(ev KeyEvent) {
	select {
	case r.keys <- ev:
	case <-r.done:
	}
}

func buildCSISequence(params []byte, final byte) []byte {
	seq := make([]byte, 0, 2+len(params)+1)
	seq = append(seq, 0x1b, '[')
	seq = append(seq, params...)
	seq = append(seq, final)
	return seq
}

func parseDSRPos(buf []byte) (int, int) {
	s := string(buf)
	if i := strings.IndexByte(s, ';'); i > 0 {
		row, _ := strconv.Atoi(s[:i])
		col, _ := strconv.Atoi(s[i+1:])
		return row, col
	}
	return 0, 0
}
