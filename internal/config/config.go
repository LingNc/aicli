package config

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultYAML []byte

// Config 是应用配置结构体
type Config struct {
	APIKey            string   `yaml:"api_key"`
	BaseURL           string   `yaml:"base_url"`
	Model             string   `yaml:"model"`
	Mode              string   `yaml:"mode"`
	Debug             bool     `yaml:"debug"`
	Whitelist         []string `yaml:"whitelist"`
	ForbiddenPatterns []string `yaml:"forbidden_patterns"` // 用户扩展的禁止命令
	DangerousPatterns []string `yaml:"dangerous_patterns"` // 用户扩展的需确认命令
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

	// 填充默认值
	fillDefaults(cfg)
	return cfg, nil
}

// loadDefault 从嵌入的默认配置加载
func loadDefault() (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultYAML, cfg); err != nil {
		return nil, fmt.Errorf("解析默认配置失败: %w", err)
	}
	return cfg, nil
}

// fillDefaults 用默认值填充空字段
func fillDefaults(cfg *Config) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.Mode == "" {
		cfg.Mode = "ai"
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
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultYAML, cfg); err != nil {
		return fmt.Errorf("解析默认配置失败: %w", err)
	}
	return Save(cfg)
}

// Validate 验证配置必填字段
func Validate(cfg *Config) error {
	if cfg.APIKey == "" {
		return fmt.Errorf("api_key 不能为空")
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("base_url 不能为空")
	}
	if cfg.Model == "" {
		return fmt.Errorf("model 不能为空")
	}
	switch cfg.Mode {
	case "ai", "rules", "permissive":
		// 合法
	default:
		return fmt.Errorf("mode 必须是 ai/rules/permissive，当前: %s", cfg.Mode)
	}
	return nil
}

// promptRetry 处理验证失败后的重试选择
// 返回: "retry"=重新编辑, "cancel"=放弃, "force"=强制保存
func promptRetry(err error, configPath string, backup []byte) string {
	for {
		fmt.Printf("错误: %v\n", err)
		fmt.Println("[e]重新编辑 / [x]放弃 / [f]强制保存")
		var choice string
		fmt.Scanln(&choice)
		switch choice {
		case "e":
			return "retry"
		case "x":
			if len(backup) > 0 {
				os.WriteFile(configPath, backup, 0600)
			}
			return "cancel"
		case "f":
			return "force"
		default:
			fmt.Println("请输入 e、x 或 f")
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

	// 选择编辑器
	editor := "vi"
	if e := os.Getenv("EDITOR"); e != "" {
		editor = e
	} else if v := os.Getenv("VISUAL"); v != "" {
		editor = v
	}

EDITOR:
	for {
		if err := exec.Command(editor, configPath).Run(); err != nil {
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

		fmt.Println("-> 配置已保存")
		break
	}

	if len(originalArgs) > 0 {
		fmt.Println("继续执行原始命令...")
	}

	return nil
}

// AddToWhitelist 添加命令到白名单（去重后追加）
func (cfg *Config) AddToWhitelist(baseName string) {
	for _, name := range cfg.Whitelist {
		if name == baseName {
			return
		}
	}
	cfg.Whitelist = append(cfg.Whitelist, baseName)
	if err := Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 保存白名单失败: %v\n", err)
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