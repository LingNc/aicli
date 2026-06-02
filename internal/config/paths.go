package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir 返回配置目录路径 (~/.aicli)
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取 home 目录失败: %w", err)
	}
	return filepath.Join(home, ".aicli"), nil
}

// Path 返回配置文件路径 (~/.aicli/config.yaml)
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Exists 检查配置文件是否存在
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}
