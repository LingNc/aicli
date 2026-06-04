package config

import (
	"fmt"
	"os"
	"reflect"

	"github.com/lingnc/aicli/internal/log"
	"gopkg.in/yaml.v3"
)

// Load 加载配置文件，不存在则返回默认配置。
// 如果检测到配置版本过旧，仅打印警告，由用户自行运行 'aicli setup' 更新。
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return loadDefault()
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 版本过旧提示，不自动迁移、不打开编辑器
	if cfg.ConfigVersion >= 0 && cfg.ConfigVersion < defaultConfigVersion() {
		log.Warn("配置文件版本过旧(v%d)，请在 'setup' 更新", cfg.ConfigVersion)
	}

	// 填充默认值（处理 struct 级零值，如 RequestBody nil）
	fillDefaults(cfg)
	return cfg, nil
}

// defaultConfigVersion 返回嵌入的 default.yaml 中的 config_version
func defaultConfigVersion() int {
	var cfg Config
	if err := yaml.Unmarshal(defaultYAML, &cfg); err != nil {
		return 0
	}
	return cfg.ConfigVersion
}

// loadDefault 从嵌入的默认配置加载
func loadDefault() (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultYAML, cfg); err != nil {
		return nil, fmt.Errorf("解析默认配置失败: %w", err)
	}
	return cfg, nil
}

// fillDefaults 用默认值填充空字段（基于反射，自动从 default.yaml 补全零值）
// 跳过 RequestBody 字段，请求体由用户自行管理。
func fillDefaults(cfg *Config) {
	var defaults Config
	if err := yaml.Unmarshal(defaultYAML, &defaults); err != nil {
		return // 解析失败不填充，使用零值
	}

	cfgV := reflect.ValueOf(cfg).Elem()
	defV := reflect.ValueOf(&defaults).Elem()
	t := cfgV.Type()

	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Name == "RequestBody" {
			continue // 请求体不自动填充
		}
		if t.Field(i).Name == "ThinkingBody" {
			continue // 思考模式请求体不自动填充
		}
		if t.Field(i).Name == "ThinkingLines" || t.Field(i).Name == "ThinkingLineLen" {
			continue // 构造器中有默认值回退，避免 0 被覆盖
		}
		if t.Field(i).Name == "APITimeout" || t.Field(i).Name == "StreamTimeout" || t.Field(i).Name == "ExecTimeout" {
			continue // 调用处有默认值回退，避免 0 被覆盖
		}
		field := cfgV.Field(i)
		if field.IsZero() {
			field.Set(defV.Field(i))
		}
	}
}
