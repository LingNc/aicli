package config

import (
	_ "embed"
	"fmt"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
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

// Load 加载配置文件，不存在则返回默认配置
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

	// 填充默认值（处理 struct 级零值，如 RequestBody nil）
	fillDefaults(cfg)
	return cfg, nil
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

// updateConfigFile 以 default.yaml 为模板合并用户已有值，结构和注释来自 default.yaml。
// 在内存中合并，最后一次性写入文件。返回 error 由调用方处理。
// forbidden_patterns 和 dangerous_patterns 去重合并，其他 slice 保持替换。
func updateConfigFile(configPath string, userData []byte) error {
	// 1. 备份原文件
	backupPath := configPath + ".bak"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("备份配置文件失败: %w", err)
	}

	// 2. 在内存中解析 default 模板和用户值
	var defMap map[string]any
	if err := yaml.Unmarshal(defaultYAML, &defMap); err != nil {
		return fmt.Errorf("解析默认配置失败: %w", err)
	}

	var userMap map[string]any
	if err := yaml.Unmarshal(userData, &userMap); err != nil {
		return fmt.Errorf("解析用户配置失败: %w", err)
	}

	// 3. 合并
	sliceMergeKeys := map[string]bool{
		"forbidden_patterns": true,
		"dangerous_patterns": true,
	}
	for k, userVal := range userMap {
		if k == "config_version" {
			continue // 版本号始终用 default 的
		}
		if k == "request_body" {
			continue // 请求体由用户自行管理，不自动补充
		}
		defVal, exists := defMap[k]
		if !exists {
			// defMap 中没有的 key 保留下来
			defMap[k] = userVal
			continue
		}
		// 两个都是 map 则深度合并
		if uMap, ok := userVal.(map[string]any); ok {
			if dMap, ok := defVal.(map[string]any); ok {
				defMap[k] = deepMergeMap(dMap, uMap)
				continue
			}
		}
		// 去重合并 slice
		if sliceMergeKeys[k] {
			if uSlice, ok := userVal.([]any); ok {
				if dSlice, ok := defVal.([]any); ok {
					defMap[k] = mergeSliceDedup(dSlice, uSlice)
					continue
				}
			}
		}
		defMap[k] = userVal
	}

	// 4. 写入文件
	merged, err := yaml.Marshal(defMap)
	if err != nil {
		return fmt.Errorf("序列化合并配置失败: %w", err)
	}
	if err := os.WriteFile(configPath, merged, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// mergeSliceDedup 合并两个 []any 字符串 slice 并去重
func mergeSliceDedup(base, extra []any) []any {
	seen := make(map[string]bool, len(base)+len(extra))
	result := make([]any, 0, len(base)+len(extra))
	for _, v := range base {
		if s, ok := v.(string); ok {
			if seen[s] {
				continue
			}
			seen[s] = true
		}
		result = append(result, v)
	}
	for _, v := range extra {
		if s, ok := v.(string); ok {
			if seen[s] {
				continue
			}
			seen[s] = true
		}
		result = append(result, v)
	}
	return result
}

// deepMergeMap 深度合并：base 的结构保留，user 的值覆盖
func deepMergeMap(base, user map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	maps.Copy(result, base)
	for k, userVal := range user {
		if baseVal, exists := result[k]; exists {
			if baseMap, ok := baseVal.(map[string]any); ok {
				if userMap, ok := userVal.(map[string]any); ok {
					result[k] = deepMergeMap(baseMap, userMap)
					continue
				}
			}
		}
		result[k] = userVal
	}
	return result
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
	if data, err := os.ReadFile(configPath); err == nil {
		backup = data
	}

	// 检查配置版本，版本不同则提示用户在打开编辑器前更新配置文件
	if data, err := os.ReadFile(configPath); err == nil {
		var userCfg Config
		if err := yaml.Unmarshal(data, &userCfg); err == nil {
			var defCfg Config
			yaml.Unmarshal(defaultYAML, &defCfg)
			if userCfg.ConfigVersion < defCfg.ConfigVersion {
				if promptConfigUpdate(configPath, userCfg.ConfigVersion, defCfg.ConfigVersion) {
					if err := updateConfigFile(configPath, data); err != nil {
						log.Warn("更新配置文件失败: %v", err)
					} else {
						// 同步更新备份，以便编辑器加载的就是新内容
						backup = nil
						if newData, err := os.ReadFile(configPath); err == nil {
							backup = newData
						}
					}
				}
			}
		}
	}

	// 选择编辑器
	editor := "vi"
	if e := os.Getenv("EDITOR"); e != "" {
		editor = e
	} else if v := os.Getenv("VISUAL"); v != "" {
		editor = v
	}

EDITOR:
	for {
		cmd := exec.Command(editor, configPath)
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
				return nil
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
				return nil
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