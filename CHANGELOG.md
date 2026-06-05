# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [v0.1.4] - 2026-06-05

### Fixed
- T2: 配置迁移保留用户 slice 和嵌套 map 数据 — whitelist/forbidden_patterns/thinking_body 不再丢失
- T5: shell history 保留用户原始提示 — `history -a` + `echo >> HISTFILE` + `history -n` 替代会删除条目的 `history -s`
<!-- - T14: 命令输出颜色 — `bash -l -c` 继承用户环境变量（LS_COLORS 等） -->
- T22: 失败命令不再写入 shell history — tmpfile 仅在 exitCode==0 时写入

### Changed
- 更新用户输入内容为user(用户输入内容)

## [v0.1.3] - 2026-06-05

### Added
- 思考模式 `--think` / `-t` — ThinkingDisplay v2，流式滚动显示 AI 思考过程
- 系统级安装/卸载 — `shell install` 支持系统范围（`/usr/local/bin/` + `/etc/profile.d/aicli.sh`）
- shell install wrapper 自动更新 — 已存在时比较内容，不同时自动更新，标记损坏自愈
- 保留用户原始提示到 shell history — wrapper 中 `history -a` + `history -s`
- 清除控制台后续内容功能，优化用户交互体验
- GitHub Actions 发布工作流 — tag 推送自动编译并创建 Release

### Changed
- 思考显示宽度自动适配终端宽度（`thinking_line_len: 0` 表示终端同宽）
- 配置系统清理 — 超时/显示参数改为 `*int` 指针类型，`default.yaml` 成为唯一默认值源
- 配置版本比较支持多段版本号（`1.12 > 1.11`），版本类型改为字符串
- 日志查看改用 `less -R` 替代 vi，保留 ANSI 颜色
- 统一输入读取器 InputReader，解决 DSR/stdin 竞争，非 debug 零开销

### Fixed
- 思考过程流式输出空白区域 — `render()` 只输出有内容的行，动态控制输出行数
- wrapper 中 `history -a` 未正确追加用户 ai 命令

## [v0.1.2] - 2026-06-01

### Added
- `shell install` 选择 UI（系统/用户），PATH 自动添加
- `shell install` 自动安装二进制到 `~/.local/bin/`，wrapper 调用 aicli，原子写入
- 命令行参数添加调试信息输出选项（`-d`/`--debug`），重构日志初始化逻辑
- 配置和日志功能增强：最大日志文件名长度限制、日志初始化改进
- 日志中添加时间戳以增强命令执行可追踪性
- 分离系统提示词系统，增加缓存命中率

### Changed
- `openReadOnly` 移至 utils 包，选项取消可选中，箭头循环，光标隐藏
- 优化命令行细节处理

### Fixed
- 日志目录路径统一为 `~/.aicli/logs/`
- shell install 信号竞态修复

## [v0.1.1] - 2026-05-29

### Added
- 版本号 `v0.1.0`，支持 `-v`/`--version` 参数，帮助信息对齐
- 请求体 debug 格式化，`ai log` 子命令，日志文件名含摘要

### Changed
- `config.go` 拆分为 8 个子文件（types/paths/load/save/validate/migrate/setup/whitelist）
- 日志查看函数独立为 `viewer.go`，`GetEditor` 导出，`openReadOnly` 只读模式，TOCTOU 修复
- 创建 utils 工具包，迁移 `GetEditor`/`FormatError`/`ResolveDir`

## [v0.1.0] - 2026-05-28

### Added
- AI MVP 原型实现 — AI 驱动的命令行交互，OpenAI 兼容 API 接口
- 命令规则系统 — forbidden/dangerous/readonly 分层防护，规则移至配置文件
- 精简 prompt + 系统信息注入 + 分类确定即执行
- 双标记系统 — `#$` 命令前缀（不显示） + 分类标记
- 统一日志系统 — Debug/Info/Warn/Error/Print，调试日志写文件，配置项控制
- thinking 模式配置 + extra_body 合并 + 配置自动补全（reflect）
- 配置版本检查与安全更新，审计修复
- `StreamChat` debug 模式，累积和输出原始 delta JSON
- `setup` 过程 `mergeDefaults` 函数，合并缺失的顶层配置

### Fixed
- 交互式确认提示优化：不换行、raw mode 清行、`-> 请确认[a/y/N]`
- 流式处理数据竞争 — 命令完成通过 channel 传递，消除轮询
- 信号处理 goroutine 泄漏 — done channel 通知退出
- `sh` 改为 `bash`，支持 `echo -e` 等 bash 特性
- setup 交互修复 — 回车/无效键处理、错误信息不重复、退出码正确
- setup 编辑器不显示 — `exec.Command` 需继承 stdin/stdout/stderr
- help 输出对齐，添加子命令标题
- 错误信息压缩为单行显示
- `migrateConfigFile` 改为模板+替换值方式，保留注释和结构
