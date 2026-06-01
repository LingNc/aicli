package main

import (
	"fmt"
	"os"
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

// parseArgs 解析命令行参数
// 返回: debug标志, 子命令, 子命令动作, 用户输入
func parseArgs(args []string) (debug bool, subcommand string, subAction string, userInput string) {
	for i, arg := range args {
		if arg == "-d" || arg == "--debug" {
			debug = true
		} else if arg == "-h" || arg == "--help" {
			showHelp()
			os.Exit(0)
		} else if arg == "setup" {
			subcommand = "setup"
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
	fmt.Println(`用法: ai [-d] <查询>

子命令:
  ai setup             配置 API 密钥和模型
  ai shell install     安装 shell 集成
  ai shell uninstall   卸载 shell 集成

示例:
  ai 查看内存
  ai 列出占用端口8085的程序
  ai 删除所有.tmp文件`)
}

func main() {
	// 1. 解析参数
	debug, subcommand, subAction, userInput := parseArgs(os.Args[1:])

	// 2. 处理 setup 子命令
	if subcommand == "setup" {
		if err := config.Setup(nil); err != nil {
			fmt.Fprintf(os.Stderr, "-> %v\n", err)
			os.Exit(4)
		}
		os.Exit(0)
	}

	// 2.5 处理 shell 子命令
	if subcommand == "shell" {
		switch subAction {
		case "install":
			if err := shell.Install(); err != nil {
				fmt.Fprintf(os.Stderr, "安装失败: %v\n", err)
				os.Exit(1)
			}
		case "uninstall":
			if err := shell.Uninstall(); err != nil {
				fmt.Fprintf(os.Stderr, "卸载失败: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintln(os.Stderr, "用法: ai shell install | uninstall")
			os.Exit(1)
		}
		os.Exit(0)
	}

	// 2.8 空参数优先显示 help
	if userInput == "" {
		showHelp()
		os.Exit(0)
	}

	// 3. 检查配置文件，不存在则自动 Setup
	if !config.Exists() {
		fmt.Fprintln(os.Stderr, "配置文件不存在，正在引导设置...")
		if err := config.Setup(nil); err != nil {
			fmt.Fprintf(os.Stderr, "设置失败: %v\n", err)
			os.Exit(4)
		}
		fmt.Fprintln(os.Stderr, "设置完成，正在继续...")
	}

	// 4. 加载配置
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(4)
	}

	// 5. 设置调试模式（配置文件或 CLI 标志均可启用）
	log.SetDebug(cfg.Debug || debug)

	// 6. 验证配置
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "配置验证失败，请运行 ai setup: %v\n", err)
		os.Exit(4)
	}

	// 8. 创建 LLM 客户端
	client := llm.New(cfg)

	// 9. 创建命令解析器
	parser := executor.NewParser()

	// 10. 记录开始时间
	startTime := time.Now()

	// 11. 流式聊天，使用 goroutine 接收流
	streamDone := make(chan error, 1)

	go func() {
		_, err := client.StreamChat(userInput, func(chunk string) {
			newCmd := parser.Feed(chunk)
			if newCmd != "" {
				newCmd = strings.ReplaceAll(newCmd, "\r", "")
				fmt.Print(newCmd)
			}
		})
		streamDone <- err
	}()

	// 12. 轮询等待命令完整（分类确定）或流结束
	var streamErr error
	var finalCommand string
	var finalCategory executor.Category
	streamDoneReceived := false

	for {
		if parser.CommandDone() {
			// 命令完整且分类已确定，不等流结束，立即使用 CurrentCommand/Category
			finalCommand = parser.CurrentCommand
			finalCategory = parser.CurrentCategory
			break
		}
		select {
		case streamErr = <-streamDone:
			streamDoneReceived = true
			if streamErr != nil {
				if strings.Contains(streamErr.Error(), "401") || strings.Contains(streamErr.Error(), "unauthorized") {
					fmt.Fprintln(os.Stderr, "API key 无效，请运行 ai setup 重新配置")
				} else {
					fmt.Fprintf(os.Stderr, "请求失败: %v\n", streamErr)
				}
				os.Exit(3)
			}
			// 流正常结束但分类未确定，使用 parser.Finish() 处理残余
			parseResult := parser.Finish()
			finalCommand = parseResult.Command
			finalCategory = parseResult.Category
			break
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if finalCommand == "" {
		fmt.Fprintln(os.Stderr, "AI 未生成命令")
		os.Exit(3)
	}

	fmt.Println()

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
		fmt.Fprintf(os.Stderr, "-> %s\n", reason)
		os.Exit(2)

	case rules.VerdictDangerous:
		approved, addWhite, err := executor.Confirm(finalCategory, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "确认过程出错: %v\n", err)
			os.Exit(1)
		}
		if !approved {
			fmt.Fprintln(os.Stderr, "-> 已取消")
			os.Exit(2)
		}
		if addWhite {
			baseName := executor.ExtractBaseName(finalCommand)
			cfg.AddToWhitelist(baseName)
			fmt.Fprintf(os.Stderr, "-> 已将 %s 加入白名单\n", baseName)
		}

	case rules.VerdictSafe:
		// 直接执行，无操作
	}

	// 18. 执行命令
	_, exitCode, err := executor.Execute(finalCommand)
	if err != nil {
		fmt.Fprintf(os.Stderr, "命令执行失败: %v\n", err)
		os.Exit(1)
	}

	// 19. 写入临时文件
	tmpfile := "/tmp/ai-cmd-" + strconv.Itoa(os.Getppid()) + ".txt"
	os.WriteFile(tmpfile, []byte(finalCommand), 0644)

	// 20. 等待流结束（如果命令先完整，流仍在后台接收 explanation）
	if !streamDoneReceived {
		<-streamDone
	}

	// 21. 根据退出码退出
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	os.Exit(0)
}
