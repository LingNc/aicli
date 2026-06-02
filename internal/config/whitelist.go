package config

import (
	"slices"

	"github.com/lingnc/aicli/internal/log"
)

// AddToWhitelist 添加命令到白名单（去重后追加）
func (cfg *Config) AddToWhitelist(baseName string) {
	if slices.Contains(cfg.Whitelist, baseName) {
		return
	}
	cfg.Whitelist = append(cfg.Whitelist, baseName)
	if err := Save(cfg); err != nil {
		log.Warn("保存白名单失败: %v", err)
	}
}

// IsWhitelisted 检查命令是否在白名单中
func (cfg *Config) IsWhitelisted(baseName string) bool {
	for _, name := range cfg.Whitelist {
		if name == baseName {
			return true
		}
	}
	return false
}
