package config

import (
	"fmt"
	"net/url"
	"strings"
)

// Validate 全面验证配置
func Validate(cfg *Config) error {
	var errs []string

	// 1. API Key 非空（必填）
	if cfg.APIKey == "" {
		errs = append(errs, "api_key 不能为空")
	} else if !strings.HasPrefix(cfg.APIKey, "sk-") && !strings.HasPrefix(cfg.APIKey, "org-") {
		// 非强制，仅警告：常见 OpenAI/兼容 key 格式提醒
		errs = append(errs, "警告: api_key 格式可能不正确（应以 sk- 或 org- 开头），请检查")
	}

	// 2. BaseURL 非空且为有效 URL
	if cfg.BaseURL == "" {
		errs = append(errs, "base_url 不能为空")
	} else if u, err := url.Parse(cfg.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		errs = append(errs, "base_url 不是有效的 HTTP/HTTPS URL")
	}

	// 3. Model 非空
	if cfg.Model == "" {
		errs = append(errs, "model 不能为空")
	} // 可选：检查是否在已知模型列表，但模型名不断更新，仅非空即可

	// 4. Mode 合法
	switch cfg.Mode {
	case "ai", "rules", "permissive":
		// ok
	default:
		errs = append(errs, "mode 必须是 ai/rules/permissive，当前: "+cfg.Mode)
	}

	// 5. 检查白名单有无重复（自动去重即可，但也可提示）
	seen := make(map[string]bool)
	for _, w := range cfg.Whitelist {
		if seen[w] {
			errs = append(errs, "白名单中存在重复命令: "+w)
		}
		seen[w] = true
	}

	// 6. Debug 字段是 bool，无需检查
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}
