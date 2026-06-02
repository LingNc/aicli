package config

import (
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/lingnc/aicli/internal/log"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultYAML []byte

// Config 是应用配置结构体
type Config struct {
	ConfigVersion     int            `yaml:"config_version"`
	APIKey            string         `yaml:"api_key"`
	BaseURL           string         `yaml:"base_url"`
	Model             string         `yaml:"model"`
	Mode              string         `yaml:"mode"`
	Debug             bool           `yaml:"debug"`
	DebugLogConsole   bool           `yaml:"debug_log_console"`
	DebugLogDir       string         `yaml:"debug_log_dir"`
	Whitelist         []string       `yaml:"whitelist"`
	ForbiddenPatterns []string       `yaml:"forbidden_patterns"` // 用户扩展的禁止命令
	DangerousPatterns []string       `yaml:"dangerous_patterns"` // 用户扩展的需确认命令
	RequestBody       map[string]any `yaml:"request_body"`       // 请求体参数，直接合并到 API 请求 JSON
}

// Dir 返回配置目录路径 (~/.aicli)
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取 home 目录失败: %w", err)
	}
	return filepath.Join(home, ".aicli"), nil
}

// Path 返回配置文件路径 (~/.aicli/config.yaml)
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Exists 检查配置文件是否存在
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Load 加载配置文件，不存在则返回默认配置。
// 如果检测到配置版本过旧，仅打印警告，由用户自行运行 'aicli setup' 更新。
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return loadDefault()
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 版本过旧提示，不自动迁移、不打开编辑器
	if cfg.ConfigVersion >= 0 && cfg.ConfigVersion < defaultConfigVersion() {
		log.Warn("配置文件版本过旧(v%d)，请在 'setup' 更新", cfg.ConfigVersion)
	}

	// 填充默认值（处理 struct 级零值，如 RequestBody nil）
	fillDefaults(cfg)
	return cfg, nil
}

// defaultConfigVersion 返回嵌入的 default.yaml 中的 config_version
func defaultConfigVersion() int {
	var cfg Config
	if err := yaml.Unmarshal(defaultYAML, &cfg); err != nil {
		return 0
	}
	return cfg.ConfigVersion
}

// getEditor 返回用户偏好的编辑器（$EDITOR > $VISUAL > vi）
func getEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	return "vi"
}

// promptConfigUpdate 配置版本不匹配时提示用户是否更新
func promptConfigUpdate(configPath string, userVer, defVer int) bool {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return false
	}

	fmt.Fprintf(os.Stderr, "-> 配置文件版本过旧 (v%d -> v%d)，是否更新？[y/N] ", userVer, defVer)

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false
	}
	defer term.Restore(fd, oldState)
	defer fmt.Fprintf(os.Stderr, "\r\033[K")

	buf := make([]byte, 1)
	if _, err := os.Stdin.Read(buf); err != nil {
		return false
	}

	return buf[0] == 'y' || buf[0] == 'Y'
}

// checkAndMigrateConfig 检查配置版本，过旧则提示用户并执行迁移。
// 返回迁移后的文件数据（未迁移则返回原始 data）和 error。
func checkAndMigrateConfig(configPath string, data []byte) ([]byte, error) {
	var userCfg Config
	if err := yaml.Unmarshal(data, &userCfg); err != nil {
		return data, nil
	}
	var defCfg Config
	yaml.Unmarshal(defaultYAML, &defCfg)
	if userCfg.ConfigVersion >= defCfg.ConfigVersion {
		return data, nil
	}

	if !promptConfigUpdate(configPath, userCfg.ConfigVersion, defCfg.ConfigVersion) {
		return data, nil
	}

	if err := migrateConfigFile(configPath, data); err != nil {
		log.Warn("更新配置文件失败: %v", err)
		return data, nil
	}

	newData, err := os.ReadFile(configPath)
	if err != nil {
		return data, fmt.Errorf("读取迁移后配置失败: %w", err)
	}
	return newData, nil
}

// migrateConfigFile 以 default.yaml 为模板合并用户已有值。
// 保留模板的注释、结构和顺序，只替换用户已有的简单键值。
// 复杂字段（slice、嵌套 map）和 request_body 保留模板默认值，用户可在编辑器中手动修改。
func migrateConfigFile(configPath string, userData []byte) error {
	// 1. 备份原文件
	backupPath := configPath + ".bak"
	if err := os.WriteFile(backupPath, userData, 0600); err != nil {
		return fmt.Errorf("备份配置文件失败: %w", err)
	}

	// 2. 解析用户值
	var userMap map[string]any
	if err := yaml.Unmarshal(userData, &userMap); err != nil {
		return fmt.Errorf("解析用户配置失败: %w", err)
	}

	// 3. 以 defaultYAML 为模板，逐行替换用户已有值
	lines := strings.Split(string(defaultYAML), "\n")
	replaced := make(map[string]bool)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}
		// 只处理顶层 key
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			continue // 跳过非顶层行
		}
		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])

		// 跳过 config_version、request_body 和复杂字段（slice、嵌套 map）
		if key == "config_version" || key == "request_body" {
			continue
		}
		userVal, exists := userMap[key]
		if !exists {
			continue
		}
		if _, isSlice := userVal.([]any); isSlice {
			continue // slice 保留模板默认值
		}
		if _, isMap := userVal.(map[string]any); isMap {
			continue // 嵌套 map 保留模板默认值
		}

		// 替换值（保留缩进和 key）
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		// 保留行内注释
		comment := ""
		commentIdx := strings.Index(trimmed, " #")
		if commentIdx > idx {
			comment = trimmed[commentIdx:]
		}
		lines[i] = indent + key + ": " + formatYAMLScalar(userVal) + comment
		replaced[key] = true
	}

	// 4. 追加用户有但模板没有的 key（按块排序：scalar 为单独块，slice 为 key+items 整体）
	type extraBlock struct {
		sortKey string   // 用于排序的 key
		lines   []string // 该块的所有行
	}
	var blocks []extraBlock
	for k, v := range userMap {
		if replaced[k] || k == "config_version" || k == "request_body" {
			continue
		}
		if _, isMap := v.(map[string]any); isMap {
			continue // 嵌套 map 过于复杂，跳过
		}
		if s, isSlice := v.([]any); isSlice {
			lines := []string{k + ":"}
			for _, item := range s {
				lines = append(lines, "  - "+formatYAMLScalar(item))
			}
			blocks = append(blocks, extraBlock{sortKey: k, lines: lines})
			continue
		}
		blocks = append(blocks, extraBlock{sortKey: k, lines: []string{k + ": " + formatYAMLScalar(v)}})
	}

	if len(blocks) > 0 {
		sort.Slice(blocks, func(i, j int) bool {
			return blocks[i].sortKey < blocks[j].sortKey
		})
		result := strings.Join(lines, "\n")
		result += "\n\n# ──── 用户自定义字段 ────\n"
		for _, b := range blocks {
			result += strings.Join(b.lines, "\n") + "\n"
		}
		return os.WriteFile(configPath, []byte(result), 0600)
	}

	return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0600)
}

// formatYAMLScalar 将 Go 值格式化为 YAML 标量
func formatYAMLScalar(v any) string {
	switch val := v.(type) {
	case string:
		if val == "" || strings.ContainsAny(val, ":@#{}[]&*!|>'\"`,\n") || isYAMLKeyword(val) {
			b, _ := yaml.Marshal(val)
			return strings.TrimSpace(string(b))
		}
		return val
	default:
		b, _ := yaml.Marshal(val)
		return strings.TrimSpace(string(b))
	}
}

// isYAMLKeyword 检查字符串是否为 YAML 关键字（yaml.v3 会将这些值解析为 bool/null/float 而非 string）
func isYAMLKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~", ".inf", "-.inf", ".nan", "-.nan":
		return true
	}
	return false
}

// loadDefault 从嵌入的默认配置加载
func loadDefault() (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultYAML, cfg); err != nil {
		return nil, fmt.Errorf("解析默认配置失败: %w", err)
	}
	return cfg, nil
}

// fillDefaults 用默认值填充空字段（基于反射，自动从 default.yaml 补全零值）
// 跳过 RequestBody 字段，请求体由用户自行管理。
func fillDefaults(cfg *Config) {
	var defaults Config
	if err := yaml.Unmarshal(defaultYAML, &defaults); err != nil {
		return // 解析失败不填充，使用零值
	}

	cfgV := reflect.ValueOf(cfg).Elem()
	defV := reflect.ValueOf(&defaults).Elem()
	t := cfgV.Type()

	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Name == "RequestBody" {
			continue // 请求体不自动填充
		}
		field := cfgV.Field(i)
		if field.IsZero() {
			field.Set(defV.Field(i))
		}
	}
}

// Save 保存配置到文件
func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// SaveDefault 将默认配置写入文件（供 setup 使用）
func SaveDefault() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	p, err := Path()
	if err != nil {
		return err
	}
	return os.WriteFile(p, defaultYAML, 0600)
}

// Validate 全面验证配置
func Validate(cfg *Config) error {
	var errs []string

	// 1. API Key 非空（必填）
	if cfg.APIKey == "" {
		errs = append(errs, "api_key 不能为空")
	} else if !strings.HasPrefix(cfg.APIKey, "sk-") && !strings.HasPrefix(cfg.APIKey, "org-") {
		// 非强制，仅警告：常见 OpenAI/兼容 key 格式提醒
		errs = append(errs, "警告: api_key 格式可能不正确（应以 sk- 或 org- 开头），请检查")
	}

	// 2. BaseURL 非空且为有效 URL
	if cfg.BaseURL == "" {
		errs = append(errs, "base_url 不能为空")
	} else if u, err := url.Parse(cfg.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		errs = append(errs, "base_url 不是有效的 HTTP/HTTPS URL")
	}

	// 3. Model 非空
	if cfg.Model == "" {
		errs = append(errs, "model 不能为空")
	} // 可选：检查是否在已知模型列表，但模型名不断更新，仅非空即可

	// 4. Mode 合法
	switch cfg.Mode {
	case "ai", "rules", "permissive":
		// ok
	default:
		errs = append(errs, "mode 必须是 ai/rules/permissive，当前: "+cfg.Mode)
	}

	// 5. 检查白名单有无重复（自动去重即可，但也可提示）
	seen := make(map[string]bool)
	for _, w := range cfg.Whitelist {
		if seen[w] {
			errs = append(errs, "白名单中存在重复命令: "+w)
		}
		seen[w] = true
	}

	// 6. Debug 字段是 bool，无需检查
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

// promptRetry 处理验证失败后的重试选择（单键输入，无需回车）
// 返回: "retry"=重新编辑, "cancel"=放弃, "force"=强制保存
func promptRetry(validationErr error, configPath string, backup []byte) string {
	fd := int(os.Stdin.Fd())
	oldState, rawErr := term.MakeRaw(fd) // 改用 rawErr，不再遮蔽 validationErr
	if rawErr != nil {
		// 非终端环境 fallback（需要回车确认）
		fmt.Fprintf(os.Stderr, "错误: %v\n", validationErr)
		fmt.Fprintf(os.Stderr, "[e]重新编辑 / [x]放弃 / [f]强制保存 ")
		var choice string
		fmt.Scanln(&choice)
		if len(choice) > 0 {
			switch choice[0] {
			case 'x', 'X':
				if len(backup) > 0 {
					os.WriteFile(configPath, backup, 0600)
				}
				return "cancel"
			case 'f', 'F':
				return "force"
			}
		}
		return "retry"
	}
	defer term.Restore(fd, oldState)
	// 退出时回到行首清空该行,然后回到上一行清空该行
	defer fmt.Fprintf(os.Stderr, "\r\033[K\033[1A\r\033[K")

	// raw 模式：\r\033[K 先回到行首并清除行尾，再打印错误，确保旧内容被完全覆盖
	fmt.Fprintf(os.Stderr, "\r\033[K[e]重新编辑 / [x]放弃 / [f]强制保存 \r\n")
	fmt.Fprintf(os.Stderr, "-> 错误: %v", validationErr)

	for {
		buf := make([]byte, 1)
		os.Stdin.Read(buf)

		switch buf[0] {
		case 'x', 'X':
			if len(backup) > 0 {
				os.WriteFile(configPath, backup, 0600)
			}
			return "cancel"
		case 'f', 'F':
			return "force"
		case 'e', 'E', '\r', '\n':
			return "retry"
		default:
			log.Bell() // 只响铃，不输出任何字符
		}
	}
}

// Setup 交互式配置引导
func Setup(originalArgs []string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	configPath, err := Path()
	if err != nil {
		return err
	}

	if !Exists() {
		if err := SaveDefault(); err != nil {
			return err
		}
	}

	// 备份旧配置内容
	var backup []byte
	data, err := os.ReadFile(configPath)
	if err == nil {
		backup = data
		data, _ = checkAndMigrateConfig(configPath, data)
		// 同步更新备份，以便编辑器加载的就是新内容
		backup = data
	}

EDITOR:
	for {
		cmd := exec.Command(getEditor(), configPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("打开编辑器失败: %w", err)
		}

		cfg, err := Load()
		if err != nil {
			result := promptRetry(err, configPath, backup)
			switch result {
			case "retry":
				goto EDITOR
			case "cancel":
				return fmt.Errorf("放弃配置")
			case "force":
				return fmt.Errorf("已保存但配置无效: %v", err)
			}
		}

		if err := Validate(cfg); err != nil {
			result := promptRetry(err, configPath, backup)
			switch result {
			case "retry":
				goto EDITOR
			case "cancel":
				return fmt.Errorf("放弃配置")
			case "force":
				return fmt.Errorf("已保存但配置无效: %v", err)
			}
		}

		// 对比备份判断是否实际修改了配置
		if current, err := os.ReadFile(configPath); err == nil && string(current) == string(backup) {
			log.Print("-> 配置未修改")
		} else {
			log.Print("-> 配置已保存")
		}
		break
	}

	if len(originalArgs) > 0 {
		log.Print("继续执行原始命令...")
	}

	return nil
}

// AddToWhitelist 添加命令到白名单（去重后追加）
func (cfg *Config) AddToWhitelist(baseName string) {
	if slices.Contains(cfg.Whitelist, baseName) {
			return
		}
	cfg.Whitelist = append(cfg.Whitelist, baseName)
	if err := Save(cfg); err != nil {
		log.Warn("保存白名单失败: %v", err)
	}
}

// IsWhitelisted 检查命令是否在白名单中
func (cfg *Config) IsWhitelisted(baseName string) bool {
	for _, name := range cfg.Whitelist {
		if name == baseName {
			return true
		}
	}
	return false
}