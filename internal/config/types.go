package config

import (
	_ "embed"
)

//go:embed default.yaml
var defaultYAML []byte

// Config 是应用配置结构体
type Config struct {
	ConfigVersion     int            `yaml:"config_version"`
	APIKey            string         `yaml:"api_key"`
	BaseURL           string         `yaml:"base_url"`
	Model             string         `yaml:"model"`
	Mode              string         `yaml:"mode"`
	Debug             bool           `yaml:"debug"`
	DebugLogConsole   bool           `yaml:"debug_log_console"`
	DebugLogDir       string         `yaml:"debug_log_dir"`
	Whitelist         []string       `yaml:"whitelist"`
	ForbiddenPatterns []string       `yaml:"forbidden_patterns"` // 用户扩展的禁止命令
	DangerousPatterns []string       `yaml:"dangerous_patterns"` // 用户扩展的需确认命令
	RequestBody       map[string]any `yaml:"request_body"`       // 请求体参数，直接合并到 API 请求 JSON
}
