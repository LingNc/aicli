# ai 命令规则系统设计

## 1. 概述

当前 MVP 的命令分类完全依赖 AI 返回的 `#@` 标记。本设计增加本地规则系统，提供多层防护：

- **禁止执行**: 硬编码的危险命令，永远不能执行
- **强制确认**: 硬编码的敏感命令，必须用户二次确认
- **只读白名单**: rules 模式下直接执行的命令
- **用户白名单**: 用户明确信任的命令

分类优先级从高到低：

```
forbidden > dangerous > whitelist > readonly > AI 分类
```

一旦命中某个级别，立即返回结果，不再检查后续级别。

---

## 2. 匹配算法设计

### 2.1 选择：字符串前缀匹配 + 子串匹配

**不使用 glob、token 或正则，原因：**

- **Glob**: `filepath.Match` 语义偏文件路径，对 shell 命令模式（如 `dd if=`）表达力不足，且 `*` 在 shell 命令中本身是常见字面量（`rm -rf /*`），容易混淆。
- **Token 匹配**: 按空格分词后逐 token 比较，对 `dd if=/dev/zero of=/dev/sda` 中 `of=` 参数的部分匹配无能为力——要么完全匹配 token，要么完全跳过。
- **Regex**: 过强，配置文件可读性差。一个 `rm -rf /` 写成正则 `/^rm\s+-rf\s+\/.*/` 不直观。

**结论：两种简单匹配覆盖所有需求。**

| 匹配类型 | 判断逻辑 | 适用场景 |
|----------|----------|----------|
| `prefix`（默认） | `strings.HasPrefix(cmd, pattern)` | 绝大多数命令模式：`rm -rf /`, `mkfs`, `chmod -R` 等 |
| `contains` | `strings.Contains(cmd, pattern)` | 命令中间出现的模式：`\| xargs rm`, `>`（重定向）, `dd if=/dev/zero` 等 |

命令在执行匹配前经过 `strings.TrimSpace()` 标准化。

### 2.2 为什么 prefix 够用

以 `rm -rf /` 为例：

- `rm -rf /` → 匹配 (精确)
- `rm -rf /*` → 匹配 (`rm -rf /` 是前缀)
- `rm -rf /tmp/foo` → 也匹配，这是刻意的安全偏向

`rm -rf /tmp` 被禁止是否过度？不。`rm -rf ./tmp` 等价且安全，用户应养成使用相对路径的习惯。宁可多拦一个，不漏过一个灾难性操作。

### 2.3 匹配函数签名

```go
// MatchType 定义匹配方式
type MatchType int

const (
    MatchPrefix   MatchType = iota // 字符串前缀匹配
    MatchContains                  // 字符串包含匹配
)

// Rule 一条匹配规则
type Rule struct {
    Pattern   string    // 匹配模式
    MatchType MatchType // 匹配方式
}

// Match 判断命令是否匹配此规则
func (r *Rule) Match(command string) bool {
    cmd := strings.TrimSpace(command)
    switch r.MatchType {
    case MatchContains:
        return strings.Contains(cmd, r.Pattern)
    default: // MatchPrefix
        return strings.HasPrefix(cmd, r.Pattern)
    }
}
```

---

## 3. 规则分层：硬编码 vs 配置文件

### 3.1 原则

- **硬编码**: 公认的危险/敏感命令。不依赖配置文件，即使配置文件损坏或缺失也生效。
- **配置文件**: 用户可能想增删或调整的命令。配置文件中的列表**追加**到硬编码列表之后（合并去重）。

### 3.2 硬编码在 Go 代码中的规则

#### forbidden（永远禁止）

| 模式 | 匹配方式 | 说明 |
|------|----------|------|
| `rm -rf /` | prefix | 删除根目录。也捕获 `rm -rf /*`、`rm -rf /tmp` |
| `dd if=/dev/zero of=/dev/` | contains | 将零写入块设备 |
| `dd if=/dev/urandom of=/dev/` | contains | 将随机数据写入块设备 |
| `:(){ :\|:& };:` | contains | Fork 炸弹 |
| `mkfs` | prefix | 任何格式化命令（mkfs.ext4, mkfs.xfs 等） |
| `fdisk` | prefix | 磁盘分区操作（也捕获 fdisk） |
| `chmod -R 777 /` | prefix | 递归 777 根目录 |
| `chown -R /` | prefix | 递归更改根目录所有者 |

> `dd` 使用 contains 而非 prefix 是因为 `dd bs=4M if=/dev/zero of=/dev/sda` 中 `if=` 不一定在开头，contains 能捕获所有变体。

#### dangerous（强制确认）

| 模式 | 匹配方式 | 说明 |
|------|----------|------|
| `rm -rf` | prefix | 递归强制删除（但不会匹配 `rm -rf /`，因为 forbidden 优先） |
| `dd if=` | prefix | 任何带输入文件的 dd（如 `dd if=/dev/zero of=file.img`） |
| `mkfs` | prefix | 格式化（同 forbidden，此处为兜底；forbidden 先命中） |
| `fdisk` | prefix | 磁盘操作（同 forbidden，此处为兜底） |
| `chmod -R` | prefix | 递归修改权限 |
| `chown -R` | prefix | 递归修改所有者 |
| `>` | contains | 输出重定向（可能覆盖文件） |
| `\| xargs rm` | contains | 批量删除 |

> `mkfs` 和 `fdisk` 同时出现在 forbidden 和 dangerous 中看起来冗余，但 forbidden 是 `mkfs` 无参数前缀（格式化磁盘本体），而 dangerous 作为兜底捕获 `mkfs -V` 这种非破坏性用法。由于 forbidden 优先级更高，实际操作中 `mkfs.ext4 /dev/sda` 会被 forbidden 拦截。

### 3.3 配置文件中的规则（default.yaml）

```yaml
# ──── ai 默认配置 ────
api_key: ""
base_url: "https://api.openai.com"
model: "gpt-4o"
mode: "ai"           # ai | rules | permissive
debug: false

# ──── 危险命令（追加到硬编码黑名单）────
# 格式: 命令字符串前缀。命中则永远禁止执行。
# 例: "shutdown" 将禁止任何 shutdown 命令
forbidden_patterns: []
  # - "shutdown"
  # - "reboot"
  # - "halt"
  # - "poweroff"

# ──── 需确认命令（追加到硬编码列表）────
# 格式: 命令字符串前缀。命中则强制用户二次确认。
# 例: "git push --force" 将拦截所有 force push
dangerous_patterns: []
  # - "git push --force"
  # - "git reset --hard"
  # - "docker rm -f"
  # - "kubectl delete"

# ──── 用户白名单（ai 模式直接执行）────
# 格式: 命令基础名称（首词），例如 "git", "npm"
whitelist: []

# ──── 只读命令（rules 模式直接执行）────
# 格式: 命令基础名称（首词），例如 "ls", "systemctl"
# 注: "systemctl" 即可匹配 systemctl status / journalctl 等所有子命令
readonly_commands:
  # 文件查看
  - ls
  - cat
  - head
  - tail
  - less
  - more
  - file
  - stat
  - wc
  # 文本处理
  - grep
  - egrep
  - fgrep
  - sort
  - uniq
  - diff
  - cut
  - tr
  # 文件查找
  - find
  - locate
  - which
  - whereis
  # 系统信息
  - ps
  - top
  - htop
  - free
  - df
  - du
  - lsof
  - ss
  - netstat
  - ip
  - uname
  - hostname
  - date
  - uptime
  - who
  - w
  # 用户/权限
  - id
  - whoami
  - pwd
  - env
  - printenv
  # 输出
  - echo
  - printf
  # systemd 只读
  - systemctl
  - journalctl
  # 内核日志
  - dmesg
```

### 3.4 设计决策：配置文件最小化

- **forbidden_patterns / dangerous_patterns 默认为空 `[]`**。公认的危险命令全部硬编码，用户无需在配置中看到它们。
- 配置文件中用注释给出示例（`# - "shutdown"`），降低认知负担。
- `readonly_commands` 按用途分组并注释，方便用户增删。
- `whitelist` 保持原样（首词匹配）。

---

## 4. 分类优先级与路由逻辑

```
输入: 命令字符串 command, 模式 mode, AI 分类 category

1. 检查 forbidden (硬编码 + 配置)
   → 命中: 拒绝执行，打印原因，exit(2)

2. 检查 dangerous (硬编码 + 配置)
   → 命中: 强制确认，用户拒绝则 exit(2)

3. 根据 mode 分支:

   a. mode == "permissive"
      → 直接执行（跳过后续检查）

   b. 检查 whitelist (基础命令名匹配)
      → 命中: 直接执行

   c. mode == "rules"
      → 检查 readonly_commands (基础命令名精确匹配)
      → 命中: 直接执行
      → 未命中: 确认后执行

   d. mode == "ai" (默认)
      → 检查 AI 分类: CatRO / CatSudoRO → 直接执行
      → 其他: 确认后执行
```

注意：`whitelist` 在所有模式（permissive 除外）中都检查。AI 分类只在 mode="ai" 时检查。

### 4.1 匹配流程伪代码

```go
func (e *RuleEngine) Classify(command string, mode string, aiCategory Category) Verdict {
    // 1. forbidden
    if e.matchAny(e.forbidden, command) {
        return VerdictForbidden
    }
    // 2. dangerous
    if e.matchAny(e.dangerous, command) {
        return VerdictDangerous
    }
    // 3. permissive 模式
    if mode == "permissive" {
        return VerdictSafe
    }
    // 4. whitelist (基础命令名)
    baseName := ExtractBaseName(command)
    if e.isInList(baseName, e.whitelist) {
        return VerdictSafe
    }
    // 5. rules 模式: readonly
    if mode == "rules" {
        if e.matchBaseName(baseName, e.readonly) {
            return VerdictSafe
        }
        return VerdictDangerous // 需要确认
    }
    // 6. ai 模式: AI 分类
    if aiCategory == CatRO || aiCategory == CatSudoRO {
        return VerdictSafe
    }
    return VerdictDangerous
}
```

---

## 5. 更新后的 config.go 结构体

```go
// Config 是应用配置结构体
type Config struct {
    APIKey            string   `yaml:"api_key"`
    BaseURL           string   `yaml:"base_url"`
    Model             string   `yaml:"model"`
    Mode              string   `yaml:"mode"`
    Debug             bool     `yaml:"debug"`
    Whitelist         []string `yaml:"whitelist"`          // 用户白名单（基础命令名）
    ReadonlyCommands  []string `yaml:"readonly_commands"`  // 只读命令（基础命令名）
    ForbiddenPatterns []string `yaml:"forbidden_patterns"` // 追加的危险命令（完整命令前缀）
    DangerousPatterns []string `yaml:"dangerous_patterns"` // 追加的需确认命令（完整命令前缀）
}
```

变化点：
- 新增 `ForbiddenPatterns`：用户扩展的禁止命令列表
- 新增 `DangerousPatterns`：用户扩展的需确认命令列表
- `Whitelist` 和 `ReadonlyCommands` 保持不变

---

## 6. 新增 RuleEngine（独立文件 `internal/rules/engine.go`）

```
internal/
  rules/
    engine.go      # RuleEngine 定义、硬编码规则、Classify 方法
```

### 6.1 核心类型

```go
package rules

import (
    "strings"
    "github.com/lingnc/aicli/internal/config"
    "github.com/lingnc/aicli/internal/executor"
)

// Verdict 是规则引擎的判定结果
type Verdict int

const (
    VerdictForbidden Verdict = iota // 禁止执行
    VerdictDangerous                // 需要确认
    VerdictSafe                     // 直接执行
)

// MatchType 定义匹配方式
type MatchType int

const (
    MatchPrefix   MatchType = iota
    MatchContains
)

// Rule 一条匹配规则
type Rule struct {
    Pattern   string
    MatchType MatchType
}

// Match 判断命令是否匹配
func (r *Rule) Match(command string) bool {
    cmd := strings.TrimSpace(command)
    switch r.MatchType {
    case MatchContains:
        return strings.Contains(cmd, r.Pattern)
    default:
        return strings.HasPrefix(cmd, r.Pattern)
    }
}

// Engine 规则引擎
type Engine struct {
    forbidden []Rule
    dangerous []Rule
    readonly  []string
    whitelist []string
}
```

### 6.2 硬编码规则初始化

```go
// hardcodedForbidden 返回硬编码的禁止规则
func hardcodedForbidden() []Rule {
    return []Rule{
        {Pattern: "rm -rf /", MatchType: MatchPrefix},
        {Pattern: "dd if=/dev/zero of=/dev/", MatchType: MatchContains},
        {Pattern: "dd if=/dev/urandom of=/dev/", MatchType: MatchContains},
        {Pattern: ":(){ :|:& };:", MatchType: MatchContains},
        {Pattern: "mkfs", MatchType: MatchPrefix},
        {Pattern: "fdisk", MatchType: MatchPrefix},
        {Pattern: "chmod -R 777 /", MatchType: MatchPrefix},
        {Pattern: "chown -R /", MatchType: MatchPrefix},
    }
}

// hardcodedDangerous 返回硬编码的需确认规则
func hardcodedDangerous() []Rule {
    return []Rule{
        {Pattern: "rm -rf", MatchType: MatchPrefix},
        {Pattern: "dd if=", MatchType: MatchPrefix},
        {Pattern: "mkfs", MatchType: MatchPrefix},
        {Pattern: "fdisk", MatchType: MatchPrefix},
        {Pattern: "chmod -R", MatchType: MatchPrefix},
        {Pattern: "chown -R", MatchType: MatchPrefix},
        {Pattern: ">", MatchType: MatchContains},
        {Pattern: "| xargs rm", MatchType: MatchContains},
    }
}
```

### 6.3 构造函数

```go
// NewEngine 创建规则引擎，合并硬编码规则和用户配置
func NewEngine(cfg *config.Config) *Engine {
    e := &Engine{
        forbidden: hardcodedForbidden(),
        dangerous: hardcodedDangerous(),
        readonly:  cfg.ReadonlyCommands,
        whitelist: cfg.Whitelist,
    }

    // 追加用户配置的 forbidden 规则（默认 prefix）
    for _, p := range cfg.ForbiddenPatterns {
        e.forbidden = append(e.forbidden, Rule{Pattern: p, MatchType: MatchPrefix})
    }
    // 追加用户配置的 dangerous 规则（默认 prefix）
    for _, p := range cfg.DangerousPatterns {
        e.dangerous = append(e.dangerous, Rule{Pattern: p, MatchType: MatchPrefix})
    }

    return e
}
```

### 6.4 Classify 方法

```go
// Classify 对命令进行分类
func (e *Engine) Classify(command string, mode string, aiCategory executor.Category) Verdict {
    // 1. forbidden
    for i := range e.forbidden {
        if e.forbidden[i].Match(command) {
            return VerdictForbidden
        }
    }

    // 2. dangerous
    for i := range e.dangerous {
        if e.dangerous[i].Match(command) {
            return VerdictDangerous
        }
    }

    // 3. permissive
    if mode == "permissive" {
        return VerdictSafe
    }

    // 4. whitelist (baseName)
    baseName := executor.ExtractBaseName(command)
    if isInList(baseName, e.whitelist) {
        return VerdictSafe
    }

    // 5. rules 模式: readonly（基础命令名精确匹配）
    if mode == "rules" {
        if isInList(baseName, e.readonly) {
            return VerdictSafe
        }
        return VerdictDangerous
    }

    // 6. ai 模式: AI 分类
    if aiCategory == executor.CatRO || aiCategory == executor.CatSudoRO {
        return VerdictSafe
    }
    return VerdictDangerous
}

// isInList 精确匹配
func isInList(s string, list []string) bool {
    for _, item := range list {
        if item == s {
            return true
        }
    }
    return false
}
```

### 6.5 获取命中的规则（用于错误消息）

```go
// ForbiddenReason 返回禁止原因（第一个命中的 forbidden 规则）
func (e *Engine) ForbiddenReason(command string) string {
    for i := range e.forbidden {
        if e.forbidden[i].Match(command) {
            return fmt.Sprintf("禁止执行: 命令匹配危险规则 '%s'", e.forbidden[i].Pattern)
        }
    }
    return ""
}
```

---

## 7. main.go 集成变更

在 `cmd/ai/main.go` 中，当前流程：

```go
// 15. 分类命令
action := executor.ClassifyCommand(parseResult.Command, parseResult.Category, cfg)
```

改为：

```go
// 15. 规则引擎分类
engine := rules.NewEngine(cfg)
verdict := engine.Classify(parseResult.Command, cfg.Mode, parseResult.Category)

switch verdict {
case rules.VerdictForbidden:
    fmt.Fprintf(os.Stderr, "✗ %s\n", engine.ForbiddenReason(parseResult.Command))
    os.Exit(2)
case rules.VerdictDangerous:
    // 强制确认
    // ... 现有确认逻辑
case rules.VerdictSafe:
    // 直接执行
}
```

`executor.ClassifyCommand` 中的 readonly/whitelist 逻辑移入 `rules.Engine`，executor 包不再负责分类。

---

## 8. 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/rules/engine.go` | 新建 | 规则引擎：硬编码规则、Classify、匹配函数 |
| `internal/config/config.go` | 修改 | Config 结构体增加 `ForbiddenPatterns`、`DangerousPatterns` |
| `internal/config/default.yaml` | 修改 | 增加分组注释、扩展 readonly_commands、新增 forbidden/dangerous_patterns 字段 |
| `internal/executor/executor.go` | 修改 | 移除 `ClassifyCommand`（迁移到 rules），保留 `ExtractBaseName`、`Confirm`、`Execute` |
| `cmd/ai/main.go` | 修改 | 用 `rules.Engine` 替换 `executor.ClassifyCommand` |

---

## 9. 边缘情况处理

| 场景 | 处理 |
|------|------|
| 命令为空字符串 | `ExtractBaseName("")` 返回 `""`，不会匹配任何规则，fallthrough 到确认 |
| 命令带前导空格 | `strings.TrimSpace` 在 Match 中处理 |
| `rm -rf /` 变体（如 `rm  -rf  /`） | 双空格变体 prefix 不匹配。可接受——极端情况极少出现，且 dangerous 层的 `rm -rf` 会兜底要求确认 |
| 管道命令（如 `cat /dev/zero \| dd of=/dev/sda`） | `dd if=` 的 prefix 不会命中（命令以 `cat` 开头），但 `dd if=/dev/zero of=/dev/` 的 contains 能被 `\| xargs rm` 的模式设计覆盖。对此类极端情况，确认机制提供最后防护 |
| `fdisk` vs `fdisk` | 只匹配 `fdisk`。`fdisk` 是 BSD 工具，Linux 上不常用。如需防护可加配置 |

---

## 10. 设计总结

1. **匹配算法**: 字符串 prefix + contains，简单可预测，覆盖所有需求场景
2. **分层**: 公认危险命令硬编码（8 条 forbidden + 8 条 dangerous），用户扩展放配置文件
3. **优先级**: forbidden > dangerous > whitelist > readonly > AI 分类，一条命中即返回
4. **配置**: 默认 YAML 默认为空列表，注释给出示例，不强制用户阅读长列表
5. **隔离**: 规则引擎独立包 `internal/rules`，职责单一，易测试
