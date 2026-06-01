package executor

import "strings"

// Category 是命令分类
type Category string

const (
	CatRO       Category = "ro"
	CatRW       Category = "rw"
	CatRM       Category = "rm"
	CatSudoRW   Category = "sudo,rw"
	CatSudoRM   Category = "sudo,rm"
	CatSudoRO   Category = "sudo,ro"
	CatUnknown  Category = ""
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
	StateCommand     ParseState = iota // 累积命令
	StateMetadata                      // 读取分类行
	StateExplanation                   // 读取说明
	StateDone                          // 解析完成
)

// Parser 是流式状态机解析器
type Parser struct {
	state          ParseState
	command        strings.Builder
	metadata       strings.Builder
	explain        strings.Builder
	buf            string // 未消费的缓冲
	Result         *Result
	CurrentCategory Category // 实时更新的分类，在元数据读取完成后设置
}

// NewParser 创建解析器
func NewParser() *Parser {
	return &Parser{state: StateCommand}
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
		case StateCommand:
			// 寻找 \n#$  分隔符
			idx := strings.Index(p.buf, "\n#$ ")
			if idx >= 0 {
				// 输出分隔符之前的命令部分
				newCmd.WriteString(p.buf[:idx])
				p.command.WriteString(p.buf[:idx])
				p.buf = p.buf[idx+4:] // 跳过 "\n#$ "
				p.state = StateMetadata
				continue
			}
			// 没找到分隔符，但检查尾部是否可能是分隔符的前缀
			// 最长可能前缀: "\n#$ " = 4 字符，检查尾部最多 3 个字符
			safe := len(p.buf)
			for k := 1; k <= 3 && k <= len(p.buf); k++ {
				suffix := "\n#$ "[:k]
				if strings.HasSuffix(p.buf, suffix) {
					safe = len(p.buf) - k
					break
				}
			}
			if safe > 0 {
				newCmd.WriteString(p.buf[:safe])
				p.command.WriteString(p.buf[:safe])
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
	if p.state == StateCommand && p.buf != "" {
		p.command.WriteString(p.buf)
		p.buf = ""
	}
	if p.state == StateMetadata && p.buf != "" {
		p.metadata.WriteString(p.buf)
		p.CurrentCategory = Category(strings.TrimSpace(p.metadata.String()))
		p.buf = ""
	}

	p.Result = &Result{
		Command:     strings.TrimSpace(p.command.String()),
		Category:    Category(strings.TrimSpace(p.metadata.String())),
		Explanation: strings.TrimSpace(p.explain.String()),
	}
	return p.Result
}
