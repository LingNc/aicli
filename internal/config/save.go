package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Save 保存配置到文件
func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// SaveDefault 将默认配置写入文件（供 setup 使用）
func SaveDefault() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	p, err := Path()
	if err != nil {
		return err
	}
	return os.WriteFile(p, defaultYAML, 0600)
}
