package display

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type ThinkingDisplay struct {
	startTime  time.Time
	spinnerIdx int
	lines      []string
	maxLines   int
	drawnLines int
	active     bool
	stopCh     chan struct{}
	mu         sync.Mutex
}

func NewThinkingDisplay() *ThinkingDisplay {
	return &ThinkingDisplay{
		maxLines: 5,
		stopCh:   make(chan struct{}),
	}
}

func (d *ThinkingDisplay) Start() {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return
	}
	d.startTime = time.Now()
	d.active = true
	go d.spin()
}

func (d *ThinkingDisplay) IsActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active
}

func (d *ThinkingDisplay) FeedReasoning(s string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	// 拆分换行，每行独立存储
	for line := range strings.SplitSeq(s, "\n") {
		if line == "" {
			continue
		}
		d.lines = append(d.lines, line)
	}
	if len(d.lines) > d.maxLines {
		d.lines = d.lines[len(d.lines)-d.maxLines:]
	}
}

func (d *ThinkingDisplay) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active {
		return
	}
	d.active = false
	close(d.stopCh)
	// 清除显示区域
	if d.drawnLines > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dA\r\033[J", d.drawnLines)
	}
}

func (d *ThinkingDisplay) spin() {
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
	fmt.Fprintf(os.Stderr, "\r\033[K-> 思考中[%.1fs] %s\n", elapsed, frame)
	// 绘制推理内容行
	for _, line := range d.lines {
		fmt.Fprintf(os.Stderr, "\r\033[K  %s\n", line)
	}
	d.drawnLines = 1 + len(d.lines)
}
