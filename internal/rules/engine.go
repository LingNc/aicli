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
	readonly  []string
	whitelist []string
}

// hardcodedForbidden 返回硬编码的禁止规则
func hardcodedForbidden() []Rule {
	return []Rule{
		{Pattern: "rm -rf /", MatchType: MatchPrefix},
		{Pattern: "dd if=/dev/zero of=/dev/", MatchType: MatchContains},
		{Pattern: "dd if=/dev/urandom of=/dev/", MatchType: MatchContains},
		{Pattern: ":(){ :|:& };:", MatchType: MatchContains},
		{Pattern: "mkfs", MatchType: MatchPrefix},
		{Pattern: "fdisk", MatchType: MatchPrefix},
		{Pattern: "chmod -R 777 /", MatchType: MatchPrefix},
		{Pattern: "chown -R /", MatchType: MatchPrefix},
	}
}

// hardcodedDangerous 返回硬编码的需确认规则
func hardcodedDangerous() []Rule {
	return []Rule{
		{Pattern: "rm -rf", MatchType: MatchPrefix},
		{Pattern: "dd if=", MatchType: MatchPrefix},
		{Pattern: "mkfs", MatchType: MatchPrefix},
		{Pattern: "fdisk", MatchType: MatchPrefix},
		{Pattern: "chmod -R", MatchType: MatchPrefix},
		{Pattern: "chown -R", MatchType: MatchPrefix},
		{Pattern: ">", MatchType: MatchContains},
		{Pattern: "| xargs rm", MatchType: MatchContains},
	}
}

// NewEngine 创建规则引擎，合并硬编码规则和用户配置
func NewEngine(cfg *config.Config) *Engine {
	e := &Engine{
		forbidden: hardcodedForbidden(),
		dangerous: hardcodedDangerous(),
		readonly:  cfg.ReadonlyCommands,
		whitelist: cfg.Whitelist,
	}

	// 追加用户配置的 forbidden 规则（默认 prefix）
	for _, p := range cfg.ForbiddenPatterns {
		e.forbidden = append(e.forbidden, Rule{Pattern: p, MatchType: MatchPrefix})
	}
	// 追加用户配置的 dangerous 规则（默认 prefix）
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

	// 5. rules 模式: readonly（基础命令名精确匹配）
	if mode == "rules" {
		if isInList(baseName, e.readonly) {
			return VerdictSafe
		}
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
