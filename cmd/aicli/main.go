package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lingnc/aicli/internal/config"
	"github.com/lingnc/aicli/internal/executor"
	"github.com/lingnc/aicli/internal/llm"
	"github.com/lingnc/aicli/internal/log"
	"github.com/lingnc/aicli/internal/rules"
	"github.com/lingnc/aicli/internal/shell"
)

var version = "v0.1.0"

// parseArgs 解析命令行参数
// 返回: debug标志, 显示版本, 子命令, 子命令动作, 用户输入
func parseArgs(args []string) (debug bool, showVersion bool, subcommand string, subAction string, userInput string) {
	for i, arg := range args {
		if arg == "-d" || arg == "--debug" {
			debug = true
		} else if arg == "-v" || arg == "--version" {
			showVersion = true
		} else if arg == "-h" || arg == "--help" {
			showHelp()
			os.Exit(0)
		} else if arg == "setup" {
			subcommand = "setup"
		} else if arg == "log" {
			subcommand = "log"
		} else if arg == "shell" && i+1 < len(args) {
			subcommand = "shell"
			subAction = args[i+1]
			break
		} else {
			if userInput != "" {
				userInput += " "
			}
			userInput += arg
		}
	}
	return
}

// showHelp 显示帮助信息
func showHelp() {
	w := os.Stderr
	printOption := func(flagText string, desc string) {
		fmt.Fprintf(w, "  %-16s %s\n", flagText, desc)
	}

	fmt.Fprintf(w, "ai %s\n", version)
	fmt.Fprintf(w, "用法: ai [-d] [-v] <查询>\n\n")
	fmt.Fprintf(w, "子命令:\n")
	printOption("setup", "配置 API 密钥和模型")
	printOption("log", "打开最新的调试日志")
	printOption("shell install", "安装 shell 集成")
	printOption("shell uninstall", "卸载 shell 集成")
	fmt.Fprintf(w, "\n选项:\n")
	printOption("-v, --version", "显示版本号")
	printOption("-d, --debug", "启用调试日志")
	printOption("-h, --help", "显示帮助信息")
	fmt.Fprintf(w, "\n示例:\n")
	fmt.Fprintf(w, "  ai 查看内存\n")
	fmt.Fprintf(w, "  ai 列出占用端口8085的程序\n")
	fmt.Fprintf(w, "  ai 删除所有.tmp文件\n")
}

func main() {
	code := run()
	path := log.Close()
	if path != "" {
		log.Info("-> 调试日志已保存: %s", path)
	}
	os.Exit(code)
}

func run() int {
	// 1. 解析参数
	debug, showVersion, subcommand, subAction, userInput := parseArgs(os.Args[1:])

	// 1.5 处理版本号显示
	if showVersion {
		fmt.Printf("ai %s\n", version)
		fmt.Println("Author: LingNc")
		fmt.Println("Repository: https://github.com/LingNc/aicli")
		return 0
	}

	// 2. 处理 setup 子命令
	if subcommand == "setup" {
		if err := config.Setup(nil); err != nil {
			log.Error("-> %v", err)
			return 4
		}
		return 0
	}

	// 2.5 处理 shell 子命令
	if subcommand == "shell" {
		switch subAction {
		case "install":
			if err := shell.Install(); err != nil {
				log.Error("安装失败: %v", err)
				return 1
			}
		case "uninstall":
			if err := shell.Uninstall(); err != nil {
				log.Error("卸载失败: %v", err)
				return 1
			}
		default:
			log.Info("用法: ai shell install | uninstall")
			return 1
		}
		return 0
	}

	// 处理 log 子命令（不需要加载配置）
	if subcommand == "log" {
		logDir := resolveLogDir("")
		latest := findLatestLog(logDir)
		if latest == "" {
			log.Info("没有找到日志文件")
			return 1
		}
		log.Info("-> %s", latest)
		openFile(latest)
		return 0
	}

	// 2.8 空参数优先显示 help
	if userInput == "" {
		showHelp()
		return 0
	}

	// 3. 检查配置文件，不存在则自动 Setup
	if !config.Exists() {
		log.Info("配置文件不存在，正在引导设置...")
		if err := config.Setup(nil); err != nil {
			log.Error("设置失败: %v", err)
			return 4
		}
		log.Info("设置完成，正在继续...")
	}

	// 4. 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Error("加载配置失败: %v", err)
		return 4
	}

	// 5. 初始化日志系统（配置文件或 CLI 标志均可启用）
	if err := log.Init(cfg.Debug || debug, cfg.DebugLogConsole, cfg.DebugLogDir); err != nil {
		log.Error("初始化日志失败: %v", err)
		return 4
	}

	// 6. 验证配置
	if err := config.Validate(cfg); err != nil {
		log.Error("配置验证失败，请运行 ai setup: %v", err)
		return 4
	}

	// 8. 创建 LLM 客户端
	client := llm.New(cfg)

	// 9. 创建命令解析器
	parser := executor.NewParser()

	// 10. 记录开始时间
	startTime := time.Now()

	// 11. 流式聊天，使用 goroutine 接收流
	// 通过 channel 传递命令结果，避免主 goroutine 与流式 goroutine 之间的数据竞争
	type cmdReady struct {
		command  string
		category executor.Category
	}
	cmdCh := make(chan cmdReady, 1)
	streamDone := make(chan error, 1)

	cmdPrefixShown := false
	cmdNewlinePrinted := false
	go func() {
		_, err := client.StreamChat(userInput, func(chunk string) {
			newCmd := parser.Feed(chunk)
			if newCmd != "" {
				newCmd = strings.ReplaceAll(newCmd, "\r", "")
				if newCmd != "" {
					if !cmdPrefixShown {
						log.PrintRaw("$ " + newCmd)
						cmdPrefixShown = true
					} else {
						log.PrintRaw(newCmd)
					}
				}
			}
			// 命令完成后输出换行（仅一次），避免主 goroutine 输出的换行与流交错
			if parser.CommandDone() && !cmdNewlinePrinted {
				log.Print("")
				cmdNewlinePrinted = true
			}
			// 在 callback 内部检查命令是否完成
			// 如果刚完成，通过 channel 通知主 goroutine（避免主 goroutine 直接读取 parser 字段造成竞争）
			// 用 len(cmdCh) == 0 避免后续 chunk 重复发送
			if parser.CommandDone() && len(cmdCh) == 0 {
				cmdCh <- cmdReady{
					command:  parser.CurrentCommand,
					category: parser.CurrentCategory,
				}
			}
		})
		streamDone <- err
	}()

	// 12. 等待命令完成或流结束
	var cmd cmdReady
	var streamErr error
	select {
	case cmd = <-cmdCh:
		// 命令已完整，立即继续
	case streamErr = <-streamDone:
		// 流结束但命令未通过 channel 发送（可能没有 #$ 前缀）
		if streamErr != nil {
			if strings.Contains(streamErr.Error(), "401") || strings.Contains(streamErr.Error(), "unauthorized") {
				log.Info("API key 无效，请运行 ai setup 重新配置")
			} else {
				log.Error("请求失败: %v", streamErr)
			}
			return 3
		}
		// 流正常结束但分类未确定，使用 parser.Finish() 处理残余
		result := parser.Finish()
		cmd = cmdReady{command: result.Command, category: result.Category}
	}

	finalCommand := cmd.command
	finalCategory := cmd.category

	if finalCommand == "" {
		log.Info("AI 未生成命令")
		return 3
	}

	// 15. 调试信息
	elapsed := time.Since(startTime)
	log.Debug("分类: %v", finalCategory)
	log.Debug("耗时: %v", elapsed)
	log.Debug("命令: %s", finalCommand)

	// 16. 规则引擎分类
	engine := rules.NewEngine(cfg)
	verdict := engine.Classify(finalCommand, cfg.Mode, finalCategory)

	// 17. 根据分类结果处理
	switch verdict {
	case rules.VerdictForbidden:
		reason := engine.ForbiddenReason(finalCommand)
		if reason == "" {
			reason = "命令被安全规则禁止执行"
		}
		log.Info("-> %s", reason)
		return 2

	case rules.VerdictDangerous:
		approved, addWhite, err := executor.Confirm(finalCategory, cfg)
		if err != nil {
			log.Error("确认过程出错: %v", err)
			return 1
		}
		if !approved {
			log.Info("-> 已取消")
			return 2
		}
		if addWhite {
			baseName := executor.ExtractBaseName(finalCommand)
			cfg.AddToWhitelist(baseName)
			log.Info("-> 已将 %s 加入白名单", baseName)
		}

	case rules.VerdictSafe:
		// 直接执行，无操作
	}

	// 18. 执行命令
	_, exitCode, err := executor.Execute(finalCommand)
	if err != nil {
		log.Error("命令执行失败: %v", err)
		return 1
	}

	// 19. 写入临时文件
	tmpfile := "/tmp/ai-cmd-" + strconv.Itoa(os.Getppid()) + ".txt"
	os.WriteFile(tmpfile, []byte(finalCommand), 0644)

	// 20. 等待流结束（如果命令先完整，流仍在后台接收 explanation）
	// 若上面 select 已读取过 streamDone，则 streamErr 非 nil，跳过等待
	if streamErr == nil {
		<-streamDone
	}

	// 写入日志摘要（取 parser 的 explanation 作为文件名）
	if parser.Result != nil && parser.Result.Explanation != "" {
		log.Rename(parser.Result.Explanation)
	}

	// 21. 根据退出码退出
	if exitCode != 0 {
		return exitCode
	}

	return 0
}

// resolveLogDir 获取日志目录（默认 ~/.aicli/log/）
func resolveLogDir(logDir string) string {
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

// findLatestLog 查找目录中最新的 .log 文件
func findLatestLog(dir string) string {
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

// openFile 用编辑器打开文件
func openFile(path string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
