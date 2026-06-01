# `ai` MVP 原型 - 架构设计

> **Opus 架构决策文档**
> **版本**: 0.3
> **目标**: `ai xxx` 最小可用原型 —— AI 生成 shell 命令并执行

---

## 1. 核心设计决策

### 1.1 零前缀流式响应格式

**设计目标**: LLM 响应的第一个 token 就是命令内容，无任何前缀（无 markdown 围栏、无解释引导语）。终端用户看到命令逐字出现，感知零延迟。

**响应格式规范**:

```
<命令内容，可多行>
#@ <分类>
<简短说明，单行，可选>
```

**格式约束**:

| 约束 | 说明 |
|------|------|
| 第一个 token 必须是命令内容 | 无 markdown、无引导语、无换行 |
| 命令与分类之间用 `\n#@ ` 分隔 | 恰好 4 字节: `0x0a 0x23 0x40 0x20` |
| 分类行以 `\n` 结尾 | 之后的内容为说明 |
| 命令本身不得包含 `#@` 序列 | 在 system prompt 中约束 LLM |
| `#@` 在 shell 中是注释 | 即使整段响应被意外粘贴执行也无害 |

**分类标记定义**:

| 标记 | 语义 | 程序行为 |
|------|------|---------|
| `#@ ro` | 只读命令 | 直接执行，无需确认 |
| `#@ rw` | 修改文件/系统状态 | `[a/y/N]` 确认 |
| `#@ rm` | 删除操作 | `[a/y/N]` 确认 |
| `#@ sudo,ro` | 需 root 权限的只读操作 | sudo + 直接执行 |
| `#@ sudo,rw` | 需 root 权限的修改操作 | sudo + `[a/y/N]` 确认 |
| `#@ sudo,rm` | 需 root 权限的删除操作 | sudo + `[a/y/N]` 确认 |

**设计对比**:

| 维度 | 零前缀 + #@ (本方案) | JSON 包裹 | Markdown 代码块 |
|------|---------------------|-----------|----------------|
| 首 token 延迟 | 0 (直接出命令) | ~10 token (JSON key) | ~5 token (```) |
| 解析复杂度 | 状态机, ~120 行 | json.Unmarshal, 5 行 | 正则提取, ~30 行 |
| Token 开销 | 约 8 token (#@ + 分类 + 说明) | 约 50+ token (JSON 结构) | 约 20 token (``` + 说明) |
| 安全性 | #@ 是 shell 注释, 无害 | JSON 有解析失败风险 | ``` 可能被部分执行 |
| 人类可读 | 高 (纯文本) | 低 | 中 |

**示例响应**:

```
lsof -i :8085
#@ ro
列出占用8085端口的进程
```

```
find . -name "*.tmp" -delete
#@ rm
删除所有 .tmp 文件
```

```
systemctl restart nginx
#@ sudo,rw
重启 nginx 服务
```

```
sudo cat /etc/nginx/nginx.conf
#@ sudo,ro
查看 nginx 配置文件
```

**异常响应处理**:

| 异常情况 | 处理策略 |
|---------|---------|
| 响应无 `#@` 标记 | 默认分类为 `rw`，触发确认 |
| 响应为空 | 错误退出，提示 LLM 返回异常 |
| `#@` 出现在命令内容中 | 状态机会在第一个 `\n#@ ` 处分割（符合预期） |
| 分类值非法 | 默认 `rw`，触发确认 |
| 多行命令中嵌套 `#@` | 仅以 `\n#@ ` (换行起始) 为分隔，命令内 `#@` 不受影响 |

---

### 1.2 流式解析状态机

**设计原则**: 单遍扫描，逐 chunk 输入，实时输出命令文本到终端，同时累积解析元数据。

**状态定义**:

```
                    ┌──────────────┐
                    │   COMMAND    │  累积命令文本，同时输出到终端
                    └──────┬───────┘
                           │ 检测到 "\n#@ "
                           ▼
                    ┌──────────────┐
                    │   METADATA   │  累积分类关键字
                    └──────┬───────┘
                           │ 检测到 "\n"
                           ▼
                    ┌──────────────┐
                    │ EXPLANATION  │  累积说明文本（可选）
                    └──────┬───────┘
                           │ 流结束 (Finish)
                           ▼
                    ┌──────────────┐
                    │     DONE     │  解析完成，Result 可用
                    └──────────────┘
```

**状态转换详细说明**:

| 当前状态 | 触发条件 | 动作 | 下一状态 |
|---------|---------|------|---------|
| COMMAND | chunk 累积, 检查是否含 `\n#@ ` | 分隔符之前的部分: 写入 command builder + 输出到终端; 分隔符之后的部分: 保留到 buf | METADATA |
| COMMAND | chunk 末尾可能匹配分隔符前缀 | 安全部分输出; 前缀候选保留在 buf | COMMAND (等待更多数据) |
| METADATA | 检测到 `\n` | 分类文本写入 metadata builder; 剩余转入 explanation | EXPLANATION |
| METADATA | 无换行 | 全部写入 metadata builder | METADATA (等待更多数据) |
| EXPLANATION | 流结束 | 剩余全部写入 explanation builder | DONE |

**分隔符前缀匹配 (COMMAND 状态的临界问题)**:

`\n#@ ` 是 4 字节分隔符。chunk 可能在任意位置断开，需要检测尾部是否匹配分隔符的部分前缀:

| 尾部后缀 | 是否为前缀候补 |
|---------|---------------|
| `\n` | 是 (分隔符前缀第 1 字节) |
| `\n#` | 是 (分隔符前缀第 1-2 字节) |
| `\n#@` | 是 (分隔符前缀第 1-3 字节) |

缓冲策略: COMMAND 状态中，从末尾向前扫描最多 3 字节，检查是否匹配 `\n#@ ` 的前缀。若匹配，则只输出安全部分（不含前缀候选），将候选留在 buf 中等待下一 chunk。

**伪代码** (chunk 处理核心逻辑):

```
safe := len(buf)
for k := 1; k <= 3 && k <= len(buf); k++ {
    if strings.HasSuffix(buf, "\n#@ "[:k]) {
        safe = len(buf) - k
        break
    }
}
// 输出 buf[:safe] 并写入 command builder
// buf = buf[safe:] 保留候选
```

**Finish 尾处理**:

流结束后调用 Finish():
- 若仍在 COMMAND 状态: 将残余 buf 写入 command builder
- 若仍在 METADATA 状态: 将残余 buf 写入 metadata builder
- 构建 Result{Command, Category, Explanation}

**Result 结构体**:

```go
type Result struct {
    Command     string    // 去除首尾空白
    Category    Category  // "ro" | "rw" | "rm" | "sudo,ro" | "sudo,rw" | "sudo,rm" | ""
    Explanation string    // 去除首尾空白
}
```

---

### 1.3 确认交互设计

**交互格式**:

```
find . -name "*.tmp" -delete
[a/y/N] █
```

**按键语义**:

| 按键 | 行为 | 说明 |
|------|------|------|
| `y` / `Y` | 执行本次 | 仅本次，不记忆 |
| `a` / `A` | 始终允许 | 将当前命令模式加入白名单，写入 config，然后执行 |
| 其他任意键 | 拒绝 | 包括 Enter、N、Ctrl+C 等 (默认拒绝) |

**终端模式切换**:

1. 输出命令后，确认前: 切换到 raw mode (`golang.org/x/term.MakeRaw`)
2. 读取单字节: `os.Stdin.Read(buf[0:1])`
3. 恢复到 canonical mode: `term.Restore(fd, oldState)`

**关键安全措施**:

| 措施 | 实现 |
|------|------|
| 信号处理 | 捕获 SIGINT/SIGTERM，恢复终端模式后再退出 |
| panic 恢复 | defer 中恢复终端状态 |
| fd 获取 | `int(os.Stdin.Fd())` —— 确保 stdin 是终端 |
| 非终端 stdin | 若 stdin 不是终端 (管道输入)，跳过确认，直接拒绝 |

**白名单持久化**:

用户按 `a` 时，将当前命令的基础命令名 (如 `find`) 追加到 config 的 `whitelist` 列表中，立即保存到 `~/.aicli/config.yaml`。

白名单匹配规则: 精确匹配命令基础名 (第一个空格前的词)，不包含参数。

---

### 1.4 配置管理设计

#### 1.4.1 文件路径

- **配置目录**: `~/.aicli/`
- **配置文件**: `~/.aicli/config.yaml`
- **目录权限**: `0700` (仅用户可访问)
- **文件权限**: `0600` (仅用户可读写，防止 api_key 泄露)

#### 1.4.2 配置结构体

```go
type Config struct {
    // --- 必填 ---
    APIKey  string `yaml:"api_key"`   // LLM API 密钥
    BaseURL string `yaml:"base_url"`  // API 基础 URL
    Model   string `yaml:"model"`     // 模型名称

    // --- 可选 (有默认值) ---
    Mode     string   `yaml:"mode"`      // "ai" | "rules" | "permissive" (默认 "ai")
    Debug    bool     `yaml:"debug"`     // 调试模式 (默认 false)
    Whitelist []string `yaml:"whitelist"` // 命令白名单 (默认空)

    // --- mode=rules 时使用 ---
    ReadonlyCommands []string `yaml:"readonly_commands"` // 只读命令列表
}
```

#### 1.4.3 默认值规范

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `api_key` | `""` | 必填，无默认值 |
| `base_url` | `"https://api.openai.com"` | OpenAI 兼容 API |
| `model` | `"gpt-4o"` | 默认模型 |
| `mode` | `"ai"` | LLM 返回 #@ 分类 |
| `debug` | `false` | 默认不输出调试信息 |
| `whitelist` | `[]` | 默认空白名单 |
| `readonly_commands` | 见 1.4.4 | mode=rules 时使用 |

#### 1.4.4 只读命令列表 (mode=rules 默认值)

```
ls, cat, head, tail, grep, find (without -delete), ps, top, free, df,
du, lsof, ss, netstat, ip, id, whoami, pwd, echo, date, uname, hostname,
wc, sort, uniq, diff, file, stat, which, whereis, env, printenv,
systemctl status, journalctl, dmesg
```

#### 1.4.5 配置加载流程

```
Load()
  ├── Path() → ~/.aicli/config.yaml
  ├── 文件存在?
  │   ├── 是 → 读取 → YAML 反序列化 → fillDefaults() → 返回
  │   └── 否 → loadDefault() (从 embed 的 default.yaml) → 返回
  └── fillDefaults(): 空字段填充默认值
```

#### 1.4.6 验证规则

```go
func Validate(cfg *Config) error {
    // 1. api_key 不能为空
    // 2. base_url 不能为空
    // 3. model 不能为空
    // 4. mode 必须是 "ai" / "rules" / "permissive" 之一
}
```

#### 1.4.7 Setup 流程 (visudo 风格)

```
ai setup
  ├── 1. 确保 ~/.aicli/ 目录存在 (0700)
  ├── 2. 若 config.yaml 不存在，先写入默认配置
  ├── 3. 打开 $EDITOR (默认 vi) 编辑 config.yaml
  ├── 4. 编辑器退出后，加载配置并验证
  │   ├── 验证通过 → "配置已保存 ✓" → 退出 0
  │   └── 验证失败 → 显示错误信息 → 提示选项:
  │       [e] 重新编辑 / [x] 放弃修改 / [f] 强制保存
  │       ├── e → 回到步骤 3
  │       ├── x → 恢复旧配置 (或删除)，退出 4
  │       └── f → 保存 (跳过验证)，退出 0
  └── 5. 首次运行自动触发: main() 中检测 !Exists() → 自动进入 setup
```

**编辑器选择**:

优先级: `$EDITOR` > `$VISUAL` > `vi`

---

### 1.5 Shell History 集成方案

**核心目标**: 用户在 bash 中按 `↑` 键看到的是实际执行的命令 (如 `lsof -i :8085`)，而不是 `ai 查看端口`。

**MVP 方案: shell wrapper 函数 + 临时文件**

**Go 二进制** (`ai`):
- 正常执行流程
- 确定要执行的 command 后，写入临时文件: `/tmp/ai-cmd-{pid}.txt`
- 然后执行命令
- 退出

**Shell wrapper 函数** (用户需添加到 `.bashrc` / `.zshrc`):
```bash
ai() {
    local tmpfile="/tmp/ai-cmd-$$.txt"
    command ai "$@"
    local rc=$?
    if [ -f "$tmpfile" ]; then
        local cmd
        cmd=$(cat "$tmpfile")
        rm -f "$tmpfile"
        if [ -n "$cmd" ]; then
            history -s "$cmd"
        fi
    fi
    return $rc
}
```

**替代方案 (未来考虑)**:
- 直接操作 `~/.bash_history` 文件追加 —— 脆弱，可能与其他 shell 会话冲突
- 通过 `fc -s` 替换历史 —— 需要更复杂的 shell 集成

**不写历史的情况**:
- 用户拒绝执行确认
- LLM 调用失败
- `--no-history` 标志 (P1 功能，MVP 暂不实现)

---

### 1.6 调试模式

**触发方式**: `ai -d <args>` 或 `ai --debug <args>`

**全局 flag 位置**: 必须是第一个参数 (在子命令和查询内容之前)

**调试输出内容**:

| 信息 | 输出位置 | 前缀 |
|------|---------|------|
| 配置加载路径 | stderr | `[DEBUG]` |
| 完整 API 请求 (URL + body) | stderr | `[DEBUG]` |
| 完整 API 响应 | stderr | `[DEBUG]` |
| 命令分类决策 | stderr | `[DEBUG]` |
| 执行耗时 | stderr | `[DEBUG]` |
| SSE 解析错误 (若有) | stderr | `[DEBUG]` |

**格式约定**: 所有调试输出以 `[DEBUG]` 开头，输出到 stderr，不干扰 stdout 的命令输出。

---

## 2. 技术选型

| 组件 | 选择 | 理由 |
|------|------|------|
| CLI 参数解析 | `os.Args` 手动解析 | MVP 仅需解析 `-d`/`--debug` 标志，不需要 cobra 框架的复杂度 |
| HTTP 客户端 | `net/http` | 标准库，成熟的 SSE 流式读取 |
| YAML 解析 | `gopkg.in/yaml.v3` | 简单 API，够用 |
| 终端 raw mode | `golang.org/x/term` | 单键读取的标准方案 |
| 嵌入资源 | `embed` | Go 1.16+ 标准库，嵌入默认配置 |
| 流式解析 | 自定义状态机 | 约 120 行，专门适配 `#@` 格式 |

**不引入的依赖**: cobra, viper, zap, 任何数据库驱动 —— MVP 不需要。

---

## 3. 文件结构与模块职责

```
aicli/
├── cmd/
│   └── ai/
│       └── main.go              # 入口 + 参数解析 + 流程编排
├── internal/
│   ├── config/
│   │   ├── config.go            # Config 结构体 + Load/Save/Validate/Setup
│   │   └── default.yaml         # 默认配置 (go:embed)
│   ├── llm/
│   │   └── client.go            # OpenAI 兼容客户端 + SSE 流式解析
│   ├── executor/
│   │   ├── executor.go          # 命令执行 + 确认交互
│   │   └── parser.go            # 流式状态机解析器 (#@ 格式)
│   └── prompt/
│       └── prompt.go            # 系统 prompt 常量
├── docs/
│   ├── plan.md                  # 完整规划文档
│   └── architecture-mvp.md      # 本文档
├── go.mod
├── go.sum
├── Makefile
├── .gitignore
└── CLAUDE.md
```

### 3.1 模块职责

#### `cmd/ai/main.go` — 入口 + 流程编排

- 解析 `os.Args`，识别 `-d`/`--debug` 全局标志
- 组装用户输入 (flag 之后的所有参数拼接为查询字符串)
- 编排完整流程:
  1. 检测配置是否存在 → 不存在则自动 setup
  2. 加载配置 → 验证
  3. 构造 prompt → 调用 LLM 流式接口
  4. 状态机解析流式响应
  5. 命令分类判断 → 确认交互 (如需)
  6. 执行命令 → 输出结果
  7. 写入命令到临时文件 (供 shell wrapper 读取)
  8. 退出
- 不包含: 任何 LLM 协议细节、解析逻辑、配置细节

#### `internal/config/` — 配置管理

- `config.go`: Config 结构体、Load/Save/Validate/Dir/Path/Exists/Setup
- `default.yaml`: 嵌入的默认配置
- 职责边界: 只管配置的读写验证，不管如何使用配置

#### `internal/llm/` — LLM 客户端

- `client.go`: OpenAI 兼容 API 的流式 Chat 客户端
  - `StreamChat(userInput, callback)` → `(*StreamResult, error)`
  - 处理 SSE 协议 (data: 行解析, [DONE] 检测)
  - Debug 模式下输出请求/响应详情
- 职责边界: 只管 API 交互和流式回调，不做格式解析

#### `internal/executor/` — 命令执行

- `parser.go`: 流式状态机，解析零前缀 `#@` 格式
  - `Feed(chunk)` → 返回本次新增的命令文本
  - `Finish()` → 返回完整 `Result{Command, Category, Explanation}`
- `executor.go`: 命令执行 + 确认交互
  - `Execute(command)` → `(stdout, exitCode, error)`
  - `Confirm(category)` → `(action, error)`
  - 终端 raw mode 管理
- 职责边界: 命令的解析和本地执行，不涉及 LLM

#### `internal/prompt/` — 系统 Prompt

- `prompt.go`: 系统 prompt 常量 (纯文本)
- 职责边界: 提供 prompt 文本，不参与运行时逻辑

### 3.2 依赖关系

```
main.go
  ├── config  (加载/验证/保存)
  ├── llm     (调用 API)
  │     └── prompt  (系统 prompt)
  └── executor
        ├── parser   (流式解析)
        └── (执行 + 确认)
```

方向: `main → config`, `main → llm → prompt`, `main → executor → parser`

---

## 4. LLM API 调用规范

### 4.1 请求规范

```
POST {base_url}/v1/chat/completions
Authorization: Bearer {api_key}
Content-Type: application/json
```

**请求体**:

```json
{
  "model": "{model}",
  "messages": [
    {"role": "system", "content": "{system_prompt}"},
    {"role": "user",   "content": "{user_input}"}
  ],
  "stream": true,
  "temperature": 0.1,
  "max_tokens": 1024
}
```

**参数说明**:

| 参数 | 值 | 理由 |
|------|-----|------|
| `stream` | `true` | 必须流式，实现零前缀感知 |
| `temperature` | `0.1` | 命令需要确定性输出，接近 0 但避免完全确定性导致的退化 |
| `max_tokens` | `1024` | 命令 + 分类 + 说明足够，长命令可截断 |

**超时设置**: HTTP Client 设置 60s 总超时。流式请求没有单 chunk 超时限制（SSE 场景下 chunk 间隔可能较大）。

### 4.2 系统 Prompt 设计

**设计原则**:

1. **零前缀约束**: 明确禁止 markdown 围栏、引导语、解释前缀
2. **格式约束**: 精确描述 `<command>\n#@ <category>\n<explanation>` 格式
3. **分类教育**: 用示例教学各分类的含义
4. **负向约束**: 明确禁止的行为（代码块、多余文字）

**Prompt 内容** (见 `internal/prompt/prompt.go`):

```
你是一个 Linux 命令行助手。用户用自然语言描述需求，你生成对应的 shell 命令。

严格按以下格式返回，不要有任何多余文字、代码块标记或解释:

<命令>
#@ <分类>
<简短说明>

分类必须是以下之一:
- ro: 纯查看命令 (ls, cat, grep, ps, free, df, lsof 等不修改系统的命令)
- rw: 修改文件/系统状态 (rm, mv, chmod, git commit, apt install 等)
- rm: 删除操作 (rm, find -delete 等删除文件/目录的命令)
- sudo,ro: 需要 root 权限的只读操作
- sudo,rw: 需要 root 权限的修改操作
- sudo,rm: 需要 root 权限的删除操作

规则:
1. 第一个字符必须是命令内容，不要有任何前缀
2. 命令中不要使用 #@ 序列
3. 命令应简洁有效，优先使用系统已安装的工具
4. 说明控制在 15 字以内

示例:

用户: 列出占用端口8085的程序
lsof -i :8085
#@ ro
列出占用8085端口的进程

用户: 删除所有tmp文件
find . -name "*.tmp" -delete
#@ rm
删除所有 .tmp 文件

用户: 查看nginx配置
sudo cat /etc/nginx/nginx.conf
#@ sudo,ro
查看 nginx 配置文件
```

**Few-shot 示例设计考量**:

- 3 个示例覆盖 3 种主要分类: ro (直接执行)、rm (确认)、sudo,ro (sudo)
- 示例保持简短，符合 "15 字说明" 规则
- 不使用代码块包裹

### 4.3 SSE 流式解析

**SSE 格式**:

```
data: {"choices":[{"delta":{"content":"lsof"}}]}
data: {"choices":[{"delta":{"content":" -i"}}]}
data: {"choices":[{"delta":{"content":" :8085\n#@ ro\n列出占用8085端口的进程"}}]}
data: [DONE]
```

**解析策略**:

1. 逐行读取 SSE 响应 (`bufio.Scanner`)
2. 跳过非 `data: ` 前缀的行
3. `data: [DONE]` → 流结束
4. 提取 `choices[0].delta.content`
5. 每个 content chunk 喂给 parser.Feed()
6. Feed() 返回的新增命令文本发送给 callback（用于终端流式显示）

---

## 5. 完整交互流程

### 5.1 首次运行流程

```
$ ai 查看内存

[未检测到配置文件 ~/.aicli/config.yaml]
[自动启动 setup...]

打开编辑器: vi ~/.aicli/config.yaml

用户编辑:
  api_key: sk-xxx
  base_url: https://api.openai.com
  model: gpt-4o

保存退出 → 验证中...
✓ 配置验证通过
[继续执行原始命令: "查看内存"...]

free -h
               total        used        free      shared  buff/cache   available
Mem:            15Gi       3.2Gi       8.1Gi       256Mi       4.2Gi        11Gi
Swap:          2.0Gi          0B       2.0Gi
```

### 5.2 只读命令流程 (ro)

```
$ ai 列出占用端口8085的程序

lsof -i :8085                              ← 流式输出，逐字出现
COMMAND   PID   USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
node    12345   ling   23u  IPv4 123456      0t0  TCP *:8085 (LISTEN)
```

分类为 `ro` → 命令显示完毕后立即执行，无需等待确认。

### 5.3 修改命令流程 (rw/rm)

```
$ ai 删除所有 .tmp 文件

find . -name "*.tmp" -delete               ← 流式输出
[a/y/N] y                                  ← 终端进入 raw mode，按 y 即响应
[执行...]
已删除 5 个 .tmp 文件
```

分类为 `rm` → 命令显示完毕后输出确认提示，进入 raw mode 等待单键输入。

### 5.4 sudo 命令流程

```
$ ai 查看nginx配置

sudo cat /etc/nginx/nginx.conf             ← 流式输出
[sudo] 用户的密码：
[sudo] password for ling:                  ← sudo 自身的密码提示
[配置内容...]
```

分类为 `sudo,ro` → 直接执行 (ro)，但 sudo 自己会触发密码输入。

### 5.5 拒绝流程

```
$ ai 删除所有 .tmp 文件

find . -name "*.tmp" -delete
[a/y/N]                                    ← 用户按 Enter 或其他键
✗ 已取消
```

退出码: 2 (用户拒绝)

### 5.6 始终允许流程

```
$ ai 删除所有 .tmp 文件

find . -name "*.tmp" -delete
[a/y/N] a                                  ← 用户按 a
✓ 已将 find 加入白名单
[执行...]
已删除 5 个 .tmp 文件
```

后续 `find ... -delete` 类命令将直接执行，不再确认。

---

## 6. 退出码约定

| 退出码 | 含义 | 触发场景 |
|--------|------|---------|
| 0 | 成功 | 命令执行成功 (退出码 0) |
| 1 | 一般错误 | 命令执行失败 (非 0 退出码)、参数解析错误、流式解析异常 |
| 2 | 用户拒绝 | 确认交互中用户按非 y/a 键 |
| 3 | LLM API 错误 | HTTP 非 200、网络超时、SSE 解析错误 |
| 4 | 配置错误 | 配置验证失败、api_key 为空 |

**命令执行退出码传递**: 当 LLM 生成的命令执行失败 (退出码非 0) 时，`ai` 自身也以该退出码退出 (或统一退为 1)。MVP 选择统一退为 1，保持简单。

---

## 7. 错误处理矩阵

| 错误场景 | 检测 | 处理 | 退出码 |
|---------|------|------|--------|
| 无配置文件 | `config.Exists() == false` | 自动进入 setup | 取决于 setup 结果 |
| api_key 为空 | `config.Validate()` | 提示用户运行 `ai setup` | 4 |
| api_key 无效 | HTTP 401 | 提示 "API key 无效，请运行 ai setup" | 3 |
| 网络超时 | HTTP timeout | 提示 "API 请求超时，请检查网络" | 3 |
| LLM 返回空 | `StreamResult.FullContent == ""` | 提示 "AI 未生成命令" | 3 |
| 响应无 `#@` | parser.Result.Category == "" | 默认分类 `rw`，触发确认 | 无需 (继续流程) |
| 非法分类值 | 不在已知分类中 | 默认 `rw`，触发确认 | 无需 (继续流程) |
| 命令执行超时 | context timeout (30s) | 提示 "命令执行超时" | 1 |
| 命令执行返回非0 | exitCode != 0 | 输出 stderr，提示失败 | 1 |
| stdin 非终端 | `term.IsTerminal(fd)` 检查 | 确认交互跳过 (直接拒绝) | 2 |
| 信号中断 | SIGINT/SIGTERM | 恢复终端模式，清理临时文件 | 130/143 |

---

## 8. 命令执行安全策略

### 8.1 分类-行为映射

```
Category     → Action
───────────────────────
ro           → 直接执行
rw           → [a/y/N] 确认
rm           → [a/y/N] 确认
sudo,ro      → 直接执行 (sudo 自身要求密码)
sudo,rw      → [a/y/N] 确认 → sudo 执行
sudo,rm      → [a/y/N] 确认 → sudo 执行
"" (unkown)  → [a/y/N] 确认 (安全默认)
```

### 8.2 模式切换

| mode 值 | 行为 |
|---------|------|
| `ai` (默认) | 使用 LLM 返回的 `#@` 分类标记决定行为 |
| `rules` | 忽略 `#@`，使用本地 `readonly_commands` 列表匹配决定 |
| `permissive` | 全部直接执行，无确认 |

### 8.3 执行约束

| 约束 | 值 | 说明 |
|------|-----|------|
| 命令执行超时 | 30s | context.WithTimeout |
| 输出大小限制 | 10KB (stdout+stderr) | 防止海量输出刷屏 |
| 工作目录 | 继承当前 shell 的 PWD | 不改变目录 |

---

## 9. MVP 不包含

以下功能明确排除在 MVP 之外，将在后续版本迭代中实现:

| 功能 | 归属阶段 | 说明 |
|------|---------|------|
| `ai chat` 持续对话模式 | P1 | 单次命令执行，无对话循环 |
| 会话持久化 (SQLite) | P1 | 无历史消息存储 |
| 上下文窗口 (多轮记忆) | P1 | 每次调用独立，无上下文传递 |
| Prompt 前缀缓存 | P1 | 无缓存优化 |
| 凭证管理系统 | P2 | 无占位符机制 |
| 记忆系统 (RAG) | P2-P3 | 无向量检索 |
| Explore / Agent 子模式 | P2 | 单模式 |
| Shell wrapper 安装命令 (`ai shell install`) | P1 | 使用者手动配置 |
| 多 LLM 后端 | P1 | 仅 OpenAI 兼容 API |
| 审计日志 | P3 | 无持久化记录 |
| macOS 适配 | P4 | 仅 Linux |
| 子命令系统 (chat/explore/agent/memory...) | P1-P2 | MVP 仅默认智能命令模式 |
| 未 `-v` 详细模式 | P1 | MVP 仅 `-d` debug 模式 |
| `--dry-run` / `--no-exec` | P1 | MVP 始终执行 (确认后) |
| 多 profile | P4 | 单配置 |

---

## 10. 构建与分发

### 10.1 构建

```bash
go build -o ai ./cmd/ai/
```

### 10.2 安装

```bash
# 1. 编译
make build   # → ./ai

# 2. 安装二进制到 PATH
cp ai /usr/local/bin/ai

# 3. 配置 shell wrapper (~/.bashrc 或 ~/.zshrc)
ai() {
    local tmpfile="/tmp/ai-cmd-$$.txt"
    command ai "$@"
    local rc=$?
    if [ -f "$tmpfile" ]; then
        local cmd
        cmd=$(cat "$tmpfile")
        rm -f "$tmpfile"
        if [ -n "$cmd" ]; then
            history -s "$cmd"
        fi
    fi
    return $rc
}
```

### 10.3 Makefile 目标 (建议)

```makefile
.PHONY: build run test clean

build:
	go build -ldflags="-s -w" -o ai ./cmd/ai/

run: build
	./ai $(ARGS)

test:
	go test ./...

clean:
	rm -f ai
```

---

*本文档由 Opus 架构层输出，是对 plan.md 中 MVP 阶段 (P0) 的架构细化。*
*实施由 Sonnet 规划、Haiku 编码完成。*
