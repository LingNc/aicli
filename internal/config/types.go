package config

import (
	_ "embed"
)

//go:embed default.yaml
var defaultYAML []byte

// Config 是应用配置结构体
type Config struct {
	ConfigVersion     string         `yaml:"config_version"`
	APIKey            string         `yaml:"api_key"`
	BaseURL           string         `yaml:"base_url"`
	Model             string         `yaml:"model"`
	Mode              string         `yaml:"mode"`
	Whitelist         []string       `yaml:"whitelist"`
	ForbiddenPatterns []string       `yaml:"forbidden_patterns"` // 用户扩展的禁止命令
	DangerousPatterns []string       `yaml:"dangerous_patterns"` // 用户扩展的需确认命令
	RequestBody       map[string]any `yaml:"request_body"`       // 请求体参数，直接合并到 API 请求 JSON
	ThinkingBody      map[string]any `yaml:"thinking_body"`      // 思考模式请求体（--think/-t 时合并到请求体）
	ThinkingLines     int            `yaml:"thinking_lines"`    // 思考显示最大行数
	ThinkingLineLen   *int           `yaml:"thinking_line_len"` // 单行最大字符数，0=终端宽度，nil=用默认值
	APITimeout        *int           `yaml:"api_timeout"`
	StreamTimeout     *int           `yaml:"stream_timeout"`
	ExecTimeout       *int           `yaml:"exec_timeout"`
	NetTimeout        *int           `yaml:"net_timeout"`         // 网络请求超时（秒）
}
