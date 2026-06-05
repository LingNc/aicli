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