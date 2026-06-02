package config

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/lingnc/aicli/internal/log"
	"github.com/lingnc/aicli/internal/utils"
	"golang.org/x/term"
)

// promptConfigUpdate 配置版本不匹配时提示用户是否更新
func promptConfigUpdate(configPath string, userVer, defVer int) bool {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return false
	}

	fmt.Fprintf(os.Stderr, "-> 配置文件版本过旧 (v%d -> v%d)，是否更新？[y/N] ", userVer, defVer)

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false
	}
	defer term.Restore(fd, oldState)
	defer fmt.Fprintf(os.Stderr, "\r\033[K")

	buf := make([]byte, 1)
	if _, err := os.Stdin.Read(buf); err != nil {
		return false
	}

	return buf[0] == 'y' || buf[0] == 'Y'
}

// promptRetry 处理验证失败后的重试选择（单键输入，无需回车）
// 返回: "retry"=重新编辑, "cancel"=放弃, "force"=强制保存
func promptRetry(validationErr error, configPath string, backup []byte) string {
	fd := int(os.Stdin.Fd())
	oldState, rawErr := term.MakeRaw(fd) // 改用 rawErr，不再遮蔽 validationErr
	if rawErr != nil {
		// 非终端环境 fallback（需要回车确认）
		fmt.Fprintf(os.Stderr, "错误: %s\n", utils.FormatError(validationErr))
		fmt.Fprintf(os.Stderr, "[e]重新编辑 / [x]放弃 / [f]强制保存 ")
		var choice string
		fmt.Scanln(&choice)
		if len(choice) > 0 {
			switch choice[0] {
			case 'x', 'X':
				if len(backup) > 0 {
					os.WriteFile(configPath, backup, 0600)
				}
				return "cancel"
			case 'f', 'F':
				return "force"
			}
		}
		return "retry"
	}
	defer term.Restore(fd, oldState)
	// 退出时回到行首清空该行,然后回到上一行清空该行
	defer fmt.Fprintf(os.Stderr, "\r\033[K\033[1A\r\033[K")

	// raw 模式：\r\033[K 先回到行首并清除行尾，再打印错误，确保旧内容被完全覆盖
	fmt.Fprintf(os.Stderr, "\r\033[K[e]重新编辑 / [x]放弃 / [f]强制保存 \r\n")
	fmt.Fprintf(os.Stderr, "-> 错误: %s", utils.FormatError(validationErr))

	for {
		buf := make([]byte, 1)
		os.Stdin.Read(buf)

		switch buf[0] {
		case 'x', 'X':
			if len(backup) > 0 {
				os.WriteFile(configPath, backup, 0600)
			}
			return "cancel"
		case 'f', 'F':
			return "force"
		case 'e', 'E', '\r', '\n':
			return "retry"
		default:
			log.Bell() // 只响铃，不输出任何字符
		}
	}
}

// Setup 交互式配置引导
func Setup(originalArgs []string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	configPath, err := Path()
	if err != nil {
		return err
	}

	if !Exists() {
		if err := SaveDefault(); err != nil {
			return err
		}
	}

	// 备份旧配置内容
	var backup []byte
	data, err := os.ReadFile(configPath)
	if err == nil {
		backup = data
		data, _ = checkAndMigrateConfig(configPath, data)
		// 同步更新备份，以便编辑器加载的就是新内容
		backup = data
	}

EDITOR:
	for {
		cmd := exec.Command(utils.GetEditor(), configPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("打开编辑器失败: %w", err)
		}

		cfg, err := Load()
		if err != nil {
			result := promptRetry(err, configPath, backup)
			switch result {
			case "retry":
				goto EDITOR
			case "cancel":
				return fmt.Errorf("放弃配置")
			case "force":
				return fmt.Errorf("已保存但配置无效: %v", err)
			}
		}

		if err := Validate(cfg); err != nil {
			result := promptRetry(err, configPath, backup)
			switch result {
			case "retry":
				goto EDITOR
			case "cancel":
				return fmt.Errorf("放弃配置")
			case "force":
				return fmt.Errorf("已保存但配置无效: %v", err)
			}
		}

		// 对比备份判断是否实际修改了配置
		if current, err := os.ReadFile(configPath); err == nil && string(current) == string(backup) {
			log.Print("-> 配置未修改")
		} else {
			log.Print("-> 配置已保存")
		}
		break
	}

	if len(originalArgs) > 0 {
		log.Print("继续执行原始命令...")
	}

	return nil
}
