package utils

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetEditor 返回用户偏好的编辑器（$EDITOR > $VISUAL > vi）
func GetEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	return "vi"
}

// FormatError 将多行错误信息压缩为单行
func FormatError(err error) string {
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.Join(strings.Fields(s), " ")
}

// ResolveDir 解析日志目录路径（默认 ~/.aicli/logs/）
func ResolveDir(logDir string) string {
	if logDir == "" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".aicli", "logs")
	}
	if strings.HasPrefix(logDir, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(logDir, "~"))
	}
	return logDir
}

// OpenReadOnly 用 less 以只读模式打开文件（-R 保留 ANSI 颜色）
func OpenReadOnly(path string) {
	cmd := exec.Command("less", "-R", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
