[2026-06-05] - v0.1.3
T1. 版本号升级的时候应该支持比如1.12 > 1.11 这种形式的可能某个版本配置文件多次变动。✅
T9. [未复现问题]命令长度超过当前终端的宽度会导致自动换行，如果这个时候下面不在新的光标下面去输出会导致错误的重叠和覆盖。
T3. 使用less查看log而不是vi ✅
  - `OpenReadOnly` 改用 `exec.Command("less", "-R", path)`，`-R` 保留 ANSI 颜色转义
  - 删除已无引用的 `ReadOnlyArgs` 函数
  - `GetEditor` 仍由 `setup.go` 使用，未动

T10. 让思考过程的显示宽度和当前终端同宽 ✅
  - `default.yaml` 中 `thinking_line_len` 默认改为 `0`，表示使用终端宽度
  - `NewThinkingDisplay` 构造器：`<= 0` 改为 `< 0`，允许 0 流通到 `Start()`
  - `Start()` 中 `effWidth = termWidth - 2`，减去 `render()` 的 2 空格缩进
  - `fillDefaults` 跳过 `ThinkingLineLen`，避免 0 被覆盖

T16. 配置系统清理：消除"补丁叠补丁"，让 struct 成为唯一真相源 ✅
  - ThinkingLineLen/APITimeout/StreamTimeout/ExecTimeout 改为 *int 指针类型，nil=未设，*0=显式 0
  - fillDefaults 删掉 ThinkingLines/ThinkingLineLen/APITimeout/StreamTimeout/ExecTimeout 的 skip 列表
  - NewThinkingDisplay 构造器删掉 maxLines<=0→3、maxLineLen<0→30 的 fallback
  - llm/client.go 删掉 apiTimeout=300、streamTimeout=30 的 fallback，直接解引用 *cfg
  - executor/executor.go 删掉 timeout<=0→30 的 fallback
  - default.yaml 成为唯一默认值源，struct 成为运行时唯一真相

T17. 思考过程流式输出空白区域问题 ✅
  - render() 原来始终输出 maxLines+1 行（1 spinner + maxLines 内容/空行），未填满时下方出现空白
  - rendered bool 改为 lastRenderLines int，记录上次实际输出行数
  - render() 只输出有内容的行，遇到空行 break，lastRenderLines = 1 + contentLines
  - cursor-up 和 Stop() 清理都使用 lastRenderLines，避免多余空白

T4. 为所有用户安装和卸载功能 ✅
  - 系统安装：binary 安装到 /usr/local/bin/，shell wrapper 写入 /etc/profile.d/aicli.sh（sudo mv + chmod 0644）
  - systemWrapper 不含 PATH 行（/usr/local/bin 已在默认 PATH 中）
  - 系统安装后跳过 .bashrc 注入，直接 return
  - Uninstall 新增范围选择菜单 selectUninstallScope()，复用 drawMenu
  - 系统卸载：sudo rm -f /usr/local/bin/aicli 和 /etc/profile.d/aicli.sh

T5. 保留用户原始提示到 shell history ✅
  - wrapper 和 systemWrapper 中在 history -s 前添加 history -a
  - history -a 将当前内存中的历史行（用户的 ai ... 输入）追加到历史文件
  - history -s 再将 AI 生成的命令作为新条目添加

T18. shell install 更新已存在的 wrapper ✅
  - 新增 replaceWrapper() 辅助函数，提取 marker 之间的完整内容（含 marker）与当前常量比较
  - 用户安装：wrapper 内容不同时 os.WriteFile 更新 .bashrc，相同时跳过
  - 系统安装：sudo cat 读取 /etc/profile.d/aicli.sh，不同时 writeSystemProfile() 更新
  - 新增 writeSystemProfile() 辅助函数，提取 temp file + sudo mv + chmod 逻辑
  - 标记损坏（有开始无结束）当作不存在，fall through 到追加逻辑自愈