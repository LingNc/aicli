package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lingnc/aicli/internal/log"
	"gopkg.in/yaml.v3"
)

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
	templateKeys := make(map[string]bool) // 记录模板中所有顶层 key

	// 解析用户的 request_body 子键值
	var userRB map[string]any
	if rb, ok := userMap["request_body"].(map[string]any); ok {
		userRB = rb
	}

	inRequestBody := false // 当前是否在 request_body 块内
	requestBodyIndent := 0 // request_body 子行的缩进层级
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}

		// 检测进入/退出 request_body 块
		if len(line) > 0 && (line[0] != ' ' && line[0] != '\t') {
			inRequestBody = false // 新的顶层 key，退出 request_body 块
		}

		// 只处理顶层 key
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			// 非顶层行：如果在 request_body 块内，尝试替换子键值
			if inRequestBody && userRB != nil {
				// 检查缩进是否还在 request_body 内
				lineIndent := len(line) - len(strings.TrimLeft(line, " \t"))
				if lineIndent <= requestBodyIndent {
					inRequestBody = false
				} else {
					// 尝试提取子键并替换
					subIdx := strings.Index(trimmed, ":")
					if subIdx > 0 {
						subKey := strings.TrimSpace(trimmed[:subIdx])
						if subVal, ok := userRB[subKey]; ok {
							if _, isMap := subVal.(map[string]any); !isMap {
								// 标量子键：替换值，保留注释
								subIndent := line[:lineIndent]
								comment := ""
								ci := strings.Index(trimmed, " #")
								if ci > subIdx {
									comment = trimmed[ci:]
								}
								lines[i] = subIndent + subKey + ": " + formatYAMLScalar(subVal) + comment
							}
							// 嵌套 map（如 extra_body）保留模板结构
						}
					}
				}
			}
			continue
		}
		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		templateKeys[key] = true

		// 标记进入 request_body 块
		if key == "request_body" {
			inRequestBody = true
			requestBodyIndent = len(line) - len(strings.TrimLeft(line, " \t")) // request_body 自身缩进
			continue
		}

		// 跳过 config_version 和复杂字段（slice、嵌套 map）
		if key == "config_version" {
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
		if replaced[k] || templateKeys[k] {
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
