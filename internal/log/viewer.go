package log

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResolveDir 获取日志目录（默认 ~/.aicli/log/）
func ResolveDir(logDir string) string {
	if logDir == "" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".aicli", "log")
	}
	if strings.HasPrefix(logDir, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(logDir, "~"))
	}
	return logDir
}

// FindLatest 查找目录中最新的 .log 文件
func FindLatest(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest string
	var latestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestMod) {
			latestMod = info.ModTime()
			latest = filepath.Join(dir, e.Name())
		}
	}
	return latest
}

// FindMatching 查找目录中文件名包含 keyword 的最新 .log 文件
func FindMatching(dir, keyword string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var match string
	var matchMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		if !strings.Contains(e.Name(), keyword) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(matchMod) {
			matchMod = info.ModTime()
			match = filepath.Join(dir, e.Name())
		}
	}
	return match
}
