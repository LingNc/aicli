package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lingnc/aicli/internal/log"
)

const githubAPI = "https://api.github.com/repos/lingnc/aicli/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Release 从 GitHub Release 下载最新版本并替换当前二进制
func Release(currentVersion string, netTimeout int) error {
	log.Print("正在检查更新...")

	// 1. 获取最新 release 信息
	if netTimeout <= 0 {
		netTimeout = 15
	}
	client := &http.Client{Timeout: time.Duration(netTimeout) * time.Second}
	resp, err := client.Get(githubAPI)
	if err != nil {
		return fmt.Errorf("无法连接到 GitHub，请检查网络: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("GitHub API 请求过于频繁，请稍后重试")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API 返回异常状态: %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}

	// 2. 版本比较
	if !needsUpdate(currentVersion, release.TagName) {
		log.Print("已是最新版本 (当前: %s, 最新 release: %s)", currentVersion, release.TagName)
		return nil
	}
	log.Print("发现新版本: %s (当前: %s)", release.TagName, currentVersion)

	// 3. 查找匹配平台的二进制
	assetName := fmt.Sprintf("aicli-%s-%s", runtime.GOOS, runtime.GOARCH)
	downloadURL := ""
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	// 回退：查找不带平台后缀的 aicli
	if downloadURL == "" {
		for _, a := range release.Assets {
			if a.Name == "aicli" {
				downloadURL = a.BrowserDownloadURL
				assetName = "aicli"
				break
			}
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("未找到适用于 %s/%s 的二进制文件", runtime.GOOS, runtime.GOARCH)
	}

	// 4. 下载到临时文件
	log.Print("正在下载 %s ...", assetName)
	tmpFile, err := os.CreateTemp("", "aicli-update-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // 清理（如果替换成功，文件已不在）

	if err := downloadFile(client, downloadURL, tmpFile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("下载失败: %w", err)
	}
	tmpFile.Close()
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("设置权限失败: %w", err)
	}

	// 5. 替换当前二进制
	target, err := findSelf()
	if err != nil {
		return fmt.Errorf("获取当前程序路径失败: %w", err)
	}

	if err := replaceBinary(tmpPath, target); err != nil {
		return err
	}

	log.Print("-> 已更新到 %s", release.TagName)
	return nil
}

// Dev 从 main 分支拉取最新代码并本地编译
func Dev(currentVersion string) error {
	// 1. 检查依赖工具
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("未找到 go，请先安装 Go 工具链")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("未找到 git，请先安装 git")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取 home 目录失败: %w", err)
	}
	repoDir := filepath.Join(home, ".aicli", "repo")
	repoURL := "https://github.com/lingnc/aicli.git"

	// 2. 克隆或更新仓库
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		log.Print("正在克隆仓库...")
		cmd := exec.Command("git", "clone", repoURL, repoDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("克隆仓库失败: %w", err)
		}
	} else {
		log.Print("正在拉取最新代码...")
		fetch := exec.Command("git", "-C", repoDir, "fetch", "origin", "main")
		fetch.Run()
		reset := exec.Command("git", "-C", repoDir, "reset", "--hard", "origin/main")
		reset.Stdout = os.Stdout
		reset.Stderr = os.Stderr
		if err := reset.Run(); err != nil {
			return fmt.Errorf("更新仓库失败: %w", err)
		}
	}

	// 3. 编译
	log.Print("正在编译...")
	binPath := filepath.Join(repoDir, "aicli")
	build := exec.Command("go", "build", "-ldflags=-s -w", "-o", binPath, "./cmd/aicli/")
	build.Dir = repoDir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("编译失败: %w", err)
	}

	// 4. 替换当前二进制
	target, err := findSelf()
	if err != nil {
		return fmt.Errorf("获取当前程序路径失败: %w", err)
	}
	if err := replaceBinary(binPath, target); err != nil {
		return err
	}

	log.Print("-> 已更新到最新开发版本")
	return nil
}

// --- 辅助函数 ---

func findSelf() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// needsUpdate 判断是否需要更新。-dev 后缀表示开发版本，比较时只取基础版本号。
func needsUpdate(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// 去掉 -dev 后缀，只比较基础版本号
	current = strings.Split(current, "-")[0]

	return compareSemver(latest, current) > 0
}

// compareSemver 比较语义版本号，返回 1/0/-1
func compareSemver(a, b string) int {
	aParts := strings.Split(strings.Split(a, "-")[0], ".")
	bParts := strings.Split(strings.Split(b, "-")[0], ".")

	for i := range 3 {
		aNum, bNum := 0, 0
		if i < len(aParts) {
			fmt.Sscanf(aParts[i], "%d", &aNum)
		}
		if i < len(bParts) {
			fmt.Sscanf(bParts[i], "%d", &bNum)
		}
		if aNum > bNum {
			return 1
		}
		if aNum < bNum {
			return -1
		}
	}
	return 0
}

func downloadFile(client *http.Client, url string, w io.Writer) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func replaceBinary(src, dst string) error {
	if src == dst {
		return nil
	}

	if needsSudo(dst) {
		cmd := exec.Command("sudo", "install", "-m", "0755", src, dst)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("系统安装失败: %w", err)
		}
		return nil
	}

	tmp := dst + ".tmp"
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0755); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("替换失败: %w", err)
	}
	return nil
}

func needsSudo(path string) bool {
	return strings.HasPrefix(path, "/usr/")
}
