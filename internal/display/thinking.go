package display

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ThinkingDisplay 终端思考动画显示
type ThinkingDisplay struct {
	startTime  time.Time
	spinnerIdx int
	lineBufs   [][]rune // 每行是一个 rune 切片，逐字符追加
	maxLines   int      // 最大显示行数
	drawnLines int
	active     bool
	stopCh     chan struct{}
	mu         sync.Mutex
	wg         sync.WaitGroup
}

// NewThinkingDisplay 创建思考显示组件
func NewThinkingDisplay(maxLines int) *ThinkingDisplay {
	if maxLines <= 0 {
		maxLines = 3
	}
	return &ThinkingDisplay{
		maxLines: maxLines,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动 spinner 动画
func (d *ThinkingDisplay) Start() {
	fd := int(os.Stderr.Fd())
	if !term.IsTerminal(fd) {
		return
	}
	d.startTime = time.Now()
	d.active = true
	d.wg.Add(1)
	go d.spin()
}

// IsActive 是否正在显示
func (d *ThinkingDisplay) IsActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active
}

// FeedReasoning 流式追加推理内容
func (d *ThinkingDisplay) FeedReasoning(s string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	for _, ch := range s {
		if ch == '\n' {
			// 开始新行（不创建空行）
			if len(d.lineBufs) == 0 || len(d.lineBufs[len(d.lineBufs)-1]) > 0 {
				d.lineBufs = append(d.lineBufs, nil)
			}
			// 如果当前行已经是空的（nil），不再追加
			continue
		}
		if len(d.lineBufs) == 0 {
			d.lineBufs = append(d.lineBufs, nil)
		}
		last := &d.lineBufs[len(d.lineBufs)-1]
		*last = append(*last, ch)
	}
	// 滚动：保留最后 maxLines 行
	if len(d.lineBufs) > d.maxLines {
		d.lineBufs = d.lineBufs[len(d.lineBufs)-d.maxLines:]
	}
}

// Stop 停止并清除显示
func (d *ThinkingDisplay) Stop() {
	d.mu.Lock()
	if !d.active {
		d.mu.Unlock()
		return
	}
	d.active = false
	close(d.stopCh)
	drawn := d.drawnLines
	d.mu.Unlock()

	// 等待 spin goroutine 退出（持锁等待会与 render 的锁死锁）
	d.wg.Wait()

	d.mu.Lock()
	if drawn > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dA\r\033[J", drawn)
	}
	d.mu.Unlock()
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

func (d *ThinkingDisplay) render() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	// 回退已绘制的行
	if d.drawnLines > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dA", d.drawnLines)
	}
	elapsed := time.Since(d.startTime).Seconds()
	frame := spinnerFrames[d.spinnerIdx%len(spinnerFrames)]
	d.spinnerIdx++
	// 绘制计时器 + spinner
	fmt.Fprintf(os.Stderr, "\r\033[K%s 思考中[%.1fs]\n", frame, elapsed)
	// 绘制推理内容行
	for _, buf := range d.lineBufs {
		fmt.Fprintf(os.Stderr, "\r\033[K  %s\n", string(buf))
	}
	d.drawnLines = 1 + len(d.lineBufs)
	// log.Debug("think render: lines=%d drawn=%d maxlen=%d", len(d.lineBufs), d.drawnLines, d.maxLineLen)
}
