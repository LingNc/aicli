# 规则引擎实施计划

## 0. 依赖方向分析（循环依赖解决方案）

**问题**：`rules` 包需要引用 `executor.Category` 和 `executor.ExtractBaseName`，而设计意图是让 `executor.ClassifyCommand` 被 `rules.Engine.Classify` 替代。

**解决方案**：依赖方向为 `rules -> executor`，单向即可。

- `rules` 包 import `executor`（使用 `executor.Category`、`executor.CatRO`、`executor.CatSudoRO`、`executor.ExtractBaseName`）
- `executor` 包 **不** import `rules`（`ClassifyCommand` 直接删除，不留引用）
- `main` 包 import `rules` 和 `executor`，用 `rules.Engine.Classify` 替代 `executor.ClassifyCommand`

不存在循环依赖。

---

## 1. 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/rules/engine.go` | **新建** | 规则引擎核心：Rule/MatchType/Engine 类型、硬编码规则、NewEngine、Classify、ForbiddenReason |
| `internal/config/config.go` | **修改** | Config 结构体新增 `ForbiddenPatterns`、`DangerousPatterns` 字段 |
| `internal/config/default.yaml` | **修改** | 新增 `forbidden_patterns`、`dangerous_patterns` 字段；扩充 `readonly_commands` 列表 |
| `internal/executor/executor.go` | **修改** | 删除 `ClassifyCommand` 函数和 `Action` 类型；保留 `ExtractBaseName`、`Confirm`、`Execute` |
| `cmd/ai/main.go` | **修改** | import `rules` 包；用 `rules.Engine.Classify` 替换 `executor.ClassifyCommand`；处理 `VerdictForbidden` |
| `internal/rules/engine_test.go` | **新建** | 规则引擎单元测试 |

---

## 2. 每个文件的具体变更

### 2.1 `internal/rules/engine.go`（新建）

**包**：`package rules`

**import**：
```go
import (
    "fmt"
    "strings"
    "github.com/lingnc/aicli/internal/config"
    "github.com/lingnc/aicli/internal/executor"
)
```

**类型定义**：

```go
// Verdict 规则引擎判定结果
type Verdict int
const (
    VerdictForbidden Verdict = iota
    VerdictDangerous
    VerdictSafe
)

// MatchType 匹配方式
type MatchType int
const (
    MatchPrefix   MatchType = iota
    MatchContains
)

// Rule 单条匹配规则
type Rule struct {
    Pattern   string
    MatchType MatchType
}

// Match 判断命令是否匹配此规则
func (r *Rule) Match(command string) bool

// Engine 规则引擎
type Engine struct {
    forbidden []Rule
    dangerous []Rule
    readonly  []string
    whitelist []string
}
```

**函数**：

| 函数 | 签名 | 说明 |
|------|------|------|
| `hardcodedForbidden` | `func hardcodedForbidden() []Rule` | 返回 8 条硬编码 forbidden 规则 |
| `hardcodedDangerous` | `func hardcodedDangerous() []Rule` | 返回 8 条硬编码 dangerous 规则 |
| `NewEngine` | `func NewEngine(cfg *config.Config) *Engine` | 合并硬编码 + 配置规则 |
| `Classify` | `func (e *Engine) Classify(command string, mode string, aiCategory executor.Category) Verdict` | 按优先级分类：forbidden > dangerous > permissive > whitelist > readonly > AI |
| `ForbiddenReason` | `func (e *Engine) ForbiddenReason(command string) string` | 返回第一个命中的 forbidden 规则的中文说明 |
| `isInList` | `func isInList(s string, list []string) bool` | 精确字符串匹配（私有辅助函数） |

**`Classify` 方法逻辑**（按优先级从高到低）：

1. 遍历 `e.forbidden`，任一命中返回 `VerdictForbidden`
2. 遍历 `e.dangerous`，任一命中返回 `VerdictDangerous`
3. `mode == "permissive"` 返回 `VerdictSafe`
4. `baseName := executor.ExtractBaseName(command)`，检查 whitelist，命中返回 `VerdictSafe`
5. `mode == "rules"`：检查 readonly 列表，命中返回 `VerdictSafe`，否则返回 `VerdictDangerous`
6. `mode == "ai"`（默认）：`aiCategory` 为 `CatRO` 或 `CatSudoRO` 返回 `VerdictSafe`，否则 `VerdictDangerous`

**硬编码 forbidden 规则**（8 条）：

| Pattern | MatchType |
|---------|-----------|
| `rm -rf /` | Prefix |
| `dd if=/dev/zero of=/dev/` | Contains |
| `dd if=/dev/urandom of=/dev/` | Contains |
| `:(){ :\|:& };:` | Contains |
| `mkfs` | Prefix |
| `fdisk` | Prefix |
| `chmod -R 777 /` | Prefix |
| `chown -R /` | Prefix |

**硬编码 dangerous 规则**（8 条）：

| Pattern | MatchType |
|---------|-----------|
| `rm -rf` | Prefix |
| `dd if=` | Prefix |
| `mkfs` | Prefix |
| `fdisk` | Prefix |
| `chmod -R` | Prefix |
| `chown -R` | Prefix |
| `>` | Contains |
| `\| xargs rm` | Contains |

---

### 2.2 `internal/config/config.go`（修改）

**变更位置**：`Config` 结构体（第 17-25 行）

**新增字段**（在 `ReadonlyCommands` 之后）：

```go
ForbiddenPatterns []string `yaml:"forbidden_patterns"` // 用户扩展的禁止命令
DangerousPatterns []string `yaml:"dangerous_patterns"` // 用户扩展的需确认命令
```

**无其他变更**。`fillDefaults` 不需要为新字段设默认值（`nil` 和 `[]` 等效，规则引擎处理 nil slice 安全）。

**向后兼容**：旧 config.yaml 缺少这两个字段时，`yaml.Unmarshal` 将它们留为 `nil`，`NewEngine` 中 `range nil` 无操作，行为等同空列表。零影响。

---

### 2.3 `internal/config/default.yaml`（修改）

**变更**：

1. 在 `whitelist` 之后新增 `forbidden_patterns` 和 `dangerous_patterns` 字段（空列表 + 注释示例）
2. 扩充 `readonly_commands` 列表，按用途分组并加注释，与设计文档对齐
3. 修正 `systemctl status` 为 `systemctl`（设计使用基础命令名匹配，`systemctl status` 无法匹配 `systemctl start`）

**新内容**大致结构：

```yaml
# ──── ai 默认配置 ────
api_key: ""
base_url: "https://api.openai.com"
model: "gpt-4o"
mode: "ai"           # ai | rules | permissive
debug: false

# ──── 用户白名单（ai 模式直接执行）────
whitelist: []

# ──── 危险命令（追加到硬编码黑名单）────
forbidden_patterns: []
  # - "shutdown"
  # - "reboot"

# ──── 需确认命令（追加到硬编码列表）────
dangerous_patterns: []
  # - "git push --force"
  # - "git reset --hard"

# ──── 只读命令（rules 模式直接执行）────
readonly_commands:
  # 文件查看
  - ls
  - cat
  # ... （完整列表见设计文档 3.3 节）
```

---

### 2.4 `internal/executor/executor.go`（修改）

**删除**：

- `Action` 类型及其常量（`ActionExecute`、`ActionConfirm`）——第 18-23 行
- `ClassifyCommand` 函数——第 26-56 行

**保留不变**：

- `Confirm` 函数（第 60-103 行）
- `Execute` 函数（第 106-129 行）
- `ExtractBaseName` 函数（第 132-138 行）

**import 变更**：删除 `config` 包 import（`ClassifyCommand` 是唯一使用 `config.Config` 的地方）。`Confirm` 接收 `*config.Config` 参数但仅用于 `cfg.IsWhitelisted`——等等，再看一下。

仔细检查：`Confirm` 函数签名是 `func Confirm(category Category, cfg *config.Config) (bool, bool, error)`，但函数体中没有使用 `cfg`（白名单相关逻辑在 `main.go` 中处理）。所以 `config` import 可以删除，`Confirm` 的 `cfg` 参数可以保留（签名不变，避免影响 `main.go` 调用），但 `cfg` 在函数体中未使用。

实际上，`Confirm` 确实没有使用 `cfg` 参数。保持签名不变以避免不必要的改动。import `_ "github.com/lingnc/aicli/internal/config"` 或直接保留 `config` 引用——不过 Go 编译器会报 unused import。

**处理方式**：`Confirm` 函数签名中保留 `cfg *config.Config` 参数（避免改动 `main.go` 的调用代码），但在函数体中用 `_ = cfg` 消除编译器警告。或者更简洁地，直接保持 `config` import 因为 `Confirm` 的参数类型需要它。

---

### 2.5 `cmd/ai/main.go`（修改）

**import 变更**：新增 `"github.com/lingnc/aicli/internal/rules"`

**步骤 15 替换**（当前第 152-153 行）：

```go
// 当前代码
action := executor.ClassifyCommand(parseResult.Command, parseResult.Category, cfg)
```

替换为：

```go
// 新代码
engine := rules.NewEngine(cfg)
verdict := engine.Classify(parseResult.Command, cfg.Mode, parseResult.Category)
```

**步骤 16 替换**（当前第 156-171 行）：

```go
// 当前代码
if action == executor.ActionConfirm {
    // ...
}
```

替换为：

```go
switch verdict {
case rules.VerdictForbidden:
    reason := engine.ForbiddenReason(parseResult.Command)
    if reason == "" {
        reason = "命令被安全规则禁止执行"
    }
    fmt.Fprintf(os.Stderr, "✗ %s\n", reason)
    os.Exit(2)

case rules.VerdictDangerous:
    approved, addWhite, err := executor.Confirm(parseResult.Category, cfg)
    if err != nil {
        fmt.Fprintf(os.Stderr, "确认过程出错: %v\n", err)
        os.Exit(1)
    }
    if !approved {
        fmt.Fprintln(os.Stderr, "✗ 已取消")
        os.Exit(2)
    }
    if addWhite {
        baseName := executor.ExtractBaseName(parseResult.Command)
        cfg.AddToWhitelist(baseName)
        fmt.Fprintf(os.Stderr, "✓ 已将 %s 加入白名单\n", baseName)
    }

case rules.VerdictSafe:
    // 直接执行，无操作
}
```

**步骤 17-19 不变**（执行命令、写临时文件、退出）。

---

### 2.6 `internal/rules/engine_test.go`（新建）

**测试用例**：

| 测试函数 | 覆盖场景 |
|----------|----------|
| `TestRuleMatch` | Rule.Match 基本功能：prefix 匹配、contains 匹配、TrimSpace、不匹配 |
| `TestHardcodedForbidden` | 所有 8 条 forbidden 规则的命中 |
| `TestHardcodedDangerous` | 所有 8 条 dangerous 规则的命中 |
| `TestPriority` | forbidden 优先于 dangerous（`rm -rf /` 命中 forbidden，不命中 dangerous 的 `rm -rf`） |
| `TestClassify_PermissiveMode` | permissive 模式下非 forbidden 命令返回 Safe |
| `TestClassify_RulesMode` | rules 模式下 readonly 命令返回 Safe，非 readonly 返回 Dangerous |
| `TestClassify_AIMode` | ai 模式下 CatRO/CatSudoRO 返回 Safe，其他返回 Dangerous |
| `TestClassify_Whitelist` | whitelist 在所有模式（除 permissive）中生效 |
| `TestUserPatterns` | 配置文件中的 forbidden_patterns/dangerous_patterns 正确追加 |
| `TestForbiddenReason` | 返回正确的禁止原因字符串 |
| `TestEdgeCases` | 空命令、纯空格命令、前导空格命令 |

---

## 3. 实施顺序

按依赖关系从底层到上层：

```
Step 1: config.go + default.yaml
  ├─ Config 新增 ForbiddenPatterns / DangerousPatterns
  ├─ default.yaml 新增字段 + 扩充 readonly_commands
  └─ 验证: go build ./internal/config/...

Step 2: rules/engine.go
  ├─ 创建 internal/rules/ 包
  ├─ 实现 Rule、Engine、NewEngine、Classify、ForbiddenReason
  └─ 验证: go build ./internal/rules/...

Step 3: rules/engine_test.go
  ├─ 编写并运行全部测试
  └─ 验证: go test ./internal/rules/ -v

Step 4: executor/executor.go
  ├─ 删除 ClassifyCommand、Action 类型
  ├─ 处理 Confirm 中未使用的 cfg 参数
  └─ 验证: go build ./internal/executor/...（预期编译失败——main.go 还引用旧函数）

Step 5: main.go
  ├─ import rules 包
  ├─ 替换 ClassifyCommand 为 rules.Engine.Classify
  ├─ 新增 VerdictForbidden 处理分支
  └─ 验证: go build ./cmd/ai/...

Step 6: 集成验证
  ├─ go build ./...
  ├─ go test ./...
  └─ 手动测试: ai "rm -rf /" 应被禁止; ai "ls" 应直接执行
```

**注意**：Step 4 和 Step 5 必须在同一次提交中完成，否则编译会失败。或者先完成 Step 5（main.go 引用 rules），再删除 Step 4（executor 旧代码）。实际操作中建议：先做 Step 5 的 main.go 改写（同时引用 rules 和旧 ClassifyCommand），确认编译通过后再做 Step 4 删除旧代码。

**修订后的安全顺序**：

```
Step 1: config.go + default.yaml          → go build 通过
Step 2: rules/engine.go                   → go build 通过
Step 3: rules/engine_test.go              → go test 通过
Step 4: main.go（改用 rules，暂保留旧代码）  → go build 通过
Step 5: executor/executor.go（删除旧代码）   → go build 通过
Step 6: go test ./... 全量验证
```

---

## 4. 向后兼容

| 场景 | 处理方式 |
|------|----------|
| 旧 config.yaml 无 `forbidden_patterns` | `yaml.Unmarshal` 留为 `nil`，`range nil` 无操作 |
| 旧 config.yaml 无 `dangerous_patterns` | 同上 |
| 旧 config.yaml 中 `readonly_commands` 包含 `systemctl status` | 仍可工作（`isInList("systemctl status", readonly)` 匹配），但不会匹配 `systemctl start`。用户需手动修正为 `systemctl` |
| 旧 config.yaml 无 `readonly_commands` 的新增项 | 用户配置文件不会被自动更新，仅 `default.yaml` 变化。用户运行 `ai setup` 可获取新默认配置 |
| `executor.ClassifyCommand` 被外部引用 | 当前仅 `main.go` 引用，无外部消费者。删除安全 |

**关键原则**：`default.yaml` 通过 `//go:embed` 嵌入，只影响新安装或 `loadDefault()` 场景。已有用户配置文件不会被覆盖。

---

## 5. 验证标准

### 编译验证

```bash
go build ./...
```

### 单元测试

```bash
go test ./internal/rules/ -v -cover
```

覆盖率目标：`engine.go` 行覆盖率 >= 90%。

### 手动功能验证

| 命令 | 预期行为 |
|------|----------|
| `ai "rm -rf /"` | 输出 "禁止执行" 消息，exit(2)，不执行 |
| `ai "rm -rf /tmp/foo"` | 输出 "禁止执行"（prefix 匹配 `rm -rf /`），exit(2) |
| `ai "mkfs.ext4 /dev/sdb"` | 输出 "禁止执行"（prefix 匹配 `mkfs`），exit(2) |
| `ai "rm -rf ./tmp"` | 进入确认流程（dangerous 匹配 `rm -rf`） |
| `ai "ls -la"` | 直接执行（rules 模式下 readonly 匹配 `ls`） |
| `ai "dd if=/dev/zero of=/dev/sda bs=4M"` | 输出 "禁止执行"（contains 匹配 `dd if=/dev/zero of=/dev/`） |
| `ai ":(){ :\|:& };:"` | 输出 "禁止执行"（contains 匹配 fork 炸弹） |

### 配置兼容验证

1. 使用不含新字段的旧 config.yaml 运行，确认无报错
2. 在 config.yaml 中添加 `forbidden_patterns: ["shutdown"]`，确认 `ai "shutdown now"` 被禁止
3. 确认 `ai setup` 生成的新配置包含新字段

---

## 6. 风险与注意事项

1. **`>` 的 contains 匹配过于宽泛**：设计文档有意为之（宁可多拦不漏过）。如果误报率太高，后续可考虑更精确的匹配（如 `\b>\b` 或限制为 `> /` 前缀）。当前按设计实施。

2. **`systemctl status` 问题**：当前 `default.yaml` 中 `readonly_commands` 包含 `systemctl status`，但规则引擎使用基础命令名匹配（`ExtractBaseName` 返回 `systemctl`）。应改为 `systemctl`。用户旧配置中的 `systemctl status` 仍能匹配（`isInList` 精确匹配），但不会匹配 `systemctl start` 等。这是已知的向后兼容缺口，在计划中已说明。

3. **`Confirm` 函数的 `cfg` 参数**：删除 `ClassifyCommand` 后，`Confirm` 的 `cfg` 参数在函数体中未使用。保持签名不变（避免改动 `main.go` 调用），用 `_ = cfg` 消除编译警告。这是最小改动原则。

4. **`fmt` import 在 `engine.go`**：`ForbiddenReason` 使用 `fmt.Sprintf`，需确保 import。
