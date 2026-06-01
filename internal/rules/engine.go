package rules

import (
	"fmt"
	"strings"

	"github.com/lingnc/aicli/internal/config"
	"github.com/lingnc/aicli/internal/executor"
)

// Verdict 规则引擎判定结果
type Verdict int

const (
	VerdictForbidden Verdict = iota
	VerdictDangerous
	VerdictSafe
)

// MatchType 匹配方式
type MatchType int

const (
	MatchPrefix   MatchType = iota
	MatchContains
)

// Rule 单条匹配规则
type Rule struct {
	Pattern   string
	MatchType MatchType
}

// Match 判断命令是否匹配此规则
func (r *Rule) Match(command string) bool {
	cmd := strings.TrimSpace(command)
	switch r.MatchType {
	case MatchContains:
		return strings.Contains(cmd, r.Pattern)
	default: // MatchPrefix
		return strings.HasPrefix(cmd, r.Pattern)
	}
}

// Engine 规则引擎
type Engine struct {
	forbidden []Rule
	dangerous []Rule
	whitelist []string
}

// NewEngine 创建规则引擎，从配置加载规则
func NewEngine(cfg *config.Config) *Engine {
	e := &Engine{
		forbidden: []Rule{},
		dangerous: []Rule{},
		whitelist: cfg.Whitelist,
	}

	// 从配置加载 forbidden 规则
	for _, p := range cfg.ForbiddenPatterns {
		e.forbidden = append(e.forbidden, Rule{Pattern: p, MatchType: MatchPrefix})
	}
	// 从配置加载 dangerous 规则
	for _, p := range cfg.DangerousPatterns {
		e.dangerous = append(e.dangerous, Rule{Pattern: p, MatchType: MatchPrefix})
	}

	return e
}

// Classify 对命令进行分类
func (e *Engine) Classify(command string, mode string, aiCategory executor.Category) Verdict {
	// 1. forbidden
	for i := range e.forbidden {
		if e.forbidden[i].Match(command) {
			return VerdictForbidden
		}
	}

	// 2. dangerous
	for i := range e.dangerous {
		if e.dangerous[i].Match(command) {
			return VerdictDangerous
		}
	}

	// 3. permissive
	if mode == "permissive" {
		return VerdictSafe
	}

	// 4. whitelist (baseName)
	baseName := executor.ExtractBaseName(command)
	if isInList(baseName, e.whitelist) {
		return VerdictSafe
	}

	// 5. rules 模式: 非白名单都需要确认
	if mode == "rules" {
		return VerdictDangerous
	}

	// 6. ai 模式: AI 分类
	if aiCategory == executor.CatRO || aiCategory == executor.CatSudoRO {
		return VerdictSafe
	}
	return VerdictDangerous
}

// ForbiddenReason 返回禁止原因（第一个命中的 forbidden 规则）
func (e *Engine) ForbiddenReason(command string) string {
	for i := range e.forbidden {
		if e.forbidden[i].Match(command) {
			return fmt.Sprintf("禁止执行: 命令匹配危险规则 '%s'", e.forbidden[i].Pattern)
		}
	}
	return ""
}

// isInList 精确匹配
func isInList(s string, list []string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
