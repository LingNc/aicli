# `ai` - 终端智能助手项目规划文档

> **版本**: 1.0
> **最后更新**: 2026-05-31
> **语言**: Go
> **目标平台**: Linux (主要), macOS (可扩展)
> **核心定位**: 命令为主 + 轻量开发 -- 不是编程 IDE 工具，是终端命令辅助工具

---

## 1. 项目概述与愿景

`ai` 是一个终端智能助手，用户输入 `ai xxx`，AI 帮助生成/执行命令，替代记忆和手动输入。目标是将 AI 封装成一条原生命令，快速、简洁、无摩擦。

**核心理念**:
- 默认不啰嗦，给出最可能的命令或答案
- 纯查看类命令直接执行，无需确认
- 命令写入 shell 历史，按上键看到的是实际命令而非 `ai xxx`
- AI 渐进了解用户和机器，随使用越来越智能

---

## 2. 核心功能清单（按优先级排序）

### P0 - 最小可用版本
| 功能 | 说明 |
|------|------|
| 隐式命令推断 | `ai 查看内存` -> 直接生成并执行 `free -h` |
| 纯查看命令直接执行 | ls, cat, grep, ps, lsof, free, df 等无需确认 |
| 修改类命令确认机制 | rm, mv, chmod 等需要首次确认 |
| 流式输出命令 | 命令逐字出现，用户可感知进度 |
| Shell 历史替换 | 实际命令写进 shell 历史，上键可见 |
| 短期对话上下文 | 当前会话保留最近 20 轮对话 |

### P1 - 核心功能
| 功能 | 说明 |
|------|------|
| `ai chat` 模式 | 持续对话，支持多轮追问 |
| sudo 命令检测 | 自动识别需要提权的命令，提示用户输入密码 |
| 命令白名单 | 用户可将修改类命令加入白名单，之后直接执行 |
| 中期记忆 | SQLite 存储压缩摘要，会话间可检索 |
| Shell wrapper 函数 | `.bashrc` / `.zshrc` 集成 |
| -v 详细模式 | 显示完整推理过程和工具调用链 |

### P2 - 进阶功能
| 功能 | 说明 |
|------|------|
| `ai explore` 子智能体 | 只读分析模式，独立上下文 |
| `ai agent` 模式 | 可修改文件的独立 agent 环境 |
| 长期记忆 (RAG) | 向量检索，语义搜索 |
| 会话分组与归档 | 按话题分组，长时间不用自动归档 |
| 分级检索 | 精确匹配 -> RAG 语义搜索 -> 模糊匹配 |
| 配置管理 | `~/.ai/config.yaml` 管理所有配置 |

### P3 - 增强功能
| 功能 | 说明 |
|------|------|
| `ai search` | 结合本地记忆与联网搜索 |
| `ai note` | 快速记录知识到长期记忆 |
| `ai memory` | 记忆管理：搜索、查看、导出、清理 |
| 项目感知 | 按工作目录自动加载项目相关记忆 |
| 审计日志 | 记录所有 AI 发起的命令及用户确认 |
| 渐进式披露 | AI 按需获取系统信息，不一性全量加载 |

### P4 - 可选增强
| 功能 | 说明 |
|------|------|
| macOS 适配 | 检测系统差异，适配命令 |
| 多 LLM 后端 | 支持 OpenAI / Ollama / 本地模型 |
| 多 profile | 不同项目使用不同配置 |
| 多模态 | 图片/PDF 感知 (按需) |

---

## 3. 详细架构设计

### 3.1 整体架构图

```
                                    用户 Shell
                                        │
                          ┌─────────────┼─────────────┐
                          │ ai xxx       │ ai chat     │ ...
                          ▼              ▼              ▼
                  ┌───────────────────────────────────────────┐
                  │         Shell Wrapper (bash/zsh)          │
                  │  - ai() 函数捕获原始输入                    │
                  │  - 调用 Go 二进制后，将结果命令写回 history │
                  └────────────────────┬──────────────────────┘
                                       │
                                       ▼
                  ┌───────────────────────────────────────────┐
                  │             CLI Entry (cobra)              │
                  │  - 解析子命令与标志                         │
                  │  - 路由到对应 handler                       │
                  └────────────────────┬──────────────────────┘
                                       │
                                       ▼
                  ┌───────────────────────────────────────────┐
                  │            Orchestrator (调度器)           │
                  │                                           │
                  │  ┌─────────────┐  ┌───────────────────┐   │
                  │  │ 意图识别器   │  │ 安全策略引擎       │   │
                  │  │ (快速模式)   │  │ - 命令分类         │   │
                  │  │ 判断: 命令   │  │ - 白名单检查       │   │
                  │  │ 生成/对话/   │  │ - 危险操作拦截     │   │
                  │  │ explore/agent│  │ - sudo 检测        │   │
                  │  └─────────────┘  └───────────────────┘   │
                  │                                           │
                  │  ┌─────────────┐  ┌───────────────────┐   │
                  │  │ 会话管理器   │  │ 上下文组装器       │   │
                  │  │ - 多会话切换 │  │ - Prompt 组装      │   │
                  │  │ - 窗口管理   │  │ - Token 预算控制   │   │
                  │  │ - 分组/归档  │  │ - 记忆注入         │   │
                  │  └─────────────┘  └───────────────────┘   │
                  └──────┬───────────────┬────────────────────┘
                         │               │
          ┌──────────────┼───────────────┼──────────────┐
          ▼              ▼               ▼              ▼
   ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐
   │ 主 Agent  │  │ explore   │  │  agent    │  │ Memory    │
   │ (命令+聊天)│  │ Agent     │  │ Agent     │  │ Agent     │
   │           │  │ (只读)    │  │ (读写)    │  │ (检索)    │
   └─────┬─────┘  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
         └───────────────┴──────────────┴───────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
      ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
      │ Memory System│ │  Tool Layer  │ │  LLM Backend │
      │              │ │              │ │              │
      │ 短期: 内存   │ │ shell exec   │ │ OpenAI       │
      │ 中期: SQLite │ │ file read    │ │ Ollama       │
      │ 长期: Vector │ │ dir2txt      │ │ 自定义适配器 │
      │   + RAG      │ │ web search   │ │              │
      └──────────────┘ └──────────────┘ └──────────────┘
              │
      ┌───────┴───────┐
      ▼               ▼
┌───────────┐   ┌───────────┐
│  SQLite   │   │ Vector DB │
│ ~/.ai/mem │   │ ~/.ai/vec │
└───────────┘   └───────────┘
```

### 3.2 各模块职责与边界

#### CLI Entry (cobra + viper)
- **职责**: 解析命令行参数，路由到对应的 handler
- **边界**: 只做解析和路由，不做业务逻辑
- **输入**: `os.Args`
- **输出**: handler 调用

#### Orchestrator (调度器) - 核心
- **意图识别器**: 
  - 使用快速模式（非思考模式）判断用户输入属于哪种意图
  - 分类: 命令生成 / 纯对话 / explore / agent / 记忆查询
  - 命令生成路径下，再细分: 纯查看 / 修改类 / 需要sudo
- **安全策略引擎**:
  - 命令分类: 查看类 / 修改类 / 危险类
  - 白名单管理: 用户可添加常用命令为可信
  - sudo 检测: 识别命令是否需要提权
  - 危险操作拦截: 禁止 `rm -rf /` 等毁灭性操作
- **会话管理器**:
  - 多会话切换 (主会话、explore 会话、agent 会话独立)
  - 会话窗口管理 (默认 20 轮，超出自动压缩)
  - 会话分组与归档
- **上下文组装器**:
  - Prompt 组装: 系统信息 + 记忆 + 当前上下文
  - Token 预算控制: 确保不超出模型限制
  - 按优先级注入记忆: 短摘要 > 语义相关 > 目录快照

#### Agent 层
- **主 Agent**: 处理 `ai xxx` 隐式命令生成、`ai chat` 对话
  - 可以: 建议命令、执行命令、回答问题
  - 工具有: shell_exec, file_read, memory_query
- **Explore Agent**: `ai explore xxx` 只读分析
  - 可以: 读文件、执行只读命令、分析项目结构
  - 不可以: 修改任何文件
  - 工具有: shell_exec(readonly), file_read, dir2txt, web_search
- **Agent (读写)**: `ai agent xxx` 独立执行环境
  - 可以: 修改配置、安装软件、编辑文件
  - 需要: 每次写操作需确认
  - 工具有: shell_exec(full), file_read, file_write, web_search
- **Memory Agent**: 记忆查询与管理
  - 服务于其他 Agent 和用户的记忆检索请求

#### Memory System (记忆系统)
- 详见第 6 节

#### Tool Layer (工具层)
- shell_exec: 执行 shell 命令，返回 stdout/stderr 和退出码
- file_read: 读取文件内容 (限制大小)
- file_write: 写入文件 (仅 agent 模式)
- dir2txt: 调用外部工具扁平化目录结构
- web_search: 联网搜索 (通过 Tavily / Bing API)
- system_info: 获取系统信息 (OS、Shell、当前目录等)

#### LLM Backend (LLM 后端)
- 可插拔设计
- 支持 OpenAI API (gpt-4o, gpt-4o-mini)
- 支持 Ollama (本地模型)
- 支持 Anthropic API (Claude)
- 接口统一: `Chat(messages, tools) -> response`

### 3.3 数据流

**典型流程 `ai 列出占用端口为8085的程序`:**

```
1. Shell Wrapper 捕获 "ai 列出占用端口为8085的程序"
   │
2. CLI Entry 解析: 无子命令 -> 路由到主 Agent (隐式命令模式)
   │
3. Orchestrator 接收请求:
   ├── 意图识别器: 判断为"命令生成" (快速判断, ~200ms)
   ├── 上下文组装器:
   │   ├── 注入系统信息: Linux, bash, uid=1000, cwd=/home/user/project
   │   ├── 查询记忆: 无相关记忆
   │   └── 组装 Prompt
   │
4. 主 Agent 调用 LLM:
   │  System: "你是终端助手。用户需要命令建议。只返回命令和简短解释。"
   │  User: "列出占用端口为8085的程序"
   │  LLM 返回: "lsof -i :8085"
   │
5. 安全策略引擎检查:
   ├── 分类: 纯查看命令 ✓
   ├── 白名单: N/A
   └── 结果: 允许直接执行
   │
6. 流式输出到终端: "lsof -i :8085" (逐字出现)
   │
7. 执行命令:
   ├── stdout: "COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME\n..."
   ├── stderr: (空)
   └── 退出码: 0
   │
8. 结果整理 (LLM 可选项):
   └── 输出精简摘要或原始输出
   │
9. Shell Wrapper 将 "lsof -i :8085" 写入 shell 历史
   │
10. 更新记忆:
    └── 异步: 记录"端口检查 -> lsof -i :PORT" 到中期记忆
```

---

## 4. 命令设计

### 4.1 完整命令结构

```
ai [global flags] [command] [args...]

当未指定子命令时，自动进入"智能命令模式"：
  ai 查看CPU型号       → 生成命令并执行
  ai 为什么磁盘满了？    → 生成命令并执行

Commands:
  chat       进入持续对话模式
  explore    启动只读探索子智能体
  agent      启动读写独立 agent
  search     搜索记忆或联网搜索
  note       记录知识到长期记忆
  memory     记忆管理（搜索/查看/导出/清理）
  config     管理配置
  shell      安装/更新 shell 集成
  session    会话管理（列表/切换/归档/分组）

Global Flags:
  -v, --verbose         显示详细推理过程
  -q, --quiet           静默模式，仅输出结果
  -m, --model string    临时切换模型
  -s, --session string  指定会话名称
  --no-exec             只建议命令，不执行
  --no-history          不写入 shell 历史
  --dry-run             显示将执行的命令但不执行

Aliases:
  ai s    → ai search
  ai n    → ai note
  ai e    → ai explore
  ai a    → ai agent
  ai c    → ai chat
```

### 4.2 各命令详细行为

#### 默认模式: 智能命令

```bash
# 纯查看命令 - 直接执行，无需确认
ai 列出占用端口8085的程序
→ lsof -i :8085 (流式显示)
→ [直接执行，输出结果]

# 修改类命令 - 首次需确认
ai 删除所有 .tmp 文件
→ find . -name "*.tmp" -delete
→ ⚠ 这是一个修改操作，是否执行？[y/N/a(始终允许)]
→ y: 执行     N: 取消     a: 加入白名单并执行

# sudo 命令 - 提示用户输入密码
ai 查看nginx配置
→ sudo cat /etc/nginx/nginx.conf
→ 🔒 需要管理员权限，请在终端输入密码:
→ [sudo 密码输入]
→ [输出结果]

# 可用标志
--no-exec      只显示命令，不执行
--dry-run      显示命令但不执行
```

#### ai chat - 持续对话

```bash
ai chat                     # 进入对话模式
ai chat 解释一下刚才那条命令  # 单次对话
ai chat -s my-session      # 指定会话名
ai chat --list             # 列出所有对话会话
ai chat --resume my-session# 恢复指定会话

对话模式交互:
You: 为什么我的nginx启动失败了？
AI: 让我查看一下... [执行 sudo systemctl status nginx]
    nginx 启动失败，错误: port 80 already in use
    端口 80 被进程 PID 1234 (apache2) 占用
    建议: systemctl stop apache2 && systemctl start nginx
You: 帮我停掉apache
AI: [执行 sudo systemctl stop apache2] ✓ 已停止
You: exit / quit / Ctrl+C
→ 退出对话模式，会话保留
```

#### ai explore - 只读探索

```bash
ai explore 这个项目为什么构建失败？
→ [启动只读子智能体]
→ Agent: 让我先看看项目结构...
  ├── [dir2txt 获取目录快照]
  ├── [cat Makefile]
  ├── [make 2>&1 捕获错误]
  └── 分析: 缺少 libssl-dev，请运行 sudo apt install libssl-dev

# explore 内可继续追问
  You: 我还缺少什么依赖？
  Agent: [检查头文件] 还需要 libz-dev, pkg-config
```

#### ai agent - 独立 agent (读写)

```bash
ai agent 帮我配置nginx反向代理到localhost:3000
→ [启动独立 agent，可修改文件]
→ Agent: 我将配置 /etc/nginx/sites-available/default
  ├── [sudo cat /etc/nginx/nginx.conf] 检查现有配置
  ├── [生成新配置]
  ├── ⚠ 即将修改 /etc/nginx/sites-available/default，是否继续？[y/N]
  ├── y: [写入文件]
  └── [sudo nginx -t] 测试通过

# agent 模式需要用户确认每一次写操作
# agent 有自己的独立上下文，不污染主会话
```

#### ai search - 搜索

```bash
ai search nginx 配置            # 搜索本地记忆
ai search --web "golang error"  # 联网搜索
ai search --all "数据库连接"     # 先本地，再联网

搜索流程:
1. 精确匹配关键词 → 本地记忆
2. 若无结果 → RAG 语义搜索
3. 若无结果 → 联网搜索 (需 --web 标志)
```

#### ai note - 记录知识

```bash
ai note "nginx 配置在 /etc/nginx/sites-enabled/default"
→ 记录到长期记忆:
  标签: nginx, 配置
  内容: nginx 配置路径 /etc/nginx/sites-enabled/default
  目录: /etc/nginx/
  时间: 2026-05-31 14:30:00
```

#### ai memory - 记忆管理

```bash
ai memory search "nginx"         # 搜索记忆
ai memory list                   # 列出所有记忆
ai memory list --session my-ses  # 按会话过滤
ai memory list --tag nginx       # 按标签过滤
ai memory show <id>              # 查看记忆详情
ai memory delete <id>            # 删除记忆
ai memory export --format json   # 导出记忆
ai memory clean --older-than 30d # 清理旧记忆
ai memory stats                  # 记忆统计信息
```

#### ai config - 配置管理

```bash
ai config show                   # 显示当前配置
ai config set model gpt-4o       # 设置模型
ai config set api-key sk-xxx     # 设置 API key
ai config set whitelist "git push, npm install"
ai config set max-history 20     # 设置历史窗口大小
ai config reset                  # 恢复默认配置
```

#### ai shell - Shell 集成

```bash
ai shell install                 # 安装 shell wrapper 到 .bashrc/.zshrc
ai shell uninstall               # 移除 shell 集成
ai shell status                  # 检查集成状态
```

#### ai session - 会话管理

```bash
ai session list                  # 列出所有会话
ai session list --active         # 列出活跃会话
ai session list --archived       # 列出已归档会话
ai session switch <name>         # 切换会话
ai session new <name>            # 新建命名会话
ai session archive <name>        # 归档会话
ai session group <names...> --tag <tag>  # 会话分组
ai session delete <name>         # 删除会话（含记忆）
ai session summary <name>        # 查看会话摘要
```

---

## 5. 上下文与会话管理详细设计

### 5.1 会话生命周期

```
创建 ──→ 活跃 ──→ 闲置(7天) ──→ 归档 ──→ 冷存储
  │         │                    │           │
  │         │ 恢复               │ 恢复      │ 恢复(慢)
  │         │◄───────────────────┘           │
  │         │                                │
  └─────────┴────────────────────────────────┘
```

**状态定义**:
- **活跃**: 当前或最近 1 小时内使用的会话
- **闲置**: 1 小时到 7 天未使用的会话，保留完整上下文
- **归档**: 7 天以上未使用，压缩为摘要，移入归档
- **冷存储**: 30 天以上，仅保留摘要和关键记忆，释放向量索引

### 5.2 会话数据结构

```go
type Session struct {
    ID          string    // UUID
    Name        string    // 用户命名或自动生成
    Type        string    // "chat" | "explore" | "agent"
    Description string    // AI 自动生成的会话描述
    Keywords    []string  // 自动提取的关键词
    Context     Context   // 会话上下文
    CreatedAt   time.Time
    LastActive  time.Time
    Status      string    // "active" | "idle" | "archived" | "cold"
    GroupID     string    // 会话分组 ID (可选)
    ParentID    string    // 父会话 ID (explore/agent 从主会话派生)
    MessageCount int
    TokenCount  int64
    Metadata    map[string]interface{} // 扩展元数据
}

type Context struct {
    Window      []Message          // 当前窗口消息 (最多 N 轮)
    Summary     string             // 窗口外消息的压缩摘要
    SystemPrompt string            // 该会话的系统提示
    CWD         string             // 工作目录
    OS          string             // 操作系统
    Shell       string             // 用户 shell
    CreatedAt   time.Time
}
```

### 5.3 压缩策略

**触发条件**: 上下文窗口消息数超过 `max_history` (默认 20 轮)

**压缩流程**:
1. 取窗口外最早的 10 轮消息
2. 调用快速/便宜模型 (如 gpt-4o-mini) 生成摘要
3. 摘要包含: 关键对话话题、已执行操作、发现的问题、未解决的 todo
4. 摘要前保留 2 轮前缀上下文 (前置锚定)
5. 将摘要注入窗口顶部 (作为 "Previous context summary")
6. 新消息继续追加到窗口底部

**压缩示例**:
```
[原始 30 轮对话]
轮1-10: 讨论 nginx 配置 → 压缩为:
  "Previously: User was configuring nginx reverse proxy to localhost:3000.
   Files modified: /etc/nginx/sites-available/default.
   Issue: port 80 was occupied by apache2, resolved by stopping apache2.
   Current status: nginx running, config tested OK."

轮11-20: [保留在窗口中]
轮21-30: [当前对话]
```

**前缀匹配策略**:
- 压缩时保留前 2 轮作为锚定上下文
- 语义检索时优先匹配最近的摘要

### 5.4 分级检索机制

```
用户查询 "nginx 配置"
    │
    ├── 第1级: 精确匹配 (SQLite LIKE/全文索引)
    │   ├── 搜索当前会话描述和关键词
    │   ├── 搜索记忆标签
    │   └── 命中 → 直接返回
    │
    ├── 第2级: RAG 语义搜索 (向量相似度)
    │   ├── 对查询生成 embedding
    │   ├── 在向量库中搜索 top-10 相似记忆
    │   ├── 按相似度阈值过滤 (cosine > 0.7)
    │   └── 命中 → 作为上下文注入
    │
    ├── 第3级: 模糊匹配
    │   ├── 关键词拆解 + 模糊搜索
    │   ├── 目录名/文件名模糊匹配
    │   └── 命中 → 返回，标记低置信度
    │
    └── 未命中 → "未找到相关记忆，是否联网搜索？"
```

**命中频率排序**:
- 每次记忆被检索命中，计数器 +1
- 高频记忆保持在 SQLite 主表，低频率的移入归档表
- 归档表不在默认检索范围内，但可通过 `--all` 标志检索

### 5.5 归档与清理策略

**自动归档**:
- 闲置超过 7 天的会话，自动生成最终摘要并移入归档
- 归档会话释放向量索引 (仅保留摘要文本，不保留 embedding)
- 归档文件存储为压缩 JSON: `~/.ai/sessions/archive/{id}.json.gz`

**自动清理**:
- 超过 30 天的归档会话，保留摘要但删除原始消息
- 超过 90 天的，完全删除 (或用户可配置保留策略)

**手动操作**:
```bash
ai session archive my-session    # 手动归档
ai session unarchive my-session  # 恢复归档
ai session clean --all           # 清理全部已归档
```

**会话分组**:
```bash
# 把 nginx 配置相关的多个会话绑定
ai session group ses-001 ses-002 ses-005 --tag "nginx-config"

# 后续检索时，分组内会话的上下文共享
```

**过滤机制**:
- 会话标记时效性: `ephemeral` (临时，不归档) vs `persistent` (持久)
- 一次性的 `ai 查看ip` 自动标记为 ephemeral
- 长对话、explore、agent 自动标记为 persistent
- ephemeral 会话 24 小时后自动清理

---

## 6. 记忆系统详细设计

### 6.1 三层记忆架构

```
┌─────────────────────────────────────────────┐
│              短期记忆 (内存)                  │
│  - 当前会话消息窗口 (最近 N 轮)               │
│  - 工具调用结果缓存                          │
│  - 系统信息缓存                              │
│  - 生命周期: 进程存活期间                     │
│  - 容量: ~20 轮对话 + 工具结果              │
└────────────────────┬────────────────────────┘
                     │ 压缩 / 归档
                     ▼
┌─────────────────────────────────────────────┐
│              中期记忆 (SQLite)               │
│  - 会话摘要                                  │
│  - 结构化记忆: 命令/配置/偏好/目录结构        │
│  - 全文索引                                  │
│  - 生命周期: 持久化，按策略归档               │
│  - 容量: 无上限 (百万级记录)                 │
└────────────────────┬────────────────────────┘
                     │ embedding 生成
                     ▼
┌─────────────────────────────────────────────┐
│              长期记忆 (向量 + RAG)            │
│  - 重要记忆的 embedding 向量                 │
│  - 语义相似度检索                            │
│  - 生命周期: 持久化                           │
│  - 容量: 受向量库限制                        │
└─────────────────────────────────────────────┘
```

### 6.2 SQLite 存储结构

```sql
-- 会话表
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,     -- UUID
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,        -- chat/explore/agent
    description TEXT,
    keywords    TEXT,                  -- JSON array
    status      TEXT DEFAULT 'active',-- active/idle/archived/cold
    group_id    TEXT,
    parent_id   TEXT,
    cwd         TEXT,
    os          TEXT,
    shell       TEXT,
    message_count INTEGER DEFAULT 0,
    token_count   BIGINT DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_active DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 消息表 (原始消息 + 压缩摘要)
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    role        TEXT NOT NULL,        -- user/assistant/system/tool
    content     TEXT NOT NULL,
    tool_calls  TEXT,                  -- JSON
    tool_result TEXT,                  -- JSON
    token_count INTEGER,
    is_summary  BOOLEAN DEFAULT 0,    -- 是否为压缩摘要
    seq         INTEGER,              -- 顺序号
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 记忆表 (结构化知识)
CREATE TABLE memories (
    id          TEXT PRIMARY KEY,
    session_id  TEXT REFERENCES sessions(id),
    type        TEXT NOT NULL,        -- command/config/preference/file_info/note
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    tags        TEXT,                  -- JSON array
    cwd         TEXT,                  -- 关联目录
    hit_count   INTEGER DEFAULT 0,    -- 检索命中次数
    importance  INTEGER DEFAULT 0,    -- 重要性 0-10
    is_ephemeral BOOLEAN DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 全文索引 (FTS5)
CREATE VIRTUAL TABLE memories_fts USING fts5(
    title, content, tags,
    content=memories,
    content_rowid=rowid
);

-- 目录快照表
CREATE TABLE dir_snapshots (
    id          TEXT PRIMARY KEY,
    cwd         TEXT NOT NULL,
    snapshot    TEXT NOT NULL,         -- dir2txt 输出
    file_count  INTEGER,
    hash        TEXT,                  -- 内容 hash，用于去重
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 白名单表
CREATE TABLE command_whitelist (
    id          TEXT PRIMARY KEY,
    command     TEXT NOT NULL,         -- 命令模式，如 "git push*"
    category    TEXT DEFAULT 'modify', -- modify/sudo
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 审计日志
CREATE TABLE audit_log (
    id          TEXT PRIMARY KEY,
    session_id  TEXT,
    command     TEXT NOT NULL,
    category    TEXT,                  -- view/modify/sudo/dangerous
    user_confirmed BOOLEAN,
    exit_code   INTEGER,
    duration_ms INTEGER,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX idx_messages_session ON messages(session_id, seq);
CREATE INDEX idx_memories_type ON memories(type);
CREATE INDEX idx_memories_cwd ON memories(cwd);
CREATE INDEX idx_memories_hit ON memories(hit_count DESC);
CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_sessions_last_active ON sessions(last_active);
CREATE INDEX idx_audit_session ON audit_log(session_id);
```

### 6.3 向量检索存储

```go
// 向量存储使用 chromem-go (嵌入 Go 的向量库)
// 或使用 sqlite-vec 扩展做向量检索

type VectorMemory struct {
    ID        string
    MemoryID  string
    Content   string     // 原始文本
    Embedding []float32  // 768/1536 维向量
    Metadata  map[string]string
}
```

**检索流程**:
```
1. 用户查询 → 生成 embedding (调用 LLM API 或本地模型)
2. 在向量库中搜索 top-K 相似向量 (cosine similarity)
3. 过滤: similarity < 0.5 的结果丢弃
4. 按 similarity * log(hit_count + 1) 综合排序
5. 返回 top-N 结果作为上下文
```

### 6.4 渐进式积累

```
每次使用 ai:
  │
  ├── 命令执行成功 → 自动记录 "port_check→lsof" 映射
  │
  ├── 用户纠错 → "上次给的那个命令不对" → 更新记忆权重
  │
  ├── 新文件/目录 → 检测变化 → 提示是否更新快照
  │
  └── ai note → 用户主动记录 → 高重要性标记
```

---

## 7. Shell 集成详细设计

### 7.1 Wrapper 函数

```bash
# ~/.bashrc 或 ~/.zshrc 中添加

# ai wrapper 函数
ai() {
    # 保存原始输入
    local original_args="$@"
    
    # 调用 Go 二进制
    # Go 程序将实际命令写入临时文件 + 返回给 stdout
    local tmpfile=$(mktemp)
    
    # 执行 ai，同时捕获实际命令
    command ai-internal "$@" --history-file="$tmpfile"
    local exit_code=$?
    
    # 如果生成了可执行命令，替换到 shell 历史
    if [[ -f "$tmpfile" && -s "$tmpfile" ]]; then
        local actual_cmd=$(cat "$tmpfile")
        # 将实际命令写入历史
        history -s "$actual_cmd"
        rm -f "$tmpfile"
    fi
    
    return $exit_code
}

# 如果需要 ai xxx 本身也出现在历史中，设置:
# export AI_HISTORY_KEEP_ORIGINAL=1
```

### 7.2 历史记录处理

**替换模式 (默认)**:
```bash
# 用户输入: ai 列出端口
# history 中写入: lsof -i :8085
# 按上键看到: lsof -i :8085
```

**实现方式**:
1. Go 程序通过 `--history-file` 标志接收临时文件路径
2. 实际执行的命令写入该临时文件
3. shell wrapper 读取并使用 `history -s` 写入历史
4. `ai xxx` 原始命令通过 HISTCONTROL 排除 (ignorespace)

**配置选项**:
```yaml
# ~/.ai/config.yaml
shell:
  history_mode: "replace"     # replace | append | none
  keep_original: false        # 是否保留 ai xxx 在历史中
  history_ignorespace: true   # 使用 HISTCONTROL=ignorespace
```

### 7.3 流式展示实现

**目标**: 命令逐字出现，让用户看到进度

```go
// Go 侧: LLM 返回时，边接收边输出
func streamCommand(w io.Writer, llmResponse <-chan string) {
    fmt.Fprint(w, "\n🤖 ")
    for chunk := range llmResponse {
        fmt.Fprint(w, chunk)
        // 不使用缓冲，立即刷新
        if f, ok := w.(interface{ Flush() error }); ok {
            f.Flush()
        }
    }
    fmt.Fprintln(w)
}
```

**效果**:
```
$ ai 查看内存占用
🤖 f│r│e│e│ │-│h│  (逐字出现，约 50ms/字)
              total        used        free      shared  buff/cache   available
Mem:           15Gi       3.2Gi       8.1Gi       234Mi       4.2Gi        11Gi
Swap:         2.0Gi          0B       2.0Gi
```

**加急输出**:
- 命令字符快速出现 (约 30-50ms/字)，模拟打字效果
- 命令完整显示后，立即执行 (不等)
- 结果直接输出的同时，AI 可以做简短总结

### 7.4 安装脚本

```bash
# ai shell install 执行的内容
install_shell_integration() {
    local shell_rc
    
    case "$SHELL" in
        */bash) shell_rc="$HOME/.bashrc" ;;
        */zsh)  shell_rc="$HOME/.zshrc" ;;
        *) echo "Unsupported shell: $SHELL"; return 1 ;;
    esac
    
    # 检查是否已安装
    if grep -q "### AI_SHELL_INTEGRATION" "$shell_rc"; then
        echo "Shell integration already installed in $shell_rc"
        return 0
    fi
    
    # 追加 wrapper 函数
    cat >> "$shell_rc" << 'EOF'
### AI_SHELL_INTEGRATION_START
# AI Terminal Assistant - Shell Integration
# Generated by: ai shell install
# Remove with: ai shell uninstall

ai() {
    local tmpfile=$(mktemp)
    local exit_code
    
    command ai-internal "$@" --history-file="$tmpfile"
    exit_code=$?
    
    if [[ -f "$tmpfile" && -s "$tmpfile" ]]; then
        local actual_cmd=$(cat "$tmpfile")
        history -s "$actual_cmd"
        rm -f "$tmpfile"
    fi
    
    return $exit_code
}

# 排除 ai 命令本身进入历史
export HISTCONTROL=ignorespace:ignoredups

### AI_SHELL_INTEGRATION_END
EOF
    
    echo "Shell integration installed. Please run: source $shell_rc"
}
```

---

## 8. 安全设计

### 8.1 命令分类体系

```go
type CommandCategory string

const (
    CatView      CommandCategory = "view"      // 纯查看，直接执行
    CatModify    CommandCategory = "modify"    // 修改，需确认
    CatSudo      CommandCategory = "sudo"      // 需要提权
    CatDangerous CommandCategory = "dangerous" // 危险操作，需二次确认
    CatForbidden CommandCategory = "forbidden" // 禁止执行
)
```

**分类规则**:
```go
var commandClassifier = map[string]struct {
    Patterns []string
    Category CommandCategory
}{
    "view": {
        Patterns: []string{
            "ls", "cat", "head", "tail", "less", "more",
            "grep", "find", "locate", "which", "whereis",
            "ps", "top", "htop", "free", "df", "du",
            "lsof", "netstat", "ss", "ip", "ifconfig",
            "uname", "hostname", "whoami", "id", "groups",
            "pwd", "stat", "file", "wc", "sort", "uniq",
            "echo", "printf", "date", "cal",
            "git log", "git status", "git diff", "git show",
            "systemctl status", "journalctl",
            "docker ps", "docker images", "docker logs",
        },
        Category: CatView,
    },
    "modify": {
        Patterns: []string{
            "rm", "mv", "cp", "touch", "mkdir", "rmdir",
            "chmod", "chown", "chgrp",
            "git add", "git commit", "git push", "git checkout",
            "git branch", "git merge", "git rebase",
            "systemctl start", "systemctl stop", "systemctl restart",
            "systemctl enable", "systemctl disable",
            "docker run", "docker stop", "docker rm",
            "npm install", "pip install", "apt install", "brew install",
            "make", "cmake", "go build", "cargo build",
        },
        Category: CatModify,
    },
    // ... sudo 和 dangerous 类似处理
}
```

### 8.2 执行流程

```
AI 建议命令
    │
    ├── 分类检查
    │   ├── view → 直接执行 ✓
    │   ├── modify → 检查白名单
    │   │   ├── 在白名单中 → 直接执行 ✓
    │   │   └── 不在 → 询问用户
    │   ├── sudo → 检测是否需要密码
    │   │   ├── 需要 → 提示用户输入
    │   │   └── 无需密码 (NOPASSWD) → 按 modify 规则
    │   ├── dangerous → 强制二次确认 (需要输入 "yes" 确认)
    │   └── forbidden → 拒绝执行 ✗
    │
    ├── 执行
    │   ├── 设置超时 (默认 30s, 可配)
    │   ├── 限制输出大小 (默认 10KB)
    │   ├── 记录审计日志
    │   └── 返回结果
    │
    └── 后处理
        ├── 敏感信息过滤 (密码/密钥正则)
        └── 输出结果
```

### 8.3 路径安全

```go
// 路径沙盒检查
func isPathSafe(cmd string, workingDir string) bool {
    // 提取命令中所有路径参数
    paths := extractPaths(cmd)
    
    for _, p := range paths {
        absPath := resolvePath(p, workingDir)
        
        // 检查是否在允许的范围内
        if !isInAllowedPaths(absPath) {
            return false
        }
        
        // 检查符号链接攻击
        if isSymlinkDangerous(absPath) {
            return false
        }
    }
    return true
}

// 允许的路径范围
var allowedPaths = []string{
    "/home/",      // 用户目录
    "/tmp/",       // 临时目录
    "/etc/",       // 只读访问
    "/var/log/",   // 只读访问
}
```

### 8.4 敏感信息过滤

```go
var sensitivePatterns = []*regexp.Regexp{
    // API Keys
    regexp.MustCompile(`(?i)(api[_-]?key|apikey|secret[_-]?key)\s*[:=]\s*\S+`),
    // Passwords
    regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
    // Tokens
    regexp.MustCompile(`(?i)(token|jwt|bearer)\s+[A-Za-z0-9\-_.]+`),
    // Private keys
    regexp.MustCompile(`-----BEGIN (RSA |EC )?PRIVATE KEY-----`),
}

func sanitizeOutput(output string) string {
    for _, p := range sensitivePatterns {
        output = p.ReplaceAllString(output, "***REDACTED***")
    }
    return output
}
```

### 8.5 审计日志

每次 AI 发起的命令执行都记录:
- 命令内容
- 分类 (view/modify/sudo/dangerous)
- 用户是否确认
- 退出码
- 执行耗时
- 会话 ID

```bash
ai memory audit --last 10   # 查看最近 10 条审计记录
ai memory audit --today     # 今日审计
```

---

## 9. 实现路线图

### 第一阶段: 最小可用版本 (MVP) -- 2-3 周

**目标**: 能用 `ai 查看xxx` 这种最简单的形式，AI 生成命令并执行

**交付物**:
1. Go 项目骨架
   - cobra CLI 框架搭建
   - viper 配置管理
   - 项目结构: `cmd/`, `internal/`, `pkg/`
2. LLM 集成 (单后端)
   - OpenAI API 适配器
   - 基础 prompt 组装
3. 命令分类与执行
   - 查看类命令直接执行
   - 修改类命令确认执行
   - sudo 命令检测
4. Shell wrapper
   - bash/zsh 函数
   - 历史替换
5. 流式输出
   - 命令逐字展示
6. 基础配置
   - `~/.ai/config.yaml`
   - API key 设置

**验证标准**:
```bash
ai 查看内存占用     → 生成 free -h → 直接执行 → 输出结果
ai 创建 test 目录   → 生成 mkdir test → 询问确认 → 执行
ai 查看nginx配置    → 生成 sudo cat ... → 提示密码 → 执行
```

### 第二阶段: 上下文与记忆 -- 2-3 周

**目标**: AI 能记住之前的对话，用户有持续会话体验

**交付物**:
1. SQLite 存储层
   - 消息持久化
   - 会话管理
2. 短期上下文窗口
   - 最近 20 轮消息管理
3. 压缩策略
   - 窗口溢出时自动压缩
4. `ai chat` 模式
   - 多轮对话
   - 会话切换 (ai chat -s)
5. 中期记忆
   - 命令-结果记录
   - 简单检索 (关键词匹配)

**验证标准**:
```bash
ai chat
> 我的nginx配置在哪？
AI: /etc/nginx/nginx.conf  (记忆中的)
> 帮我看看有没有错误
AI: sudo nginx -t  (上下文连贯)
```

### 第三阶段: Explore & Agent -- 2-3 周

**目标**: 独立的只读探索和读写 agent 模式

**交付物**:
1. `ai explore` 子命令
   - 只读工具集
   - 独立上下文
   - 项目分析能力
2. `ai agent` 子命令
   - 读写工具集
   - 写操作确认
   - 文件修改能力
3. dir2txt 集成
   - 目录快照生成
   - 自动更新检测
4. 会话分组
   - explore/agent 关联到主会话

**验证标准**:
```bash
ai explore 为什么构建失败？
→ 独立分析，不污染主会话

ai agent 帮我添加一个新的 systemd service
→ 创建文件，需要确认
```

### 第四阶段: 长期记忆与 RAG -- 2-3 周

**目标**: 跨会话的语义记忆检索

**交付物**:
1. 向量存储
   - chromem-go 或 sqlite-vec 集成
2. Embedding 生成
   - OpenAI embedding API
   - 或本地 onnx 模型
3. RAG 检索管线
   - 语义搜索
   - 混合检索 (关键词 + 语义)
4. `ai note` 和 `ai search`
5. 归档与清理
   - 自动归档策略
   - 冷热数据分离
6. 渐进式披露
   - 系统信息按需获取
   - 目录结构按需加载

**验证标准**:
```bash
ai note "数据库密码在 ~/db-pass.txt 第一行"
# ... 一周后，新会话 ...
ai search 数据库密码
→ 找到记忆: 数据库密码在 ~/db-pass.txt 第一行
```

### 第五阶段: 增强与优化 -- 持续

**交付物**:
1. 联网搜索
2. 多 LLM 后端 (Ollama 支持)
3. macOS 适配
4. 多 profile
5. 性能优化
6. 测试覆盖

---

## 10. 技术选型

### 10.1 核心框架

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| CLI 框架 | `github.com/spf13/cobra` | Go 生态最成熟的 CLI 框架，支持子命令、标志、自动补全 |
| 配置管理 | `github.com/spf13/viper` | 与 cobra 同源，支持 YAML/JSON/ENV，多配置源合并 |
| 日志 | `go.uber.org/zap` | 高性能结构化日志，适合生产环境 |
| 数据库 | `github.com/ncruces/go-sqlite3` | 纯 Go 实现，无需 CGO，嵌入方便，支持 FTS5 |

### 10.2 LLM 交互

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| OpenAI | `github.com/sashabaranov/go-openai` | 最成熟的 Go OpenAI SDK，支持流式、工具调用、embedding |
| Anthropic | `github.com/anthropics/anthropic-sdk-go` | 官方 SDK，支持 Claude 系列 |
| Ollama | `github.com/ollama/ollama` API | REST API 调用，支持本地模型 |
| 抽象层 | 自定义 `LLMClient` 接口 | 统一接口，方便切换后端 |

```go
type LLMClient interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

### 10.3 向量检索

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| 向量存储 | `github.com/philippgille/chromem-go` | 纯 Go 实现，嵌入式向量库，支持持久化，零依赖 |
| 备选 | `github.com/asg017/sqlite-vec` | SQLite 向量扩展，SQL 查询向量，但需要编译扩展 |
| Embedding API | OpenAI `text-embedding-3-small` | 托管服务，1536 维，效果好，零运维 |
| 本地 Embedding | `github.com/yalue/onnxruntime_go` + all-MiniLM-L6-v2 | 本地推理，无需联网，384 维 |

**推荐方案**: 先用 OpenAI embedding API (无需额外部署)，后续按需切本地。

### 10.4 工具与工具集成

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| Shell 执行 | 标准库 `os/exec` | 无需第三方，功能完备 |
| 终端输出 | `github.com/charmbracelet/lipgloss` | 优雅的终端样式，支持颜色、布局 |
| 流式渲染 | `github.com/charmbracelet/bubbletea` 或手动 ANSI | 可选，用于复杂 TUI；简单流式用 fmt 即可 |
| 进度提示 | 自定义 spinner | 简单转圈动画 |
| dir2txt | 调用外部二进制 | `os/exec` 调用已有的 dir2txt 工具 |

### 10.5 测试

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| 单元测试 | 标准库 `testing` | Go 原生，足够 |
| 断言 | `github.com/stretchr/testify` | 最流行的 Go 测试辅助库 |
| Mock | `go.uber.org/mock` | Uber 维护的 mockgen，生成接口 mock |
| LLM Mock | 自定义 recorder/replayer | 录制 LLM 响应用于回归测试 |

### 10.6 不推荐的库

| 库 | 理由 |
|----|------|
| `mattn/go-sqlite3` | 需要 CGO，跨平台编译复杂 |
| `langchaingo` | 太重，抽象过多，本项目只需轻量 LLM 客户端 |
| `gpt4all` | 依赖重，不如直接用 Ollama API |

---

## 11. 项目结构建议

```
ai/
├── cmd/
│   └── ai/
│       └── main.go                 # 入口
├── internal/
│   ├── cli/
│   │   ├── root.go                 # cobra root command
│   │   ├── chat.go                 # ai chat
│   │   ├── explore.go              # ai explore
│   │   ├── agent.go                # ai agent
│   │   ├── search.go               # ai search
│   │   ├── note.go                 # ai note
│   │   ├── memory.go               # ai memory
│   │   ├── config.go               # ai config
│   │   ├── shell.go                # ai shell
│   │   └── session.go              # ai session
│   ├── orchestrator/
│   │   ├── orchestrator.go         # 调度器核心
│   │   ├── intent.go               # 意图识别
│   │   ├── context.go              # 上下文组装
│   │   └── safety.go               # 安全策略引擎
│   ├── agent/
│   │   ├── base.go                 # Agent 基础接口
│   │   ├── main_agent.go           # 主 Agent (命令+聊天)
│   │   ├── explore_agent.go        # Explore Agent (只读)
│   │   └── worker_agent.go         # Agent Agent (读写)
│   ├── memory/
│   │   ├── store.go                # 存储接口
│   │   ├── sqlite.go               # SQLite 实现
│   │   ├── vector.go               # 向量存储
│   │   ├── compressor.go           # 压缩/摘要
│   │   └── retriever.go            # 分级检索
│   ├── llm/
│   │   ├── client.go               # LLM 客户端接口
│   │   ├── openai.go               # OpenAI 适配器
│   │   ├── anthropic.go            # Anthropic 适配器
│   │   └── ollama.go               # Ollama 适配器
│   ├── tool/
│   │   ├── registry.go             # 工具注册表
│   │   ├── shell.go                # shell 执行工具
│   │   ├── file.go                 # 文件读/写工具
│   │   ├── search.go               # 联网搜索工具
│   │   └── system.go               # 系统信息工具
│   └── config/
│       ├── config.go               # 配置结构定义
│       └── default.go              # 默认配置
├── pkg/
│   └── shell/
│       ├── wrapper.sh              # bash wrapper 模板
│       └── wrapper.zsh             # zsh wrapper 模板
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 12. 配置文件示例

```yaml
# ~/.ai/config.yaml

# LLM 配置
llm:
  provider: openai              # openai | anthropic | ollama
  model: gpt-4o                 # 默认模型
  fast_model: gpt-4o-mini       # 用于压缩摘要的快速/便宜模型
  embedding_model: text-embedding-3-small
  api_key: ${OPENAI_API_KEY}    # 支持环境变量
  base_url: ""                  # 自定义 API 地址
  max_tokens: 4096
  temperature: 0.0

# Shell 集成
shell:
  history_mode: replace         # replace | append | none
  keep_original: false
  auto_execute_view: true       # 查看类命令自动执行
  stream_output: true           # 流式输出

# 上下文
context:
  max_history: 20               # 窗口大小 (轮)
  compress_threshold: 30        # 超过此轮数触发压缩
  compress_keep_prefix: 2       # 压缩时保留前 N 轮

# 记忆
memory:
  auto_record: true             # 自动记录命令映射
  auto_snapshot: true           # 自动更新目录快照
  archive_after_days: 7         # 闲置天数后归档
  cold_after_days: 30           # 归档后转为冷存储
  clean_after_days: 90          # 彻底清理

# 安全
security:
  sudo_auto_detect: true        # 自动检测 sudo 需求
  output_size_limit: 10240      # 命令输出大小限制 (bytes)
  exec_timeout: 30              # 命令执行超时 (秒)
  sensitive_filter: true        # 敏感信息过滤

# 命令白名单
whitelist:
  - "git push*"
  - "git commit*"
  - "npm install*"
  - "pip install*"
  - "make*"

# 禁止命令
forbidden:
  - "rm -rf /*"
  - "dd if=* of=/dev/*"
  - ":(){ :|:& };:"

# 显示
display:
  style: auto                   # auto | plain | rich
  color: true
  show_tokens: false            # 显示 token 消耗
  show_cost: false              # 显示 API 费用
```

---

## 13. 附录: 关键数据流图

### 13.1 命令执行完整流程

```
用户输入 "ai 列出大文件"
        │
        ▼
┌─────────────────┐
│ Shell Wrapper    │  ai() bash 函数
│ 记录原始输入     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Cobra CLI        │  解析: 无子命令 → 智能命令模式
│ 路由             │  args: ["列出大文件"]
└────────┬────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ Orchestrator.runSmartCommand()       │
│                                      │
│  1. 意图识别 (快速模型, ~200ms):     │
│     "列出大文件" → 命令生成          │
│                                      │
│  2. 上下文组装:                      │
│     - System: 终端助手, Linux, bash  │
│     - Memory: [空, 新用户]           │
│     - User: "列出当前目录大文件"     │
│                                      │
│  3. 调用 LLM:                       │
│     → "find . -type f -exec du -h {} + | sort -rh | head -10"
│                                      │
│  4. 流式输出到终端                   │
│                                      │
│  5. 安全分类: "view" → 直接执行      │
│                                      │
│  6. 执行命令 (30s 超时)              │
│                                      │
│  7. 结果输出                         │
│                                      │
│  8. 写入历史文件                     │
└──────────────────────────────────────┘
         │
         ▼
┌─────────────────┐
│ Shell Wrapper    │  读取历史文件
│ history -s       │  → "find . -type f ..."
└─────────────────┘
```

---

*本文档随项目演进持续更新。*
