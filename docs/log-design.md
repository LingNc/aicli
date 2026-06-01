# 调试日志系统设计

## 1. 现有输出点清单

### 1.1 按语义分类

| 分类 | 位置 | 当前方式 | 语义 |
|------|------|----------|------|
| **Debug** | `internal/llm/client.go:84-85` | `log.Debug(...)` | API 请求/响应详情 |
| **Debug** | `internal/llm/client.go:120` | `log.Debug(...)` | SSE 解析失败诊断 |
| **Debug** | `internal/llm/client.go:140-141` | `log.Debug(...)` | 完整响应内容、耗时 |
| **Debug** | `cmd/ai/main.go:206-208` | `log.Debug(...)` | 分类结果、耗时、命令 |
| **Error** | `cmd/ai/main.go:65` | `fmt.Fprintf(os.Stderr, "-> %v\n")` | setup 失败 |
| **Error** | `cmd/ai/main.go:76,81` | `fmt.Fprintf(os.Stderr, "...失败: %v\n")` | shell install/uninstall 失败 |
| **Error** | `cmd/ai/main.go:85` | `fmt.Fprintln(os.Stderr, "用法:...")` | shell 子命令错误 |
| **Error** | `cmd/ai/main.go:101` | `fmt.Fprintf(os.Stderr, "设置失败: %v\n")` | 自动 setup 失败 |
| **Error** | `cmd/ai/main.go:110` | `fmt.Fprintf(os.Stderr, "加载配置失败: %v\n")` | 配置加载失败 |
| **Error** | `cmd/ai/main.go:119` | `fmt.Fprintf(os.Stderr, "配置验证失败...")` | 配置验证失败 |
| **Error** | `cmd/ai/main.go:185` | `fmt.Fprintln(os.Stderr, "API key 无效...")` | 401 认证错误 |
| **Error** | `cmd/ai/main.go:187` | `fmt.Fprintf(os.Stderr, "请求失败: %v\n")` | 通用 API 错误 |
| **Error** | `cmd/ai/main.go:200` | `fmt.Fprintln(os.Stderr, "AI 未生成命令")` | LLM 未产出命令 |
| **Error** | `cmd/ai/main.go:227` | `fmt.Fprintf(os.Stderr, "确认过程出错: %v\n")` | 确认交互失败 |
| **Error** | `cmd/ai/main.go:247` | `fmt.Fprintf(os.Stderr, "命令执行失败: %v\n")` | 命令执行异常 |
| **Info** | `cmd/ai/main.go:99` | `fmt.Fprintln(os.Stderr, "配置文件不存在...")` | 自动 setup 开始 |
| **Info** | `cmd/ai/main.go:104` | `fmt.Fprintln(os.Stderr, "设置完成...")` | 自动 setup 完成 |
| **Info** | `cmd/ai/main.go:221` | `fmt.Fprintf(os.Stderr, "-> %s\n")` | 禁止原因 |
| **Info** | `cmd/ai/main.go:231` | `fmt.Fprintln(os.Stderr, "-> 已取消")` | 用户取消确认 |
| **Info** | `cmd/ai/main.go:237` | `fmt.Fprintf(os.Stderr, "-> 已将 %s 加入白名单\n")` | 白名单添加 |
| **Warn** | `internal/config/config.go:332` | `fmt.Fprintf(os.Stderr, "警告: 保存白名单失败")` | 非致命失败 |
| **Print** | `cmd/ai/main.go:45` | `fmt.Println(...)` | help 文本 |
| **Print** | `cmd/ai/main.go:150,153` | `fmt.Print(...)` | 流式命令展示 |
| **Print** | `cmd/ai/main.go:159` | `fmt.Println()` | 命令后换行 |
| **Print** | `internal/shell/shell.go:78,93` | `fmt.Printf("-> ...\n")` | shell 安装成功 |
| **Print** | `internal/shell/shell.go:114` | `fmt.Printf("未找到...\n")` | shell 卸载跳过 |
| **Print** | `internal/shell/shell.go:135` | `fmt.Printf("-> ...\n")` | shell 卸载成功 |
| **Print** | `internal/config/config.go:314,319` | `fmt.Println("-> ...")` | setup 保存成功 |
| **Interactive** | `internal/config/config.go:196-239` | raw mode terminal I/O | setup 验证重试 |
| **Interactive** | `internal/executor/executor.go:22-74` | raw mode terminal I/O | 命令确认 |
| **Subprocess** | `internal/config/config.go:283-284` | `cmd.Stdout/Stderr = os.Stdout/Stderr` | 编辑器进程 |
| **Subprocess** | `internal/executor/executor.go:85-86` | `cmd.Stdout/Stderr = os.Stdout/Stderr` | 用户命令执行 |

### 1.2 不适合封装的情况

以下场景因其特殊性，**不纳入**统一输出封装：

- **raw mode 终端交互**（`config.go` promptRetry、`executor.go` Confirm）：涉及 `term.MakeRaw`、单字节读取、ANSI 控制序列，是独立功能，不是通用输出。
- **子进程 stdout/stderr 绑定**（编辑器启动、命令执行）：需要直接绑定 `os.Stdout`/`os.Stderr` 到子进程，不经过格式化输出。

## 2. log 包 API 设计

### 2.1 完整函数签名

```go
package log

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    "sync"
    "time"
)

// ──── 生命周期 ────

// Init 初始化日志系统。
//   debug: 是否启用调试模式（写入日志文件）
//   consoleDebug: 调试模式下，debug 级别日志是否同时输出到控制台
//   logDir: 日志目录，空字符串使用默认值 ~/.aicli/log/
// 非 debug 模式不创建文件，行为和当前代码一致（直接写 stderr/stdout）。
func Init(debug bool, consoleDebug bool, logDir string) error

// Close 关闭日志文件。如果是 debug 模式，返回日志文件路径；否则返回空字符串。
// 调用方用返回值提示用户日志位置。
func Close() string

// IsDebug 返回是否处于调试模式。
func IsDebug() bool

// ──── 输出函数 ────

// Debug 输出调试信息。仅在调试模式下写入。
// 始终写入日志文件（带时间戳），是否写控制台由 consoleDebug 配置决定。
func Debug(format string, args ...interface{})

// Info 输出信息到 stderr，始终显示。
// 调试模式下同时写入日志文件（带时间戳）。
func Info(format string, args ...interface{})

// Warn 输出警告到 stderr，始终显示。
// 调试模式下同时写入日志文件（带时间戳）。
func Warn(format string, args ...interface{})

// Error 输出错误到 stderr，始终显示。
// 调试模式下同时写入日志文件（带时间戳）。
func Error(format string, args ...interface{})

// Fatal 输出错误并退出（exit code 1）。
// 即使非 debug 模式也确保输出后退出。
func Fatal(format string, args ...interface{})

// Print 输出到 stdout。用于 help 文本、shell 状态消息、流式命令展示。
// 调试模式下同时写入日志文件（带时间戳），不包含 ANSI 转义序列。
func Print(format string, args ...interface{})

// PrintRaw 输出原始内容到 stdout，不加换行、不转义。
// 用于流式命令展示（逐 chunk 输出）等场景。
// 调试模式下同时写入日志文件。
func PrintRaw(s string)

// ──── 终端控制（仅控制台，不写日志文件）────

// ClearStderrLine 清除 stderr 当前行（\033[2K\r）。仅控制台，不写日志。
func ClearStderrLine()

// Bell 响铃（\a）。仅控制台，不写日志。
func Bell()
```

### 2.2 行为矩阵

| 函数 | 非 debug 模式 | debug 模式（文件） | debug 模式（控制台） |
|------|-------------|-------------------|---------------------|
| `Debug` | 无输出 | 写文件（带时间戳） | `consoleDebug=true` 时写 stderr |
| `Info` | 写 stderr | 写文件（带时间戳） | 写 stderr |
| `Warn` | 写 stderr | 写文件（带时间戳） | 写 stderr |
| `Error` | 写 stderr | 写文件（带时间戳） | 写 stderr |
| `Fatal` | 写 stderr + exit(1) | 写文件 + 写 stderr + exit(1) | 同左 |
| `Print` | 写 stdout | 写文件（带时间戳） | 写 stdout |
| `PrintRaw` | 写 stdout | 写文件（无时间戳，追加） | 写 stdout |
| `ClearStderrLine` | ANSI 序列到 stderr | 无文件输出 | ANSI 序列到 stderr |
| `Bell` | `\a` 到 stderr | 无文件输出 | `\a` 到 stderr |

### 2.3 内部实现要点

```go
var (
    debug        bool
    consoleDebug bool
    mu           sync.Mutex
    logFile      *os.File  // debug 模式持有，非 debug 为 nil
    logPath      string
)

// 写日志文件的内部辅助函数
func writeFile(level, format string, args ...interface{}) {
    if logFile == nil {
        return
    }
    mu.Lock()
    defer mu.Unlock()
    ts := time.Now().Format("2006-01-02 15:04:05")
    msg := fmt.Sprintf(format, args...)
    fmt.Fprintf(logFile, "[%s] [%s] %s\n", ts, level, msg)
}
```

### 2.4 调用方迁移对照

| 当前代码 | 替换为 |
|----------|--------|
| `fmt.Fprintf(os.Stderr, "-> %v\n", err)` | `log.Info("-> %v", err)` 或 `log.Error("%v", err)` |
| `fmt.Fprintln(os.Stderr, "配置文件不存在...")` | `log.Info("配置文件不存在，正在引导设置...")` |
| `fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)` | `log.Error("加载配置失败: %v", err)` |
| `fmt.Fprintf(os.Stderr, "警告: 保存白名单失败: %v\n", err)` | `log.Warn("保存白名单失败: %v", err)` |
| `fmt.Println("用法: ai [-d] ...")` | `log.Print("用法: ai [-d] ...")` |
| `fmt.Print("$ " + newCmd)` | `log.PrintRaw("$ " + newCmd)` |
| `fmt.Println()` | `log.Print("")` |
| `fmt.Fprint(os.Stderr, "\033[2K\r")` | `log.ClearStderrLine()` |
| `fmt.Fprintf(os.Stderr, "\a")` | `log.Bell()` |
| `fmt.Printf("-> 已安装到 %s\n", rcPath)` | `log.Print("-> 已安装到 %s", rcPath)` |

## 3. 配置项设计

### 3.1 Config 结构体新增字段

```go
type Config struct {
    // ... 现有字段保持不变 ...
    Debug             bool     `yaml:"debug"`
    DebugLogConsole   bool     `yaml:"debug_log_console"`   // 新增：debug 日志是否显示在控制台
    DebugLogDir       string   `yaml:"debug_log_dir"`       // 新增：日志目录，空表示默认
    // ...
}
```

### 3.2 default.yaml 新增项

```yaml
# ──── 调试日志 ────
debug: false
debug_log_console: true   # 调试模式下，debug 日志是否显示在控制台
debug_log_dir: ""         # 日志目录，空表示 ~/.aicli/log/
```

### 3.3 配置语义

- `debug: true` -- 启用调试模式，所有输出写入日志文件。CLI `-d` / `--debug` 标志效果等同。
- `debug_log_console: true` -- 调试模式下，`log.Debug()` 消息同步输出到 stderr。对 `Info/Error/Warn/Print` 无影响（它们始终输出控制台）。
- `debug_log_dir: ""` -- 空字符串使用默认值 `~/.aicli/log/`。非空时使用指定路径（支持 `~` 展开）。

## 4. 日志文件设计

### 4.1 文件命名规则

```
~/.aicli/log/2026-06-01_21-30-00.log
```

格式：`YYYY-MM-DD_HH-MM-SS.log`，精确到秒。同一秒内多次运行（罕见）不会冲突，因为 `os.OpenFile` 使用 `O_CREATE|O_EXCL` 模式，冲突时自动追加序号：

```
~/.aicli/log/2026-06-01_21-30-00.log
~/.aicli/log/2026-06-01_21-30-00_1.log
~/.aicli/log/2026-06-01_21-30-00_2.log
```

每次运行生成新文件，不覆盖旧文件。

### 4.2 日志格式

```
[2026-06-01 21:30:00] [DEBUG] 请求 URL: https://api.openai.com/v1/chat/completions
[2026-06-01 21:30:01] [INFO] -> 配置已保存
[2026-06-01 21:30:05] [ERROR] 加载配置失败: permission denied
```

### 4.3 调试结束提示

在 `main()` 退出前调用 `log.Close()`，若返回非空路径则输出提示：

```
-> 调试日志已保存: ~/.aicli/log/2026-06-01_21-30-00.log
```

## 5. 实施计划

### 步骤 1：重构 log 包

**文件**: `internal/log/log.go`

1. 新增 `Init(debug, consoleDebug bool, logDir string) error`
   - debug=false: 不创建文件，维持现有行为
   - debug=true: 创建 `logDir`（默认 `~/.aicli/log/`），打开带时间戳的日志文件
2. 新增 `Close() string` -- flush + close 文件，返回路径
3. 重构 `Debug()` -- 按 2.2 行为矩阵实现
4. 新增 `Info()`, `Warn()`, `Error()`, `Fatal()`, `Print()`, `PrintRaw()`
5. 新增 `ClearStderrLine()`, `Bell()`
6. 使用 `sync.Mutex` 保护文件写入

**验证**: 包内写测试，验证 debug=true/false 时各函数行为正确。

### 步骤 2：扩展 Config

**文件**: `internal/config/config.go` + `internal/config/default.yaml`

1. Config 结构体新增 `DebugLogConsole bool` 和 `DebugLogDir string`
2. default.yaml 新增默认值
3. fillDefaults 中处理 `DebugLogDir` 默认值（空则用 `~/.aicli/log/`）

**验证**: 加载配置后字段值正确。

### 步骤 3：迁移 main.go

**文件**: `cmd/ai/main.go`

1. 在加载配置后调用 `log.Init(cfg.Debug || debug, cfg.DebugLogConsole, cfg.DebugLogDir)`
2. 在 `os.Exit()` 前调用 `log.Close()`（使用 defer）
3. 替换所有 `fmt.Fprint*(os.Stderr, ...)` 为对应 `log.*` 函数
4. 替换所有 `fmt.Print*` (stdout) 为 `log.Print/PrintRaw`
5. 替换 ANSI 控制序列为 `log.ClearStderrLine()`

**验证**: 编译通过，功能回归。

### 步骤 4：迁移其他包

**文件**: `internal/shell/shell.go`, `internal/config/config.go`, `internal/executor/executor.go`

1. `shell.go`: `fmt.Printf` -> `log.Print`
2. `config.go`: `fmt.Fprintf(os.Stderr, ...)` -> `log.Warn`（仅 AddToWhitelist 中的警告）
3. `config.go`: `fmt.Println(...)` -> `log.Print`（配置保存成功等 stdout 输出）
4. `config.go`: `fmt.Fprintf(os.Stderr, "\a")` -> `log.Bell()`
5. `executor.go`: `fmt.Fprint(os.Stderr, "\033[2K\r")` -> `log.ClearStderrLine()`
6. `executor.go`: `fmt.Fprintln/os.Stderr` -> `log.Info`
7. `executor.go`: `fmt.Fprintf(os.Stderr, ...)` -> `log.Info`

注意：`config.go` 中 `promptRetry()` 的 raw mode 交互代码不迁移（见 1.2），但其中的 `fmt.Fprintf(os.Stderr, ...)` 错误输出可以迁移。

**验证**: 编译通过，功能回归。

### 步骤 5：集成测试

1. 运行 `go build ./...` 确保编译
2. 手动测试 debug 模式：`ai -d "echo hello"`
   - 验证 `~/.aicli/log/` 下生成日志文件
   - 验证日志文件内容格式正确
   - 验证退出提示日志路径
3. 手动测试非 debug 模式：`ai "echo hello"`
   - 验证无日志文件生成
   - 验证控制台输出正常
4. 测试 `debug_log_console: false` 时 debug 日志不显示在控制台

### 迁移优先级

1. **log 包重构** -- 基础依赖
2. **Config 扩展** -- 配置支持
3. **main.go 迁移** -- 最大收益，覆盖主要输出
4. **shell.go 迁移** -- stdout 输出
5. **executor.go 迁移** -- ANSI 控制序列
6. **config.go 迁移** -- 剩余 fmt.Print 调用
