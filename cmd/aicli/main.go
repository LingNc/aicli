package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lingnc/aicli/internal/config"
	"github.com/lingnc/aicli/internal/display"
	"github.com/lingnc/aicli/internal/executor"
	"github.com/lingnc/aicli/internal/llm"
	"github.com/lingnc/aicli/internal/log"
	"github.com/lingnc/aicli/internal/rules"
	"github.com/lingnc/aicli/internal/shell"
	"github.com/lingnc/aicli/internal/utils"
	"golang.org/x/term"
)

var version = "v0.1.1"

// parseArgs 解析命令行参数
// 返回: debug标志, showDebug标志, 显示版本, 思考模式, 子命令, 子命令动作, 用户输入
func parseArgs(args []string) (debug bool, showDebug bool, showVersion bool, think bool, subcommand string, subAction string, userInput string) {
	for i, arg := range args {
		if arg == "-d" || arg == "--debug" {
			debug = true
		} else if arg == "-sd" || arg == "--show-debug" {
			showDebug = true
		} else if arg == "-v" || arg == "--version" {
			showVersion = true
		} else if arg == "-h" || arg == "--help" {
			showHelp()
			os.Exit(0)
		} else if arg == "-t" || arg == "--think" {
			think = true
		} else if arg == "setup" {
			subcommand = "setup"
		} else if arg == "log" {
			subcommand = "log"
			// 收集后续关键词
			if i+1 < len(args) {
				userInput = strings.Join(args[i+1:], " ")
			}
			break
		} else if arg == "shell" {
			subcommand = "shell"
			if i+1 < len(args) {
				subAction = args[i+1]
				i++ // 跳过 subAction
			}
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
	printOption("-sd, --show-debug", "调试信息同时输出到控制台")
	printOption("-t, --think", "启用思考模式")
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
		log.Info("-> 调试日志已保存: %s (使用 'ai log' 查看)", path)
	}
	os.Exit(code)
}

func run() int {
	// 1. 解析参数
	debug, showDebug, showVersion, think, subcommand, subAction, userInput := parseArgs(os.Args[1:])

	// 2. 初始化日志系统（不依赖配置，可尽早启用）
	cwd, _ := os.Getwd()
	fullCmd := cwd + "/ai " + strings.Join(os.Args[1:], " ")
	cmdName := "ask"
	if subcommand != "" {
		cmdName = subcommand
	}
	if err := log.Init(debug, showDebug, cmdName, fullCmd); err != nil {
		log.Error("初始化日志失败: %v", err)
		return 4
	}

	// 3. 处理版本号显示
	if showVersion {
		fmt.Printf("ai %s\n", version)
		fmt.Println("Author: LingNc")
		fmt.Println("Repository: https://github.com/LingNc/aicli")
		return 0
	}

	// 3.1 空参数检查（仅在无子命令时显示 help）
	if userInput == "" && subcommand == "" {
		showHelp()
		return 0
	}

	// 7.1 先处理 setup 子命令
	if subcommand == "setup" {
		if err := config.Setup(nil); err != nil {
			log.Error("-> %v", err)
			return 4
		}
		return 0
	}

	// 4. 检查配置文件，不存在则自动 Setup
	if !config.Exists() {
		log.Info("配置文件不存在，正在引导设置...")
		if err := config.Setup(nil); err != nil {
			log.Error("-> %v", err)
			return 4
		}
	}

	// 5. 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Error("加载配置失败: %v", err)
		return 4
	}

	// 6. 验证配置
	if err := config.Validate(cfg); err != nil {
		log.Error("配置验证失败: %v", err)
		enterSetup := false
		fmt.Fprintf(os.Stderr, "-> 是否进入 setup 修改配置？[Y/n] ")
		fd := int(os.Stdin.Fd())
		if term.IsTerminal(fd) {
			oldState, rawErr := term.MakeRaw(fd)
			if rawErr == nil {
				buf := make([]byte, 1)
				os.Stdin.Read(buf)
				term.Restore(fd, oldState)
				fmt.Fprintf(os.Stderr, "\r\033[1A\033[J")
				if buf[0] != 'n' && buf[0] != 'N' {
					enterSetup = true
				}
			}
		} else {
			enterSetup = true
		}
		if enterSetup {
			if err := config.Setup(nil); err != nil {
				log.Error("-> %v", err)
				return 4
			}
			// 清理 Setup 的输出
			fmt.Fprintf(os.Stderr, "\r\033[1A\033[J")
			// 重新加载配置
			cfg, err = config.Load()
			if err != nil {
				log.Error("[Load] 加载配置失败: %v", err)
				return 4
			}
		} else {
			// 用户取消
			log.Info("-> 已取消")
			return 4
		}
		// 有子命令时跳过 setup，继续执行
	}

	// 7. 处理子命令（setup/shell/log）
	// 7.2 处理 shell 子命令
	if subcommand == "shell" {
		switch subAction {
		case "install":
			if err := shell.Install(); err != nil {
				log.Error("[Shell] 安装失败: %v", err)
				return 1
			}
		case "uninstall":
			if err := shell.Uninstall(); err != nil {
				log.Error("[Shell] 卸载失败: %v", err)
				return 1
			}
		default:
			log.Info("用法: ai shell [install | uninstall]")
			return 1
		}
		return 0
	}

	// 7.3 处理 log 子命令
	if subcommand == "log" {
		logDir := utils.ResolveDir("")
		if userInput != "" {
			// 有关键词：模糊匹配
			latest := log.FindMatching(logDir, userInput)
			if latest == "" {
				log.Info("没有匹配 '%s' 的日志文件", userInput)
				return 1
			}
			log.Info("-> %s", latest)
			utils.OpenReadOnly(latest)
		} else {
			// 无关键词：打开最新日志
			latest := log.FindLatest(logDir)
			if latest == "" {
				log.Info("没有找到日志文件")
				return 1
			}
			log.Info("-> %s", latest)
			utils.OpenReadOnly(latest)
		}
		return 0
	}

	// 8. 创建 LLM 客户端
	client := llm.New(cfg)

	// 8.1 创建命令解析器
	parser := executor.NewParser()

	// 8.2 记录开始时间
	startTime := time.Now()

	// 9. 流式聊天，使用 goroutine 接收流
	// 通过 channel 传递命令结果，避免主 goroutine 与流式 goroutine 之间的数据竞争
	type cmdReady struct {
		command  string
		category executor.Category
	}
	cmdCh := make(chan cmdReady, 1)
	streamDone := make(chan error, 1)

	cmdPrefixShown := false
	cmdNewlinePrinted := false

	// 思考模式显示
	var thinkDisplay *display.ThinkingDisplay
	if think {
		thinkDisplay = display.NewThinkingDisplay(cfg.ThinkingLines, cfg.ThinkingLineLen)
		thinkDisplay.Start()
	}

	go func() {
		_, err := client.StreamChat(userInput, think,
			func(chunk string) {
				// 首个 content chunk：清除思考显示
				if thinkDisplay != nil && thinkDisplay.IsActive() {
					thinkDisplay.Stop()
				}
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
			},
			func(reasoning string) {
				if thinkDisplay != nil {
					thinkDisplay.FeedReasoning(reasoning)
				}
			},
		)
		// 流结束后确保思考显示被清理
		if thinkDisplay != nil && thinkDisplay.IsActive() {
			thinkDisplay.Stop()
		}
		streamDone <- err
	}()

	// 9.1 等待命令完成或流结束
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

	// 10. 调试信息
	elapsed := time.Since(startTime)
	log.Debug("分类: %v", finalCategory)
	log.Debug("耗时: %v", elapsed)
	log.Debug("命令: %s", finalCommand)

	// 11. 规则引擎分类
	engine := rules.NewEngine(cfg)
	verdict := engine.Classify(finalCommand, cfg.Mode, finalCategory)

	// 11.1 根据分类结果处理
	log.Debug("规则分类: verdict=%v, mode=%s, aiCategory=%v", verdict, cfg.Mode, finalCategory)
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

	// 12. 执行命令
	_, exitCode, err := executor.Execute(finalCommand, cfg.ExecTimeout)
	if err != nil {
		log.Error("命令执行失败: %v", err)
		return 1
	}

	// 12.1 写入临时文件供 shell 集成读取；失败不影响主流程
	tmpfile := "/tmp/ai-cmd-" + strconv.Itoa(os.Getppid()) + ".txt"
	os.WriteFile(tmpfile, []byte(finalCommand), 0644)
	log.Debug("写入临时文件: %s", tmpfile)

	// 13. 等待流结束（如果命令先完整，流仍在后台接收 explanation）
	// 若上面 select 已读取过 streamDone，则 streamErr 非 nil，跳过等待
	if streamErr == nil {
		<-streamDone
	}

	// 14. 根据退出码退出
	if exitCode != 0 {
		return exitCode
	}

	return 0
}
