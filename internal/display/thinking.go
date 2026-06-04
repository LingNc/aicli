package display

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ThinkingDisplay 终端思考动画显示。
//
// 使用固定大小的行缓冲区，手动控制每行宽度，不依赖终端自动换行。
// rows 最多 maxLines 行，每行显示宽度不超过 effWidth。
type ThinkingDisplay struct {
	startTime  time.Time
	spinnerIdx int
	rows       [][]rune
	maxLines   int
	maxLineLen int
	effWidth   int
	rendered   bool
	active     bool
	stopCh     chan struct{}
	mu         sync.Mutex
	wg         sync.WaitGroup
	termWidth  int
}

// NewThinkingDisplay 创建思考显示组件。
func NewThinkingDisplay(maxLines, maxLineLen int) *ThinkingDisplay {
	if maxLines <= 0 {
		maxLines = 3
	}
	if maxLineLen <= 0 {
		maxLineLen = 30
	}
	return &ThinkingDisplay{
		maxLines:   maxLines,
		maxLineLen: maxLineLen,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动 spinner 动画。
func (d *ThinkingDisplay) Start() {
	fd := int(os.Stderr.Fd())
	if !term.IsTerminal(fd) {
		return
	}
	d.termWidth, _, _ = term.GetSize(fd)
	if d.termWidth <= 0 {
		d.termWidth = 80
	}
	d.effWidth = d.termWidth
	if d.maxLineLen > 0 && d.maxLineLen < d.effWidth {
		d.effWidth = d.maxLineLen
	}
	d.startTime = time.Now()
	d.active = true
	d.wg.Add(1)
	go d.spin()
}

// IsActive 是否正在显示。
func (d *ThinkingDisplay) IsActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active
}

// FeedReasoning 流式追加推理内容。
func (d *ThinkingDisplay) FeedReasoning(s string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	for _, ch := range s {
		if ch == '\n' {
			if len(d.rows) == 0 || len(d.rows[len(d.rows)-1]) > 0 {
				d.advanceRow()
			}
			continue
		}
		if len(d.rows) == 0 {
			d.rows = append(d.rows, []rune{})
		}
		last := d.rows[len(d.rows)-1]
		w := runeWidth(ch)
		curWidth := rowWidth(last)
		if len(last) > 0 && curWidth+w > d.effWidth {
			d.advanceRow()
			last = d.rows[len(d.rows)-1]
		}
		d.rows[len(d.rows)-1] = append(last, ch)
	}
}

// advanceRow 前进到下一行。缓冲区满时滚动。
func (d *ThinkingDisplay) advanceRow() {
	if len(d.rows) < d.maxLines {
		d.rows = append(d.rows, []rune{})
	} else {
		d.rows = append(d.rows[1:], []rune{})
	}
}

// rowWidth 计算一行 rune 的终端显示宽度。
func rowWidth(row []rune) int {
	w := 0
	for _, r := range row {
		w += runeWidth(r)
	}
	return w
}

// Stop 停止 spinner 并清除显示。
func (d *ThinkingDisplay) Stop() {
	d.mu.Lock()
	if !d.active {
		d.mu.Unlock()
		return
	}
	d.active = false
	close(d.stopCh)
	rendered := d.rendered
	d.mu.Unlock()

	// 等待 spin goroutine 退出（持锁等待会与 render 的锁死锁）
	d.wg.Wait()
	if rendered {
		fmt.Fprintf(os.Stderr, "\033[%dA\r\033[J", d.maxLines+1)
	}
}

func (d *ThinkingDisplay) spin() {
	defer d.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.render()
		}
	}
}

// render 始终输出 maxLines+1 行。
func (d *ThinkingDisplay) render() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	elapsed := time.Since(d.startTime).Seconds()
	frame := spinnerFrames[d.spinnerIdx%len(spinnerFrames)]
	d.spinnerIdx++
	if d.rendered {
		fmt.Fprintf(os.Stderr, "\033[%dA", d.maxLines+1)
	}
	fmt.Fprintf(os.Stderr, "\r\033[K%s 思考中[%.1fs]\n", frame, elapsed)
	for i := 0; i < d.maxLines; i++ {
		if i < len(d.rows) {
			fmt.Fprintf(os.Stderr, "\r\033[K\033[38;5;245m  %s\033[0m\n", string(d.rows[i]))
		} else {
			fmt.Fprintf(os.Stderr, "\r\033[K\n")
		}
	}
	d.rendered = true
}

// runeWidth 返回 rune 的终端显示宽度（CJK/全角=2，其他=1）。
func runeWidth(r rune) int {
	if r < 0x20 {
		return 0
	}
	if isCJK(r) {
		return 2
	}
	return 1
}

func isCJK(r rune) bool {
	return r >= 0x1100 && r <= 0x115f || // Hangul Jamo
		r >= 0x2e80 && r <= 0x303e || // CJK Radicals, Kangxi, Symbols
		r >= 0x3040 && r <= 0x33bf || // Hiragana, Katakana, CJK Compat
		r >= 0x3400 && r <= 0x4dbf || // CJK Extension A
		r >= 0x4e00 && r <= 0xa4cf || // CJK Unified Ideographs
		r >= 0xac00 && r <= 0xd7a3 || // Hangul Syllables
		r >= 0xf900 && r <= 0xfaff || // CJK Compat Ideographs
		r >= 0xfe10 && r <= 0xfe6f || // CJK Compat Forms
		r >= 0xff01 && r <= 0xff60 || // Fullwidth Forms
		r >= 0xffe0 && r <= 0xffe6 || // Fullwidth Signs
		r >= 0x20000 && r <= 0x2fa1f // CJK Extensions B-H
}
