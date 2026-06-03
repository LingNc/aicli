package log

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

// FindMatching 查找目录中文件名或首行包含 keyword 的最新 .log 文件
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
		found := strings.Contains(e.Name(), keyword)
		if !found {
			found = firstLineContains(filepath.Join(dir, e.Name()), keyword)
		}
		if !found {
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

// firstLineContains 读取文件第一行，判断是否包含 keyword。
func firstLineContains(path, keyword string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false
	}
	line := buf[:n]
	if idx := strings.IndexByte(string(line), '\n'); idx >= 0 {
		line = line[:idx]
	}
	return strings.Contains(string(line), keyword)
}
