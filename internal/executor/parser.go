package executor

import "strings"

// Category 是命令分类
type Category string

const (
	CatRO      Category = "ro"
	CatRW      Category = "rw"
	CatRM      Category = "rm"
	CatSudoRW  Category = "sudo,rw"
	CatSudoRM  Category = "sudo,rm"
	CatSudoRO  Category = "sudo,ro"
	CatUnknown Category = ""
)

// Result 是解析结果
type Result struct {
	Command     string   // 命令内容
	Category    Category // 分类
	Explanation string   // 说明
}

// ParseState 是解析器状态
type ParseState int

const (
	StatePrefix      ParseState = iota // 等待 #$ 前缀（3 字节），跳过不输出
	StateCommand                        // 累积命令（从 #$ 之后到 \n#@ 之前），流式输出
	StateMetadata                       // 读取分类行
	StateExplanation                    // 读取说明
	StateDone                           // 解析完成
)

const (
	prefixMarker    = "#$ "  // 命令行前缀（3 字节）
	prefixMarkerLen = 3
	metaMarker      = "\n#@ " // 元数据分隔符（4 字节）
	metaMarkerLen   = 4
)

// Parser 是流式状态机解析器
type Parser struct {
	state           ParseState
	command         strings.Builder
	metadata        strings.Builder
	explain         strings.Builder
	buf             string // 未消费的缓冲
	Result          *Result
	CurrentCategory Category // 实时更新的分类，在元数据读取完成后设置
	CurrentCommand  string   // 实时更新的命令内容，在 StateCommand 状态中累积
}

// NewParser 创建解析器
func NewParser() *Parser {
	return &Parser{state: StatePrefix}
}

// CommandDone 命令是否已完整解析（状态已过 StateCommand，分类已确定）
func (p *Parser) CommandDone() bool {
	return p.state > StateCommand
}

// Feed 输入一个 chunk，返回本次新增的命令文本（用于流式输出）
func (p *Parser) Feed(chunk string) string {
	if p.state == StateDone {
		return ""
	}

	p.buf += chunk
	var newCmd strings.Builder

	for len(p.buf) > 0 {
		switch p.state {
		case StatePrefix:
			// 累积到至少 prefixMarkerLen 字节再判断（可能跨 chunk）
			if len(p.buf) < prefixMarkerLen {
				return ""
			}
			if p.buf[:prefixMarkerLen] == prefixMarker {
				// 正常：跳过 #$ 前缀，不输出
				p.buf = p.buf[prefixMarkerLen:]
				p.state = StateCommand
				continue
			}
			// LLM 没有按格式输出 #$ 前缀
			// 直接进入 Command 状态，把当前 buf 当作命令内容
			p.state = StateCommand
			continue

		case StateCommand:
			// 寻找 \n#@  分隔符
			idx := strings.Index(p.buf, metaMarker)
			if idx >= 0 {
				// 输出分隔符之前的命令部分
				newCmd.WriteString(p.buf[:idx])
				p.command.WriteString(p.buf[:idx])
				p.CurrentCommand = p.command.String()
				p.buf = p.buf[idx+metaMarkerLen:] // 跳过 "\n#@ "
				p.state = StateMetadata
				continue
			}
			// 没找到分隔符，但检查尾部是否可能是分隔符的前缀
			// 最长可能前缀: "\n#@" = 3 字符，检查尾部最多 3 个字符
			safe := len(p.buf)
			for k := 1; k < metaMarkerLen && k <= len(p.buf); k++ {
				suffix := metaMarker[:k]
				if strings.HasSuffix(p.buf, suffix) {
					safe = len(p.buf) - k
					break
				}
			}
			if safe > 0 {
				newCmd.WriteString(p.buf[:safe])
				p.command.WriteString(p.buf[:safe])
				p.CurrentCommand = p.command.String()
				p.buf = p.buf[safe:]
			}
			// 剩余字符保留在 buf 中等待更多数据
			return newCmd.String()

		case StateMetadata:
			// 读取到行尾
			idx := strings.Index(p.buf, "\n")
			if idx >= 0 {
				p.metadata.WriteString(p.buf[:idx])
				p.CurrentCategory = Category(strings.TrimSpace(p.metadata.String()))
				p.buf = p.buf[idx+1:]
				p.state = StateExplanation
				continue
			}
			// 没有换行，继续累积
			p.metadata.WriteString(p.buf)
			p.buf = ""
			return newCmd.String()

		case StateExplanation:
			// 读取说明到末尾
			p.explain.WriteString(p.buf)
			p.buf = ""
			p.state = StateDone
			p.Result = &Result{
				Command:     strings.TrimSpace(p.command.String()),
				Category:    Category(strings.TrimSpace(p.metadata.String())),
				Explanation: strings.TrimSpace(p.explain.String()),
			}
			return newCmd.String()
		}
	}

	return newCmd.String()
}

// Finish 标记输入结束，处理残余数据
func (p *Parser) Finish() *Result {
	if p.state == StateDone {
		return p.Result
	}

	// 处理残余
	switch p.state {
	case StatePrefix:
		// 残余的不足 3 字节视为空，丢弃
		p.buf = ""
	case StateCommand:
		if p.buf != "" {
			p.command.WriteString(p.buf)
			p.CurrentCommand = p.command.String()
			p.buf = ""
		}
	case StateMetadata:
		if p.buf != "" {
			p.metadata.WriteString(p.buf)
			p.CurrentCategory = Category(strings.TrimSpace(p.metadata.String()))
			p.buf = ""
		}
	}

	p.Result = &Result{
		Command:     strings.TrimSpace(p.command.String()),
		Category:    Category(strings.TrimSpace(p.metadata.String())),
		Explanation: strings.TrimSpace(p.explain.String()),
	}
	return p.Result
}
