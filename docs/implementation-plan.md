# `ai` MVP - 实施计划

> **Sonnet 实施规划文档**
> **版本**: 1.0
> **基于**: architecture-mvp.md v0.3
> **当前状态**: 部分文件已存在（config.go, default.yaml, prompt.go, client.go, parser.go），需补充缺失部分 + 创建新文件

---

## 0. 现状分析

| 文件 | 状态 | 需要做的事 |
|------|------|-----------|
| `internal/config/config.go` | **已存在，需修改** | 缺少 `Whitelist` 字段、`Setup()` 函数 |
| `internal/config/default.yaml` | **已存在，需修改** | 缺少 `whitelist` 字段 |
| `internal/prompt/prompt.go` | **已存在，需修改** | 缺少 `sudo,ro` 分类说明 |
| `internal/llm/client.go` | **已存在，需修改** | `debugWriter()` 始终返回 `io.Discard`，需改为输出到 stderr |
| `internal/executor/parser.go` | **已存在，完整** | 无需修改 |
| `internal/executor/executor.go` | **不存在** | 需新建 |
| `cmd/ai/main.go` | **不存在** | 需新建 |
| `Makefile` | **不存在** | 需新建 |
| `go.sum` | **不存在** | `go mod tidy` 自动生成 |

---

## 1. `internal/config/default.yaml` (修改)

### 职责
默认配置模板，通过 `go:embed` 嵌入二进制。

### 需要修改的内容

在 `debug: false` 之后添加 `whitelist` 字段:

```yaml
whitelist: []
```

**完整文件内容**:

```yaml
# ai 默认配置
api_key: ""
base_url: "https://api.openai.com"
model: "gpt-4o"
mode: "ai"           # ai | rules | permissive
debug: false
whitelist: []

# 只读命令白名单 (rules 模式使用)
readonly_commands:
  - ls
  # ... (保持现有列表不变)
```

### 注意事项
- `whitelist` 是用户通过 `[a]` 操作动态添加的命令白名单，与 `readonly_commands`（rules 模式用）不同
- 默认为空切片 `[]`，不是 nil

---

## 2. `internal/config/config.go` (修改)

### 职责
配置结构体定义、加载、保存、验证、setup 流程。

### 需要修改的内容

#### 2.1 Config 结构体添加 Whitelist 字段

```go
type Config struct {
    APIKey           string   `yaml:"api_key"`
    BaseURL          string   `yaml:"base_url"`
    Model            string   `yaml:"model"`
    Mode             string   `yaml:"mode"`
    Debug            bool     `yaml:"debug"`
    Whitelist        []string `yaml:"whitelist"`         // 新增
    ReadonlyCommands []string `yaml:"readonly_commands"`
}
```

#### 2.2 新增 `Setup()` 函数

```go
func Setup(originalArgs []string) error
```

**关键实现要点**:

1. 确保 `~/.aicli/` 目录存在 (`os.MkdirAll(dir, 0700)`)
2. 若 config.yaml 不存在，调用 `SaveDefault()` 写入默认配置
3. 选择编辑器: `$EDITOR` > `$VISUAL` > `vi`
4. 备份旧配置内容（用于放弃时恢复）
5. 调用 `exec.Command(editor, configPath).Run()` 打开编辑器（继承 stdin/stdout/stderr，让用户直接编辑）
6. 编辑器退出后，调用 `Load()` + `Validate()` 验证
7. 验证通过: 输出 "配置已保存" 返回 nil
8. 验证失败: 输出错误，提示选项:
   - 读取用户输入（单行 `bufio.Reader`）
   - `e` → 递归调用自身（或循环）
   - `x` → 恢复备份（或删除文件），返回 error
   - `f` → 跳过验证，返回 nil
9. 若 `originalArgs` 非空，setup 完成后提示 "继续执行原始命令"

**需要导入**: `os/exec`, `bufio`, `os`

**注意**: Setup 函数内部使用循环而非递归，避免栈溢出。

#### 2.3 新增 `AddToWhitelist(baseName string)` 方法

```go
func (cfg *Config) AddToWhitelist(baseName string)
```

**关键实现要点**:

1. 检查 `baseName` 是否已在 `cfg.Whitelist` 中（遍历比较），避免重复
2. 若不存在，`append(cfg.Whitelist, baseName)`
3. 调用 `Save(cfg)` 持久化

#### 2.4 新增 `IsWhitelisted(baseName string) bool` 方法

```go
func (cfg *Config) IsWhitelisted(baseName string) bool
```

遍历 `cfg.Whitelist`，精确匹配 `baseName`。

### 依赖
- `os`, `os/exec`, `bufio`, `fmt`, `path/filepath`, `embed`, `gopkg.in/yaml.v3`

### 注意事项
- Setup 中编辑器进程必须继承当前终端的 stdin/stdout/stderr（不能用 `Output()` 或 `CombinedOutput()`），否则用户看不到编辑器
- 验证失败时的交互循环要处理 EOF（用户按 Ctrl+D）

---

## 3. `internal/prompt/prompt.go` (修改)

### 职责
提供发给 LLM 的系统 prompt 常量。

### 需要修改的内容

在分类列表中添加 `sudo,ro`。当前第 15-17 行跳过了 `sudo,ro`:

```
- rm: 删除操作 (rm, find -delete 等删除文件/目录的命令)
- sudo,rw: 需要 root 权限的修改操作
```

改为:

```
- rm: 删除操作 (rm, find -delete 等删除文件/目录的命令)
- sudo,ro: 需要 root 权限的只读操作
- sudo,rw: 需要 root 权限的修改操作
```

同时在规则第 1 条补充"第一个字符必须是命令内容"（与架构文档对齐）:

```
规则:
1. 第一个字符必须是命令内容，不要有任何前缀
2. 命令中不要使用 #@ 序列
```

### 注意事项
- 这是纯字符串常量，修改要与 architecture-mvp.md 第 4.2 节的 prompt 完全一致

---

## 4. `internal/llm/client.go` (修改)

### 职责
OpenAI 兼容 API 的流式 Chat 客户端。

### 需要修改的内容

#### 4.1 修复 `debugWriter()` 方法

当前实现:
```go
func (c *Client) debugWriter() io.Writer {
    return io.Discard
}
```

改为:
```go
func (c *Client) debugWriter() io.Writer {
    if c.Debug {
        return os.Stderr
    }
    return io.Discard
}
```

需要添加 `os` 导入。

#### 4.2 Debug 从 Config 读取

在 `New()` 函数中，从 `cfg.Debug` 设置 `c.Debug`:

```go
func New(cfg *config.Config) *Client {
    return &Client{
        cfg:   cfg,
        http:  &http.Client{Timeout: 60 * time.Second},
        Debug: cfg.Debug,
    }
}
```

### 依赖
- `bufio`, `bytes`, `encoding/json`, `fmt`, `io`, `net/http`, `os`, `strings`, `time`
- `github.com/lingnc/aicli/internal/config`
- `github.com/lingnc/aicli/internal/prompt`

### 注意事项
- SSE 解析中 `bufio.Scanner` 的默认 buffer 是 64KB，对于正常 LLM 响应足够
- HTTP 401 错误需要在调用方（main.go）中特别处理，给出 "API key 无效" 提示

---

## 5. `internal/executor/parser.go` (无修改)

### 职责
流式状态机解析器，解析零前缀 `#@` 格式。

**已完整实现**，经检查与架构文档一致:
- `Feed(chunk string) string` -- 输入 chunk，返回新增命令文本
- `Finish() *Result` -- 处理残余数据，返回最终 Result
- 状态机: COMMAND → METADATA → EXPLANATION → DONE
- 分隔符前缀匹配逻辑正确（尾部 1-3 字节检查）

---

## 6. `internal/executor/executor.go` (新建)

### 职责
命令执行 + 确认交互 + 白名单判断。

### 文件结构

```go
package executor

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "github.com/lingnc/aicli/internal/config"
    "golang.org/x/term"
)
```

### 导出函数/类型

#### `Action` 类型

```go
type Action int

const (
    ActionExecute Action = iota // 直接执行
    ActionConfirm               // 需要确认
    ActionReject                // 拒绝（非终端 stdin）
)
```

#### `ClassifyCommand(command string, cfg *config.Config) (Category, Action)`

**职责**: 根据 mode 和配置判断命令的分类和应采取的动作。

**关键实现要点**:

1. 提取基础命令名: `strings.Fields(command)[0]`，若 command 为空返回 `(CatUnknown, ActionConfirm)`
2. 根据 `cfg.Mode` 分支:
   - `"permissive"`: 返回 `(CatRO, ActionExecute)` -- 全部直接执行
   - `"rules"`: 遍历 `cfg.ReadonlyCommands`，若 `baseName` 匹配则返回 `(CatRO, ActionExecute)`，否则返回 `(CatRW, ActionConfirm)`
   - `"ai"`: 检查白名单 `cfg.IsWhitelisted(baseName)`，若命中返回 `(CatRO, ActionExecute)`；否则根据传入的 category 返回对应 action:
     - `CatRO`, `CatSudoRO` → `ActionExecute`
     - `CatRW`, `CatRM`, `CatSudoRW`, `CatSudoRM`, `CatUnknown` → `ActionConfirm`

**注意**: `ClassifyCommand` 的 `category` 参数由外部（parser 结果）传入，函数签名应为:

```go
func ClassifyCommand(command string, category Category, cfg *config.Config) Action
```

#### `Confirm(category Category, cfg *config.Config) (bool, bool, error)`

**职责**: 执行确认交互，返回 (是否执行, 是否加入白名单, error)。

**返回值**:
- `(true, false, nil)` -- 用户按 y，执行但不加入白名单
- `(true, true, nil)` -- 用户按 a，执行并加入白名单
- `(false, false, nil)` -- 用户拒绝
- `(false, false, nil)` -- stdin 非终端，直接拒绝

**关键实现要点**:

1. 检查 stdin 是否为终端: `term.IsTerminal(int(os.Stdin.Fd()))`
   - 非终端: 输出 "非终端环境，跳过确认" 到 stderr，返回 `(false, false, nil)`
2. 输出确认提示到 stderr: `fmt.Fprintf(os.Stderr, "[a/y/N] ")`
3. 获取 stdin fd: `fd := int(os.Stdin.Fd())`
4. 保存旧终端状态: `oldState, err := term.MakeRaw(fd)`
5. **defer 恢复终端状态**: `defer term.Restore(fd, oldState)`
6. **注册信号处理**: 捕获 SIGINT/SIGTERM，恢复终端后退出
   ```go
   sigCh := make(chan os.Signal, 1)
   signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
   go func() {
       <-sigCh
       term.Restore(fd, oldState)
       os.Exit(130)
   }()
   defer signal.Stop(sigCh)
   ```
7. 读取单字节: `buf := make([]byte, 1); _, err = os.Stdin.Read(buf)`
8. 恢复终端模式（defer 已处理）
9. 输出换行到 stderr: `fmt.Fprintln(os.Stderr)`
10. 判断按键:
    - `buf[0] == 'y' || buf[0] == 'Y'` → 返回 `(true, false, nil)`
    - `buf[0] == 'a' || buf[0] == 'A'` → 返回 `(true, true, nil)`
    - 其他 → 返回 `(false, false, nil)`

#### `Execute(command string) (string, int, error)`

**职责**: 执行 shell 命令，返回 (stdout+stderr, exitCode, error)。

**关键实现要点**:

1. 创建带超时的 context: `ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)`
2. 创建命令: `cmd := exec.CommandContext(ctx, "sh", "-c", command)`
3. 设置 `cmd.Stdout` 和 `cmd.Stderr` 为 `os.Stdout` 和 `os.Stderr`（直接输出到终端，不捕获）
4. 设置 `cmd.Stdin = os.Stdin`（支持 sudo 密码输入等交互式场景）
5. 调用 `cmd.Run()`
6. 错误处理:
   - `context.DeadlineExceeded` → 输出 "命令执行超时 (30s)"，返回 `("", 1, err)`
   - `exec.ExitError` → 提取 `ExitCode()`，返回 `("", exitCode, err)`
   - 其他 error → 返回 `("", 1, err)`
7. 成功 → 返回 `("", 0, nil)`

**注意**: 输出直接 pipe 到终端（不捕获），所以返回的 stdout 字符串为空。这是设计决策 -- 让命令的输出实时显示，而非缓冲后一次性输出。

#### `ExtractBaseName(command string) string`

**职责**: 从命令字符串中提取基础命令名。

```go
func ExtractBaseName(command string) string {
    fields := strings.Fields(command)
    if len(fields) == 0 {
        return ""
    }
    return fields[0]
}
```

### 依赖
- `context`, `fmt`, `os`, `os/exec`, `os/signal`, `strings`, `syscall`, `time`
- `golang.org/x/term`
- `github.com/lingnc/aicli/internal/config`

### 注意事项
- 终端 raw mode 和信号处理是**最容易出错的部分**，必须确保:
  - `term.Restore` 在任何退出路径都被调用（defer）
  - 信号处理 goroutine 在 Confirm 返回后被清理（signal.Stop）
  - panic 时终端状态也能恢复（defer 在 signal handler 之前注册）
- `exec.CommandContext` 在超时时发送 SIGKILL，不会给进程清理机会。这是 MVP 的简化选择。
- stdin 非终端时直接拒绝而非跳过，是为了安全 -- 管道输入场景不应自动执行命令。

---

## 7. `cmd/ai/main.go` (新建)

### 职责
入口 + 参数解析 + 流程编排。

### 文件结构

```go
package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/lingnc/aicli/internal/config"
    "github.com/lingnc/aicli/internal/executor"
    "github.com/lingnc/aicli/internal/llm"
)
```

### 关键实现要点

#### 7.1 参数解析

```
os.Args[1:] 中:
  - "-d" 或 "--debug" → debug = true，从 args 中移除
  - "setup" → 进入 setup 流程
  - 其余参数拼接为 userInput (strings.Join)
```

解析逻辑:
```go
func parseArgs(args []string) (debug bool, subcommand string, userInput string) {
    remaining := args
    // 检查第一个参数是否为 debug flag
    if len(remaining) > 0 && (remaining[0] == "-d" || remaining[0] == "--debug") {
        debug = true
        remaining = remaining[1:]
    }
    // 检查是否为 setup 子命令
    if len(remaining) > 0 && remaining[0] == "setup" {
        return debug, "setup", ""
    }
    // 其余拼接为 user input
    return debug, "", strings.Join(remaining, " ")
}
```

#### 7.2 main() 流程编排

```
main()
  ├── 1. parseArgs(os.Args[1:])
  ├── 2. if subcommand == "setup" → config.Setup(os.Args[1:]) → exit
  ├── 3. if !config.Exists() → 自动 setup，完成后继续
  ├── 4. cfg := config.Load()
  │      err → "配置加载失败" → exit 4
  ├── 5. if debug → cfg.Debug = true
  ├── 6. config.Validate(cfg)
  │      err → "配置验证失败，请运行 ai setup" → exit 4
  ├── 7. if userInput == "" → "用法: ai [-d] <查询>" → exit 1
  ├── 8. client := llm.New(cfg)
  ├── 9. 创建 parser: parser := executor.NewParser()
  ├── 10. 调用 LLM:
  │      result, err := client.StreamChat(userInput, func(chunk string) {
  │          newCmd := parser.Feed(chunk)
  │          fmt.Print(newCmd)  // 流式输出命令到终端
  │      })
  │      err → 根据错误类型输出提示 → exit 3
  ├── 11. 解析完成:
  │      parseResult := parser.Finish()
  │      if parseResult.Command == "" → "AI 未生成命令" → exit 3
  │      fmt.Println()  // 命令后的换行
  ├── 12. 分类判断:
  │      action := executor.ClassifyCommand(parseResult.Command, parseResult.Category, cfg)
  ├── 13. 确认交互 (if action == ActionConfirm):
  │      exec, addWhite, err := executor.Confirm(parseResult.Category, cfg)
  │      err → exit 1
  │      !exec → "已取消" → exit 2
  │      addWhite → baseName := executor.ExtractBaseName(parseResult.Command)
  │                  cfg.AddToWhitelist(baseName)
  │                  fmt.Println("已将", baseName, "加入白名单")
  ├── 14. 执行命令:
  │      _, exitCode, err := executor.Execute(parseResult.Command)
  │      err → 输出错误 → exit 1
  ├── 15. 写入临时文件 (shell history 集成):
  │      tmpfile := "/tmp/ai-cmd-" + strconv.Itoa(os.Getppid()) + ".txt"
  │      os.WriteFile(tmpfile, []byte(parseResult.Command), 0644)
  ├── 16. exit 0
```

#### 7.3 调试输出

在关键步骤输出 `[DEBUG]` 信息到 stderr:
- 配置加载路径
- API 请求 URL
- 分类决策结果
- 执行耗时

使用统一函数:
```go
func debugLog(debug bool, format string, args ...interface{}) {
    if debug {
        fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
    }
}
```

#### 7.4 临时文件写入 (Shell History)

```go
func writeCmdTmpFile(command string) {
    // 使用 ppid (父进程 PID) 作为文件名，与 shell wrapper 中的 $$ 对应
    tmpfile := fmt.Sprintf("/tmp/ai-cmd-%d.txt", os.Getppid())
    os.WriteFile(tmpfile, []byte(command), 0644)
}
```

**注意**: shell wrapper 中使用 `$$`（shell PID），而 Go 二进制的 `os.Getppid()` 返回的就是调用它的 shell 的 PID。两者一致。

#### 7.5 退出码

| 场景 | 退出码 |
|------|--------|
| 成功 | 0 |
| 一般错误（参数、执行失败） | 1 |
| 用户拒绝 | 2 |
| LLM API 错误 | 3 |
| 配置错误 | 4 |

使用 `os.Exit(code)` 退出。

### 依赖
- `fmt`, `os`, `strconv`, `strings`
- `github.com/lingnc/aicli/internal/config`
- `github.com/lingnc/aicli/internal/executor`
- `github.com/lingnc/aicli/internal/llm`

### 注意事项
- main.go 不包含任何 LLM 协议细节、解析逻辑、配置细节 -- 只做编排
- 流式输出使用 `fmt.Print(newCmd)` 而非 `fmt.Println`，因为 parser 返回的文本已包含必要的字符
- LLM 调用失败时，需要区分 HTTP 401（API key 无效）和其他错误，给出不同提示

---

## 8. `Makefile` (新建)

### 目标定义

```makefile
.PHONY: build run test clean install lint

# 编译，strip debug 信息
build:
	go build -ldflags="-s -w" -o ai ./cmd/ai/

# 编译并运行，支持传参: make run ARGS="查看内存"
run: build
	./ai $(ARGS)

# 运行所有测试
test:
	go test -v ./...

# 清理编译产物
clean:
	rm -f ai

# 安装到 /usr/local/bin
install: build
	cp ai /usr/local/bin/ai

# 静态分析
lint:
	go vet ./...
```

---

## 9. 依赖初始化

实施开始时需要执行:

```bash
cd /home/lingnc/workspace/aicli
go get gopkg.in/yaml.v3
go get golang.org/x/term
go mod tidy
```

当前 `go.mod` 中的 module 名是 `github.com/lingnc/aicli`，所有内部包导入路径基于此。

---

## 10. 实施顺序（按依赖关系）

```
Step 1: config/default.yaml  (修改 - 添加 whitelist 字段)
Step 2: config/config.go      (修改 - 添加 Whitelist/Setup/AddToWhitelist/IsWhitelisted)
Step 3: prompt/prompt.go      (修改 - 添加 sudo,ro 分类)
Step 4: llm/client.go         (修改 - 修复 debugWriter)
Step 5: executor/executor.go  (新建 - 命令执行+确认交互)
Step 6: cmd/ai/main.go        (新建 - 入口编排)
Step 7: Makefile              (新建)
Step 8: go mod tidy           (生成 go.sum)
Step 9: go build ./cmd/ai/    (验证编译)
Step 10: 手动测试基本流程
```

每一步完成后验证:
1. `go vet ./...` -- 无错误
2. `go build ./cmd/ai/` -- 编译通过

---

## 11. 关键风险点

### 11.1 流式解析状态机 (parser.go -- 已完成)

已实现且经检查正确。核心风险是 chunk 边界断在 `\n#@ ` 中间，当前实现通过尾部前缀匹配（1-3 字节）正确处理。

### 11.2 终端 raw mode 管理 (executor.go)

**风险**: 终端状态未恢复导致 shell 不可用。

**缓解**:
- `defer term.Restore(fd, oldState)` 作为第一行 defer
- 信号处理 goroutine 中也调用 Restore
- 测试时用 `stty sane` 恢复

### 11.3 SSE 解析 (client.go -- 已完成)

**风险**: `bufio.Scanner` 默认 max token 64KB，正常 LLM 响应不会超过。但如果 LLM 返回异常长内容，可能被截断。

**缓解**: MVP 接受此限制。后续可增大 buffer。

### 11.4 Shell History 临时文件

**风险**: 并发调用 `ai` 时，同一 ppid 的临时文件可能被覆盖。

**缓解**: MVP 接受此限制。shell wrapper 中 `$$` 对于同一 shell 会话是唯一的，正常用户不会并发调用。

---

## 12. 测试策略

### 单元测试 (可选，MVP 优先编译通过)

| 测试文件 | 测试内容 |
|---------|---------|
| `internal/executor/parser_test.go` | 状态机各种 chunk 边界情况 |
| `internal/config/config_test.go` | 配置加载、默认值填充、验证 |

### parser_test.go 关键用例

1. 完整单行命令: `"ls -la\n#@ ro\n列出文件"` → Command="ls -la", Category="ro"
2. 多行命令: `"find . -name '*.go'\n -exec grep TODO {} +\n#@ ro\n查找TODO"` → 命令包含换行
3. chunk 边界断在 `\n#@ ` 中间: 分两个 chunk 传入
4. 无 `#@` 标记: 命令后直接 EOF → Category=""
5. 空输入: `""` → Command=""
6. 分隔符出现在命令内容中: `"echo '#@ test'\n#@ ro\n说明"` → 命令保留 `#@`

---

*本文档由 Sonnet 规划层输出，Haiku 按此文档逐文件实施。*
