# `aicli` - 终端智能助手项目规划文档

> **版本**: 1.0
> **最后更新**: 2026-05-31
> **语言**: Go
> **目标平台**: Linux (主要), macOS (可扩展)
> **核心定位**: 命令为主 + 轻量开发 -- 不是编程 IDE 工具，是终端命令辅助工具

---

## 1. 项目概述与愿景

`aicli` 是一个终端智能助手，命令行入口为 `ai` 或 `aicli`（两者等价，`aicli` 用于避免与其他工具命名冲突）。用户输入 `ai xxx`，AI 帮助生成/执行命令，替代记忆和手动输入。目标是将 AI 封装成一条原生命令，快速、简洁、无摩擦。

**核心理念**:
- 默认不啰嗦，给出最可能的命令或答案
- 纯查看类命令直接执行，无需确认
- 命令写入 shell 历史，按上键看到的是实际命令而非 `ai xxx`
- AI 渐进了解用户和机器，随使用越来越智能
- 会话持久化存储，不依赖进程存活，随时恢复对话
- AI 永不接触真实密钥，凭证通过占位符机制安全注入

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
| `ai chat` 模式 | 持续对话，会话持久化存储，自动续接 |
| 会话持久化与自动续接 | SQLite 存储，30min 窗口自动恢复，退出不丢失 |
| sudo 命令检测 | 自动识别需要提权的命令，提示用户输入密码 |
| 命令白名单 | 用户可将修改类命令加入白名单，之后直接执行 |
| 中期记忆 | SQLite 存储压缩摘要，会话间可检索 |
| Shell wrapper 函数 | `.bashrc` / `.zshrc` 集成 (ai/aicli 双入口) |
| Prompt 前缀缓存 | 系统信息/目录/记忆前缀复用，LLM API 缓存友好 |
| -v 详细模式 | 显示完整推理过程和工具调用链 |

### P2 - 进阶功能
| 功能 | 说明 |
|------|------|
| `ai explore` 子智能体 | 只读分析模式，独立上下文，从主会话派生 |
| `ai agent` 模式 | 可修改文件的独立 agent 环境 |
| 凭证管理系统 | 加密存储，占位符机制，AI 永不接触真实密钥 |
| 长期记忆 (RAG) | 向量检索，语义搜索 |
| 会话分组与归档 | 按话题分组，长时间不用自动归档 |
| 分级检索 | 精确匹配 -> RAG 语义搜索 -> 模糊匹配 |
| 配置管理 | `~/.aicli/config.yaml` 管理所有配置 |

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
                  │  - ai()/aicli() 函数捕获原始输入           │
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
                  │  │ - 持久化会话 │  │ - Prompt 组装      │   │
                  │  │ - 自动续接   │  │ - 前缀缓存控制     │   │
                  │  │ - 分组/归档  │  │ - Token 预算控制   │   │
                  │  │ - 派生 Agent │  │ - 记忆注入         │   │
                  │  └─────────────┘  └───────────────────┘   │
                  │                                           │
                  │  ┌─────────────┐  ┌───────────────────┐   │
                  │  │ 凭证解析器   │  │ Prompt Cache Mgr  │   │
                  │  │ - 占位符替换 │  │ - 前缀哈希/复用   │   │
                  │  │ - 自动检测   │  │ - TTL 管理         │   │
                  │  │ - 安全注入   │  │ - 缓存命中统计    │   │
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
      │              │ │ credential   │ │              │
      └──────────────┘ └──────┬───────┘ └──────────────┘
              │               │
      ┌───────┴───────┐       │
      ▼               ▼       ▼
┌───────────┐   ┌───────────┐  ┌───────────────────┐
│  SQLite   │   │ Vector DB │  │ Credential Store  │
│ ~/.aicli/mem │   │ ~/.aicli/vec │  │ ~/.aicli/credentials │
└───────────┘   └───────────┘  │ .enc (加密)       │
                               └───────────────────┘
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
  - 持久化会话存储 (SQLite)，不依赖进程存活
  - 自动续接: 短时间内（默认 30min）再次调用 `ai chat`，自动接上上次会话
  - 超时自动新开会话（时间阈值可配置）
  - 手动指定会话: `ai chat -s session-name` 切换/恢复特定会话
  - 派生 Agent: explore/agent 从主会话派生，继承上下文但有独立对话窗口
  - 会话分组与归档
- **上下文组装器**:
  - Prompt 组装: 系统信息 + 记忆 + 当前上下文
  - **前缀缓存控制**: 分离静态前缀（系统信息、目录快照、记忆注入）与动态后缀（用户输入、最近对话），保证短时间内相同前缀可被 LLM API 的 prompt caching 命中
  - Token 预算控制: 确保不超出模型限制
  - 按优先级注入记忆: 短摘要 > 语义相关 > 目录快照
- **凭证解析器**:
  - 占位符替换: 将 AI 响应中的 `{{credential-id}}` 在执行前替换为真实密钥
  - 自动检测: 识别常见凭证模式（环境变量赋值、curl header 等），提示用户存储
  - 逆向替换: 工具返回值中的疑似密钥，替换为占位符后再送给 LLM
  - 安全注入: 执行命令时注入真实凭证，进程内存中短暂持有后立即清除
- **Prompt Cache Manager**:
  - 前缀哈希计算与复用: 短时间内未变化的系统信息/目录/记忆部分，复用上一次的 prompt 前缀
  - TTL 管理: 缓存有效期（默认 5min），超时后重新计算前缀
  - 缓存命中统计: 记录命中率，用于调试和优化

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
- credential_lookup: 查询凭证标识符列表（仅返回标识符，不返回真实值）

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
   │   ├── 计算前缀哈希: 系统信息 + 目录快照 + 记忆注入
   │   ├── 检查 Prompt Cache: 若前缀未变化且未过期(5min)，复用前缀
   │   ├── 注入系统信息: Linux, bash, uid=1000, cwd=/home/user/project
   │   ├── 查询记忆: 无相关记忆
   │   └── 组装 Prompt: [缓存前缀] + [用户输入]
   │
4. 主 Agent 调用 LLM:
   │  System: "你是终端助手。用户需要命令建议。只返回命令和简短解释。"
   │  User: "列出占用端口为8085的程序"
   │  LLM 返回: "lsof -i :8085"
   │
5. 凭证解析器检查:
   ├── 检测命令中是否含占位符 (如 {{xxx}})
   ├── 无占位符 → 跳过
   │
6. 安全策略引擎检查:
   ├── 分类: 纯查看命令 ✓
   ├── 白名单: N/A
   └── 结果: 允许直接执行
   │
7. 流式输出到终端: "lsof -i :8085" (逐字出现)
   │
8. 执行命令:
   ├── stdout: "COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME\n..."
   ├── stderr: (空)
   └── 退出码: 0
   │
9. 结果整理 (LLM 可选项):
   ├── 敏感信息过滤: 输出中若有疑似密钥，替换为占位符
   └── 输出精简摘要或原始输出
   │
10. Shell Wrapper 将 "lsof -i :8085" 写入 shell 历史
   │
11. 更新记忆:
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
  chat       进入持续对话模式（自动续接或新开会话）
  explore    启动只读探索子智能体
  agent      启动读写独立 agent
  search     搜索记忆或联网搜索
  note       记录知识到长期记忆
  memory     记忆管理（搜索/查看/导出/清理）
  config     管理配置
  credential 凭证管理（添加/列出/删除/测试）
  shell      安装/更新 shell 集成
  session    会话管理（列表/切换/归档/分组）

Global Flags:
  -v, --verbose         显示详细推理过程
  -q, --quiet           静默模式，仅输出结果
  -m, --model string    临时切换模型
  -s, --session string  指定会话名称（chat/explore/agent 均可用）
  --no-exec             只建议命令，不执行
  --no-history          不写入 shell 历史
  --dry-run             显示将执行的命令但不执行
  --new-session         强制新开会话（即使短时间内有活跃会话）

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
# 自动续接: 30min 内再次调用，自动接上上次会话
ai chat
> [自动恢复上次会话 my-session]
> 上下文: 之前我们在讨论 nginx 配置...

# 超时后自动新开会话
ai chat
> [距上次会话已超过 30min，创建新会话]
> 新会话已创建: ses-20260531-001

# 手动指定会话
ai chat -s my-session      # 恢复指定会话
ai chat --new-session       # 强制新开会话（忽略自动续接）

# 查看会话
ai chat --list              # 列出所有 chat 会话

# 单次对话模式
ai chat 解释一下刚才那条命令  # 单次对话，不进入交互模式
```

**自动续接规则**:
- 默认阈值: 30 分钟内再次调用 `ai chat`，自动恢复到上一次活跃的 chat 会话
- 阈值可通过 `context.session_auto_resume_minutes` 配置
- 若上次会话已被归档，则创建新会话
- 使用 `--new-session` 强制跳过自动续接

**对话模式交互**:
```
You: 为什么我的nginx启动失败了？
AI: 让我查看一下... [执行 sudo systemctl status nginx]
    nginx 启动失败，错误: port 80 already in use
    端口 80 被进程 PID 1234 (apache2) 占用
    建议: systemctl stop apache2 && systemctl start nginx
You: 帮我停掉apache
AI: [执行 sudo systemctl stop apache2] ✓ 已停止
You: exit / quit / Ctrl+C
→ 退出对话模式，会话已持久化保存
→ 下次 ai chat 将自动续接
```

**会话持久化**:
- 每次对话轮次即时写入 SQLite，不依赖进程存活
- 退出对话模式后，会话状态保持为活跃
- 下次调用 `ai chat` 时，系统从 SQLite 加载完整上下文
- 消息按 seq 排序重建对话窗口

**派生 Agent**:
- 在 chat 会话中可通过指令派生 explore/agent
- 派生 Agent 继承主会话的上下文摘要，但有独立对话窗口
- 派生 Agent 的消息不污染主会话窗口

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

#### ai credential - 凭证管理

```bash
# 添加凭证
ai credential add my-openai-key
> 请输入密钥: **********
> ✓ 已安全存储，标识符: my-openai-key

ai credential add github-token
> 请输入密钥: **********
> ✓ 已安全存储，标识符: github-token

# 列出所有凭证（仅显示标识符，不显示值）
ai credential list
> my-openai-key     (created: 2026-05-31, last used: 1h ago)
> github-token      (created: 2026-05-30, last used: 2d ago)
> aws-access-key    (created: 2026-05-28, never used)

# 删除凭证
ai credential remove my-openai-key
> ⚠ 确认删除凭证 my-openai-key？[y/N] y
> ✓ 已删除

# 测试凭证是否有效
ai credential test github-token
> 测试中...
> ✓ 凭证有效

# 导入环境变量中的密钥
ai credential import OPENAI_API_KEY
> ✓ 已从环境变量 OPENAI_API_KEY 导入，标识符: openai-api-key
> 环境变量值已标记为"已导入"，下次启动将不再提示
```

**使用场景**:
```bash
# AI 生成命令时自动使用占位符
ai 设置 OPENAI_API_KEY 环境变量并运行 python script.py
> AI 生成: export OPENAI_API_KEY={{my-openai-key}} && python script.py
> 系统执行时自动替换为真实密钥

# curl 请求中自动注入
ai 用我的 github token 调用 API 查看我的仓库列表
> AI 生成: curl -H "Authorization: Bearer {{github-token}}" https://api.github.com/user/repos
> 系统执行时自动替换为真实 token
```

**自动检测与提示**:
```bash
# 当 AI 生成的命令中包含可识别的凭证模式时，系统自动提示
ai 设置 AWS 环境变量
> AI 生成: export AWS_SECRET_ACCESS_KEY=...
> 💡 检测到 AWS_SECRET_ACCESS_KEY，是否存储为凭证？[y/N]
> y: 存储，之后 AI 将使用占位符
> N: 跳过，本次直接使用
> 标识符: aws-secret-key
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
创建 ──→ 活跃 ──→ 可续接(30min) ──→ 闲置(7天) ──→ 归档 ──→ 冷存储
  │         │          │                │           │           │
  │         │          │ 恢复(自动)     │ 恢复      │ 恢复      │ 恢复(慢)
  │         │          │◄──────────────┘           │           │
  │         │          │                            │           │
  │         └──────────┴────────────────────────────┘           │
  └─────────────────────────────────────────────────────────────┘
```

**状态定义**:
- **活跃**: 当前或最近 30 分钟内使用的会话，再次调用 `ai chat` 自动续接
- **可续接**: 30 分钟到 7 天未使用的会话，可通过 `-s` 手动恢复
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
    Status      string    // "active" | "resumable" | "idle" | "archived" | "cold"
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

### 5.2.1 会话自动续接机制

**核心原则**: 会话持久化在 SQLite 中，不依赖进程存活。每次 `ai chat` 调用时，系统从存储中加载会话状态，而非依赖内存中的进程。

**续接判定流程**:
```
ai chat 被调用
    │
    ├── 检查是否指定 -s <name>
    │   ├── 是 → 加载指定会话
    │   └── 否 → 继续
    │
    ├── 检查是否指定 --new-session
    │   ├── 是 → 创建新会话
    │   └── 否 → 继续
    │
    ├── 查询 SQLite: 最近一次活跃的 chat 会话
    │   ├── 无历史会话 → 创建新会话
    │   └── 找到上次会话 → 继续
    │
    ├── 检查时间差: now - LastActive
    │   ├── < auto_resume_minutes (默认 30min) → 自动续接
    │   │   ├── 从 SQLite 加载完整消息历史
    │   │   ├── 恢复压缩摘要和窗口上下文
    │   │   └── 显示: "已恢复会话 <name> (上次: 5 分钟前)"
    │   │
    │   └── >= auto_resume_minutes → 创建新会话
    │       └── 显示: "距上次会话已超过 30min，创建新会话"
    │
    └── 若上次会话已被归档 → 创建新会话
```

**配置**:
```yaml
context:
  session_auto_resume_minutes: 30   # 自动续接时间窗口（分钟）
  session_auto_resume_enabled: true # 是否启用自动续接
```

**派生 Agent 会话关联**:
```
主 chat 会话 (ses-001)
    │
    ├── explore 派生 → ses-001-explore-1
    │   ParentID = "ses-001"
    │   继承: 主会话的系统信息 + 当前上下文摘要
    │   独立: 自己的消息窗口，不污染主会话
    │
    └── agent 派生 → ses-001-agent-1
        ParentID = "ses-001"
        继承: 主会话的系统信息 + 当前上下文摘要
        独立: 自己的消息窗口 + 文件操作权限
```

### 5.3 前缀匹配与 Prompt Cache 优化

**设计目标**: 利用用户操作的局部性——短时间内连续多次查询，系统信息、当前目录、上下文基本不变——尽可能保证 prompt 前缀匹配，避免重复组装和传输相同的前缀内容，让 LLM API 的 prompt caching 机制生效。

**Prompt 结构分层**:
```
┌──────────────────────────────────────────────┐
│              前缀 (静态/缓存友好)              │
│                                              │
│  1. System Prompt (固定，按模式选择)          │
│  2. 系统信息 (OS, Shell, User, Hostname)      │
│  3. 当前目录快照 (dir2txt 输出)               │
│  4. 记忆注入 (SQLite 检索结果)                │
│  5. 会话压缩摘要 (若存在)                     │
│                                              │
│  注: 以上内容在短时间内通常不变                │
│  系统对前缀计算哈希，相同则复用                │
├──────────────────────────────────────────────┤
│              后缀 (动态)                      │
│                                              │
│  6. 最近 N 轮对话 (窗口消息)                  │
│  7. 当前用户输入                              │
│                                              │
│  注: 这些内容每次请求都会变化                  │
└──────────────────────────────────────────────┘
```

**缓存策略**:
```go
type PromptCache struct {
    PrefixHash string        // 前缀内容的 SHA256 哈希
    PrefixText string        // 缓存的完整前缀文本
    CreatedAt  time.Time
    TTL        time.Duration // 默认 5 分钟
}

// 每次组装 prompt 时:
func (ca *ContextAssembler) BuildPrompt(session *Session, userInput string) *Prompt {
    prefix := ca.buildPrefix(session)       // 组装系统信息 + 记忆 + 摘要
    prefixHash := sha256(prefix)

    // 检查缓存: 前缀未变 + 未过期 → 复用
    if ca.cache != nil && ca.cache.PrefixHash == prefixHash && time.Since(ca.cache.CreatedAt) < ca.cache.TTL {
        // LLM API 的 prompt caching 将命中此前缀
        // 大多数 LLM 提供商 (OpenAI, Anthropic) 支持 prompt caching
        // 相同前缀部分不会被重复计费
        metrics.CacheHits++
    } else {
        ca.cache = &PromptCache{
            PrefixHash: prefixHash,
            PrefixText: prefix,
            CreatedAt:  time.Now(),
            TTL:        5 * time.Minute,
        }
    }

    suffix := ca.buildSuffix(session, userInput) // 窗口消息 + 当前输入
    return &Prompt{Prefix: prefix, Suffix: suffix}
}
```

**触发缓存失效的条件**:
- 目录切换 (cwd 变化)
- 目录快照更新 (文件增删)
- 新记忆注入 (新检索结果)
- TTL 过期 (超过 5 分钟)
- 会话切换

**LLM API Prompt Caching 注意事项**:
- OpenAI: prompt caching 对超过 1024 token 的前缀自动生效 (gpt-4o, gpt-4o-mini)
- Anthropic: 需显式标记 `cache_control` 断点，最多 4 个断点
- 适配器层抽象: 各 LLM 后端实现各自的缓存标记方式
- 缓存命中时，前缀 token 按 10%-25% 的价格计费（取决于提供商）

### 5.4 压缩策略

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

### 5.5 分级检索机制

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

### 5.6 归档与清理策略

**自动归档**:
- 闲置超过 7 天的会话，自动生成最终摘要并移入归档
- 归档会话释放向量索引 (仅保留摘要文本，不保留 embedding)
- 归档文件存储为压缩 JSON: `~/.aicli/sessions/archive/{id}.json.gz`

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
    status      TEXT DEFAULT 'active',-- active/resumable/idle/archived/cold
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
    command aicli-internal "$@" --history-file="$tmpfile"
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

# aicli 别名 (与 ai 等价，用于避免命名冲突)
aicli() {
    ai "$@"
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
# ~/.aicli/config.yaml
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
    
    command aicli-internal "$@" --history-file="$tmpfile"
    exit_code=$?
    
    if [[ -f "$tmpfile" && -s "$tmpfile" ]]; then
        local actual_cmd=$(cat "$tmpfile")
        history -s "$actual_cmd"
        rm -f "$tmpfile"
    fi
    
    return $exit_code
}

# aicli 别名 (防冲突，与 ai 等价)
aicli() {
    ai "$@"
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

### 8.6 凭证安全

**核心原则**: AI 模型永远不接触真实密钥值。所有密钥交互通过占位符机制完成。

**凭证注入安全**:
- 真实密钥仅在命令执行前由凭证解析器解密，注入命令字符串后立即执行
- 执行完成后立即从进程内存中清除（`runtime.KeepAlive` 禁用 + 显式清零）
- 绝不将真实密钥写入日志、审计记录或任何持久化存储
- 审计日志中，凭证相关命令的敏感部分替换为 `[CREDENTIAL:my-key]`

**工具返回值清洗**:
- 所有 AI 可调用的工具（shell_exec, file_read）的返回值在发送给 LLM 前必须经过凭证过滤器
- 若返回值包含已知凭证的真实值，替换为 `{{credential-id}}` 占位符
- 若返回值匹配通用敏感模式（API key 格式、私钥头尾等），替换为 `***REDACTED***`

**存储安全** (详见 Section 9):
- 凭证文件使用 AES-256-GCM 加密存储
- 主密钥通过 Argon2id 从用户口令派生
- 文件权限强制 0600

**AI 提示中的凭证信息**:
- 系统提示/记忆注入中可包含凭证标识符列表（如 "可用凭证: {{my-key}}, {{gh-token}}"），但绝不包含真实值
- 上下文注入策略: 仅注入当前任务可能相关的凭证标识符，而非全部

**内存加固建议**:
```go
// 生产环境可选的额外加固
import "github.com/awnumar/memguard"

func (cs *CredentialStore) Decrypt(id string) (string, error) {
    // 使用 memguard 保护的缓冲区
    lb := memguard.NewBuffer(32) // locked buffer, 不可 swap
    defer lb.Destroy()
    // ... 解密操作 ...
    // 命令执行后立即 Destroy
}
```

---

## 9. 凭证管理系统

### 9.1 核心原则

**AI 永远不会接触到真实密钥。** 系统在所有与 AI 的交互边界上实施凭证隔离：
- AI 生成的命令中，真实密钥被替换为占位符
- AI 的工具调用返回值中，疑似密钥被替换为占位符
- AI 只看到 `{{credential-id}}` 形式的标识符，永远不接触真实值

### 9.2 架构设计

```
用户添加凭证                       AI 生成/执行命令
     │                                    │
     ▼                                    ▼
┌──────────────┐                  ┌──────────────────┐
│ ai credential │  加密存储        │  Orchestrator     │
│ add <name>   │ ──────────────→  │  凭证解析器       │
└──────────────┘                  │                  │
                                  │ 1. 解析 AI 响应   │
┌──────────────┐                  │ 2. 检测占位符     │
│ 系统 keyring │  (备选方案)      │ 3. 替换为真实密钥 │
│ 或加密文件   │                  │ 4. 执行命令       │
└──────────────┘                  │ 5. 清除内存中的   │
                                  │    真实密钥       │
┌──────────────┐                  └────────┬─────────┘
│ ~/.aicli/    │                           │
│ credentials  │                           ▼
│ .enc         │                  ┌──────────────────┐
│ (AES-256-    │                  │  Tool 拦截层     │
│  GCM 加密)   │                  │                  │
└──────────────┘                  │ shell_exec 结果: │
                                  │ 疑似密钥 → 替换  │
                                  │ file_read 内容:  │
                                  │ 疑似密钥 → 替换  │
                                  └──────────────────┘
```

### 9.3 存储安全

**主方案: 加密文件**
```go
type CredentialStore struct {
    path       string           // ~/.aicli/credentials.enc
    masterKey  []byte           // 从用户口令派生 (PBKDF2/Argon2)
}

type CredentialEntry struct {
    ID          string          // 用户定义的标识符
    Encrypted   []byte          // AES-256-GCM 加密的密钥值
    Nonce       []byte          // 加密 nonce
    Patterns    []string        // 关联的命令模式 (如 "OPENAI_API_KEY")
    CreatedAt   time.Time
    LastUsed    time.Time
    UseCount    int64
}

// 存储格式: JSON 数组, AES-256-GCM 加密后写入文件
// 文件权限: 0600 (仅用户可读写)
```

**备选方案: 系统 keyring**
- Linux: Secret Service API (gnome-keyring / kde-wallet)
- macOS: Keychain
- 优点: 系统级安全，自动锁定/解锁
- 缺点: 依赖系统服务，跨平台复杂度高

**推荐**: 先用加密文件方案（零依赖），后续可扩展系统 keyring 适配器。

**主密钥派生**:
```go
func deriveMasterKey(passphrase string, salt []byte) []byte {
    // Argon2id 或 PBKDF2-SHA256
    // 用户首次使用时设定主口令，后续使用时要求输入（或缓存到 keyring）
    return argon2.IDKey([]byte(passphrase), salt, 
        1,          // iterations (tune for target machine)
        64*1024,    // memory
        4,          // parallelism
        32,         // key length
    )
}
```

### 9.4 占位符机制

**占位符格式**: `{{credential-id}}`

**替换流程**:
```
AI 生成的命令字符串
    │
    ▼
┌─────────────────────────────────────────┐
│ 正则匹配: \{\{([a-zA-Z0-9_-]+)\}\}     │
│                                         │
│ export OPENAI_API_KEY={{my-openai-key}} │
│                                         │
│ 匹配到: my-openai-key                   │
├─────────────────────────────────────────┤
│ 查询 CredentialStore.Get("my-openai-key")│
│ → "sk-proj-abc123..."                   │
├─────────────────────────────────────────┤
│ 替换后的命令:                            │
│ export OPENAI_API_KEY=sk-proj-abc123... │
│                                         │
│ 执行后立即从内存中清除真实密钥            │
└─────────────────────────────────────────┘
```

**工具返回值逆向替换**:
```go
// 工具返回值处理管线
func sanitizeToolResult(result string) string {
    // 1. 匹配已知凭证值
    for _, cred := range credentialStore.All() {
        realValue := cred.Decrypt()
        if strings.Contains(result, realValue) {
            result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", cred.ID))
        }
    }
    
    // 2. 通用敏感模式正则匹配
    result = sanitizePatterns(result) // 复用 Section 8 的 sensitivePatterns
    
    return result
}
```

### 9.5 自动检测模式

系统内置常见凭证模式，当 AI 生成的命令匹配时，自动提示存储:

```go
var credentialPatterns = map[string][]string{
    // 环境变量模式
    "env_var": {
        `OPENAI_API_KEY`,
        `ANTHROPIC_API_KEY`,
        `AWS_ACCESS_KEY_ID`,
        `AWS_SECRET_ACCESS_KEY`,
        `GITHUB_TOKEN`,
        `GITLAB_TOKEN`,
        `DOCKER_PASSWORD`,
        `NPM_TOKEN`,
        `PYPI_TOKEN`,
    },
    // curl header 模式
    "curl_header": {
        `Authorization: Bearer`,
        `Authorization: Basic`,
        `X-API-Key:`,
        `api-key:`,
    },
    // 配置文件模式
    "config_file": {
        `password:`,
        `secret:`,
        `token:`,
        `api_key:`,
    },
}

// 自动检测触发器
func (cr *CredentialResolver) DetectAndPrompt(cmd string) {
    for pattern, value := range extractCredentialPatterns(cmd) {
        fmt.Printf("💡 检测到可能包含凭证: %s (模式: %s)\n", pattern, value)
        fmt.Printf("   是否存储为凭证？[y/N/a(始终存储)] ")
        // ... 处理用户响应
    }
}
```

**检测时机**:
1. AI 生成命令后，执行前 — 检测命令中是否含未识别的密钥模式
2. 用户手动输入的命令 (如 `ai config set api-key`) — 自动提示存储

### 9.6 AI 工具层面的拦截

当 AI 调用的工具返回值中可能包含密钥时，在工具层做拦截:

```go
type CredentialInterceptor struct {
    store *CredentialStore
}

func (ci *CredentialInterceptor) Intercept(toolName string, result string) string {
    // 对所有 AI 可见的工具返回值做清洗
    switch toolName {
    case "shell_exec":
        // shell 输出可能包含密钥 (如 cat ~/.env, echo $API_KEY)
        return ci.replaceWithPlaceholders(result)
    case "file_read":
        // 文件内容可能包含密钥 (如 .env, config.yaml)
        return ci.replaceWithPlaceholders(result)
    case "dir2txt":
        // 目录快照不会包含敏感信息，跳过
        return result
    default:
        return result
    }
}

func (ci *CredentialInterceptor) replaceWithPlaceholders(text string) string {
    for _, cred := range ci.store.All() {
        realValue := ci.store.Decrypt(cred)
        text = strings.ReplaceAll(text, realValue, 
            fmt.Sprintf("{{%s}}", cred.ID))
    }
    // 通用敏感模式也做替换
    text = sanitizePatterns(text)
    return text
}
```

### 9.7 主密钥管理

**首次使用流程**:
```
$ ai credential add my-key
> 首次使用凭证管理，请设置主口令:
> 主口令: **********
> 确认口令: **********
> 📝 请牢记此口令。口令丢失则凭证无法恢复。
> ✓ 主口令已设置
> 请输入密钥: **********
> ✓ 已安全存储，标识符: my-key
```

**会话解锁**:
```
$ ai credential list
> 🔒 凭证存储已锁定，请输入主口令:
> 主口令: **********
> ✓ 已解锁 (本次会话有效)
> my-key  (created: 2026-05-31)
```

**解锁策略**:
- 每个 shell 会话首次访问凭证时需要输入主口令
- 解锁后，主密钥缓存在进程内存中（Go 的 `memguard` 或类似方案保护）
- 可选项: 将主密钥缓存到系统 keyring (30min TTL)，避免频繁输入
- 进程退出时自动清除主密钥

### 9.8 数据流示例

**完整流程: AI 使用凭证执行命令**:
```
1. 用户: ai 用我的 openai key 测试 API 连通性
   │
2. Orchestrator 上下文组装:
   ├── 系统信息 + 目录 + 记忆 → 前缀 (缓存命中)
   ├── 凭证列表注入: "可用的凭证标识符: {{my-openai-key}}, {{github-token}}"
   │   (AI 只看到标识符列表，不看到真实值)
   └── 用户输入 → 后缀
   │
3. LLM 返回:
   curl -H "Authorization: Bearer {{my-openai-key}}" https://api.openai.com/v1/models
   │
4. 凭证解析器:
   ├── 检测到占位符 {{my-openai-key}}
   ├── 查询 CredentialStore → 解密 → "sk-proj-abc123..."
   ├── 替换: curl -H "Authorization: Bearer sk-proj-abc123..." ...
   └── 内存中保留真实值 (30s 超时后清除)
   │
5. 安全策略引擎: 分类为 view → 直接执行
   │
6. 执行命令 → 获取结果
   │
7. 工具返回值拦截:
   ├── stdout: "{"data": [...], "object": "list"}"
   ├── 无密钥泄露 → 直接返回给 LLM
   └── (若含密钥，替换为占位符后再返回)
   │
8. LLM 整理结果 → 输出给用户
   │
9. 清除内存中的真实密钥
```

### 9.9 安全考量

- **内存安全**: 真实密钥仅在命令执行前后短暂存在于进程内存中。使用 `memguard` 或手动清零缓冲区
- **Core dump 防护**: 生产环境建议禁用 core dump 或使用 `mlock` 锁定敏感内存页
- **审计日志**: 凭证的添加、删除、使用记录写入审计日志，敏感值被替换为 `[CREDENTIAL:my-key]`
- **加密算法**: AES-256-GCM (认证加密)，防止篡改
- **密钥派生**: Argon2id (抗 GPU/ASIC 暴力破解)
- **文件权限**: `~/.aicli/credentials.enc` 设置为 0600
- **TTL 清除**: 真实密钥解密后 30 秒自动从内存清除

---

## 10. 实现路线图

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
   - `~/.aicli/config.yaml`
   - API key 设置

**验证标准**:
```bash
ai 查看内存占用     → 生成 free -h → 直接执行 → 输出结果
ai 创建 test 目录   → 生成 mkdir test → 询问确认 → 执行
ai 查看nginx配置    → 生成 sudo cat ... → 提示密码 → 执行
```

### 第二阶段: 上下文与记忆 -- 2-3 周

**目标**: AI 能记住之前的对话，用户有持续会话体验，会话持久化不依赖进程

**交付物**:
1. SQLite 存储层
   - 消息持久化
   - 会话管理
2. 会话自动续接
   - 30min 窗口内自动恢复
   - 超时自动新开会话
   - `ai chat -s` 手动切换
3. 短期上下文窗口
   - 最近 20 轮消息管理
4. 压缩策略
   - 窗口溢出时自动压缩
5. `ai chat` 模式
   - 多轮对话
   - 会话切换 (ai chat -s)
   - 退出后会话持久化保留
6. 前缀缓存
   - 系统信息/目录/记忆前缀复用
   - LLM API prompt caching 命中性优化
7. 中期记忆
   - 命令-结果记录
   - 简单检索 (关键词匹配)

**验证标准**:
```bash
ai chat
> 我的nginx配置在哪？
AI: /etc/nginx/nginx.conf  (记忆中的)
> 帮我看看有没有错误
AI: sudo nginx -t  (上下文连贯)
> exit
# 5 分钟后...
ai chat
> [自动恢复上次会话] 接着聊...
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

### 第四阶段: 凭证管理系统 -- 1-2 周

**目标**: AI 永不接触真实密钥，凭证通过占位符机制安全管理

**交付物**:
1. 加密存储
   - AES-256-GCM 加密文件存储
   - Argon2id 主密钥派生
   - 文件权限 0600
2. `ai credential` CLI
   - 添加/列出/删除/测试凭证
   - 导入环境变量中的密钥
3. 占位符解析与替换
   - `{{credential-id}}` 格式解析
   - 执行时自动注入真实密钥
   - 内存中密钥 TTL 清除
4. 自动检测
   - 常见凭证模式识别 (环境变量、curl header 等)
   - 提示用户存储未管理的密钥
5. 工具拦截层
   - shell_exec/file_read 返回值过滤
   - 疑似密钥替换为占位符

**验证标准**:
```bash
ai credential add my-key
> ✓ 已安全存储
ai 用 my-key 调用 API
> AI: curl -H "Authorization: Bearer {{my-key}}" ...
> 系统执行时自动替换，AI 从未接触真实值
```

### 第五阶段: 长期记忆与 RAG -- 2-3 周

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

### 第六阶段: 增强与优化 -- 持续

**交付物**:
1. 联网搜索
2. 多 LLM 后端 (Ollama 支持)
3. macOS 适配
4. 多 profile
5. 性能优化
6. 测试覆盖

---

## 11. 技术选型

### 11.1 核心框架

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| CLI 框架 | `github.com/spf13/cobra` | Go 生态最成熟的 CLI 框架，支持子命令、标志、自动补全 |
| 配置管理 | `github.com/spf13/viper` | 与 cobra 同源，支持 YAML/JSON/ENV，多配置源合并 |
| 日志 | `go.uber.org/zap` | 高性能结构化日志，适合生产环境 |
| 数据库 | `github.com/ncruces/go-sqlite3` | 纯 Go 实现，无需 CGO，嵌入方便，支持 FTS5 |

### 11.2 LLM 交互

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

### 11.3 向量检索

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| 向量存储 | `github.com/philippgille/chromem-go` | 纯 Go 实现，嵌入式向量库，支持持久化，零依赖 |
| 备选 | `github.com/asg017/sqlite-vec` | SQLite 向量扩展，SQL 查询向量，但需要编译扩展 |
| Embedding API | OpenAI `text-embedding-3-small` | 托管服务，1536 维，效果好，零运维 |
| 本地 Embedding | `github.com/yalue/onnxruntime_go` + all-MiniLM-L6-v2 | 本地推理，无需联网，384 维 |

**推荐方案**: 先用 OpenAI embedding API (无需额外部署)，后续按需切本地。

### 11.4 工具与工具集成

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| Shell 执行 | 标准库 `os/exec` | 无需第三方，功能完备 |
| 终端输出 | `github.com/charmbracelet/lipgloss` | 优雅的终端样式，支持颜色、布局 |
| 流式渲染 | `github.com/charmbracelet/bubbletea` 或手动 ANSI | 可选，用于复杂 TUI；简单流式用 fmt 即可 |
| 进度提示 | 自定义 spinner | 简单转圈动画 |
| dir2txt | 调用外部二进制 | `os/exec` 调用已有的 dir2txt 工具 |

### 11.5 测试

| 组件 | 推荐库 | 理由 |
|------|--------|------|
| 单元测试 | 标准库 `testing` | Go 原生，足够 |
| 断言 | `github.com/stretchr/testify` | 最流行的 Go 测试辅助库 |
| Mock | `go.uber.org/mock` | Uber 维护的 mockgen，生成接口 mock |
| LLM Mock | 自定义 recorder/replayer | 录制 LLM 响应用于回归测试 |

### 11.6 不推荐的库

| 库 | 理由 |
|----|------|
| `mattn/go-sqlite3` | 需要 CGO，跨平台编译复杂 |
| `langchaingo` | 太重，抽象过多，本项目只需轻量 LLM 客户端 |
| `gpt4all` | 依赖重，不如直接用 Ollama API |

---

## 12. 项目结构建议

```
aicli/
├── cmd/
│   └── aicli/
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
│   │   ├── credential.go           # ai credential
│   │   ├── shell.go                # ai shell
│   │   └── session.go              # ai session
│   ├── orchestrator/
│   │   ├── orchestrator.go         # 调度器核心
│   │   ├── intent.go               # 意图识别
│   │   ├── context.go              # 上下文组装 + 前缀缓存
│   │   ├── cache.go                # Prompt Cache 管理
│   │   ├── credential.go           # 凭证解析器
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
│   │   ├── system.go               # 系统信息工具
│   │   └── credential.go           # 凭证拦截工具
│   ├── credential/
│   │   ├── store.go                # 凭证存储接口
│   │   ├── file.go                 # 加密文件存储实现
│   │   ├── keyring.go              # 系统 keyring 实现
│   │   ├── resolver.go             # 占位符解析/替换
│   │   └── detector.go             # 自动检测凭证模式
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

## 13. 配置文件示例

```yaml
# ~/.aicli/config.yaml

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

# 上下文与会话
context:
  max_history: 20               # 窗口大小 (轮)
  compress_threshold: 30        # 超过此轮数触发压缩
  compress_keep_prefix: 2       # 压缩时保留前 N 轮
  session_auto_resume: true     # 是否启用自动续接
  session_auto_resume_minutes: 30  # 自动续接时间窗口 (分钟)

# Prompt 缓存
prompt_cache:
  enabled: true                 # 是否启用前缀缓存
  ttl_minutes: 5                # 前缀缓存有效期 (分钟)
  track_hit_rate: true          # 是否统计缓存命中率

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
  credential_auto_detect: true  # 自动检测命令中的凭证模式并提示存储
  credential_ttl_seconds: 30    # 解密后凭证在内存中保留时间 (秒)

# 凭证存储
credentials:
  backend: file                 # file | keyring
  path: ~/.aicli/credentials.enc
  keyring_service: ""           # keyring 服务名 (仅 keyring 后端)
  auto_lock_minutes: 30         # keyring 缓存的解锁 TTL

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

## 14. 附录: 关键数据流图

### 14.1 命令执行完整流程

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
│     - 检查 Prompt Cache (前缀复用)   │
│     - System: 终端助手, Linux, bash  │
│     - Memory: [空, 新用户]           │
│     - User: "列出当前目录大文件"     │
│                                      │
│  3. 调用 LLM:                       │
│     → "find . -type f -exec du -h {} + | sort -rh | head -10"
│                                      │
│  4. 凭证解析: 无占位符 → 跳过       │
│                                      │
│  5. 流式输出到终端                   │
│                                      │
│  6. 安全分类: "view" → 直接执行      │
│                                      │
│  7. 执行命令 (30s 超时)              │
│                                      │
│  8. 结果清洗 (敏感信息过滤)          │
│                                      │
│  9. 结果输出                         │
│                                      │
│  10. 写入历史文件                    │
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
