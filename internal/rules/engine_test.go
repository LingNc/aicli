package rules

import (
	"testing"

	"github.com/lingnc/aicli/internal/config"
	"github.com/lingnc/aicli/internal/executor"
)

func TestRuleMatch(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		matchType MatchType
		command   string
		want      bool
	}{
		{"prefix exact", "rm -rf /", MatchPrefix, "rm -rf /", true},
		{"prefix match", "rm -rf /", MatchPrefix, "rm -rf /tmp", true},
		{"prefix no match", "rm -rf /", MatchPrefix, "rm -rf ./tmp", false},
		{"contains match", "dd if=", MatchContains, "dd if=/dev/zero of=/dev/sda", true},
		{"contains match mid", ">", MatchContains, "cat file | grep foo > output.txt", true},
		{"contains no match", "dd if=", MatchContains, "ls -la", false},
		{"trim space", "ls", MatchPrefix, "  ls -la", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rule{Pattern: tt.pattern, MatchType: tt.matchType}
			if got := r.Match(tt.command); got != tt.want {
				t.Errorf("Rule.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHardcodedForbidden(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"rm -rf /", true},
		{"rm -rf /tmp", true},
		{"rm -rf /*", true},
		{"dd if=/dev/zero of=/dev/sda bs=4M", true},
		{"dd if=/dev/urandom of=/dev/sda", true},
		{":(){ :|:& };:", true},
		{"mkfs.ext4 /dev/sdb", true},
		{"fdisk -l /dev/sda", true},
		{"chmod -R 777 /", true},
		{"chown -R root /", false},
		// not matched
		{"ls -la", false},
		{"echo hello", false},
	}

	e := NewEngine(&config.Config{})
	for _, c := range cases {
		t.Run(c.cmd, func(t *testing.T) {
			matched := false
			for _, r := range e.forbidden {
				if r.Match(c.cmd) {
					matched = true
					break
				}
			}
			if matched != c.want {
				t.Errorf("forbidden match(%q) = %v, want %v", c.cmd, matched, c.want)
			}
		})
	}
}

func TestHardcodedDangerous(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"rm -rf ./tmp", true},
		{"dd if=/dev/zero of=file.img bs=1M", true},
		{"mkfs.ext4 /dev/sdb", true},
		{"fdisk -l /dev/sda", true},
		{"chmod -R 755 /home/user", true},
		{"chown -R user:user /home/user", true},
		{"cat file > output.txt", true},
		{"find . -name '*.tmp' | xargs rm", true},
		// not matched
		{"ls -la", false},
		{"echo hello", false},
	}

	e := NewEngine(&config.Config{})
	for _, c := range cases {
		t.Run(c.cmd, func(t *testing.T) {
			matched := false
			for _, r := range e.dangerous {
				if r.Match(c.cmd) {
					matched = true
					break
				}
			}
			if matched != c.want {
				t.Errorf("dangerous match(%q) = %v, want %v", c.cmd, matched, c.want)
			}
		})
	}
}

func TestPriority(t *testing.T) {
	// rm -rf / should match forbidden (not dangerous)
	e := NewEngine(&config.Config{})
	verdict := e.Classify("rm -rf /", "ai", executor.CatUnknown)
	if verdict != VerdictForbidden {
		t.Errorf("rm -rf / should be VerdictForbidden, got %v", verdict)
	}
}

func TestClassify_PermissiveMode(t *testing.T) {
	e := NewEngine(&config.Config{})
	verdict := e.Classify("ls -la", "permissive", executor.CatUnknown)
	if verdict != VerdictSafe {
		t.Errorf("permissive mode should return VerdictSafe, got %v", verdict)
	}
}

func TestClassify_RulesMode(t *testing.T) {
	cfg := &config.Config{
		ReadonlyCommands: []string{"ls", "cat", "ps"},
	}
	e := NewEngine(cfg)

	// readonly 命令应返回 Safe
	verdict := e.Classify("ls -la", "rules", executor.CatUnknown)
	if verdict != VerdictSafe {
		t.Errorf("ls should be VerdictSafe in rules mode, got %v", verdict)
	}

	// 非 readonly 命令应返回 Dangerous
	verdict = e.Classify("rm -rf ./tmp", "rules", executor.CatUnknown)
	if verdict != VerdictDangerous {
		t.Errorf("rm -rf ./tmp should be VerdictDangerous in rules mode, got %v", verdict)
	}
}

func TestClassify_AIMode(t *testing.T) {
	e := NewEngine(&config.Config{})

	// CatRO 应返回 Safe
	verdict := e.Classify("ls -la", "ai", executor.CatRO)
	if verdict != VerdictSafe {
		t.Errorf("CatRO should be VerdictSafe, got %v", verdict)
	}

	// CatSudoRO 应返回 Safe
	verdict = e.Classify("ls -la", "ai", executor.CatSudoRO)
	if verdict != VerdictSafe {
		t.Errorf("CatSudoRO should be VerdictSafe, got %v", verdict)
	}

	// 其他类别应返回 Dangerous
	verdict = e.Classify("ls -la", "ai", executor.CatUnknown)
	if verdict != VerdictDangerous {
		t.Errorf("unknown category should be VerdictDangerous, got %v", verdict)
	}
}

func TestClassify_Whitelist(t *testing.T) {
	cfg := &config.Config{
		Whitelist: []string{"git", "npm"},
	}
	e := NewEngine(cfg)

	// 白名单命令在 ai 模式下直接执行
	verdict := e.Classify("git status", "ai", executor.CatUnknown)
	if verdict != VerdictSafe {
		t.Errorf("whitelisted git should be VerdictSafe, got %v", verdict)
	}

	verdict = e.Classify("npm install", "ai", executor.CatUnknown)
	if verdict != VerdictSafe {
		t.Errorf("whitelisted npm should be VerdictSafe, got %v", verdict)
	}

	// 白名单在 rules 模式也生效
	verdict = e.Classify("git push origin main", "rules", executor.CatUnknown)
	if verdict != VerdictSafe {
		t.Errorf("whitelisted git in rules mode should be VerdictSafe, got %v", verdict)
	}

	// 非白名单命令在 rules 模式需要确认
	verdict = e.Classify("ls -la", "rules", executor.CatUnknown)
	if verdict != VerdictDangerous {
		t.Errorf("non-whitelisted command in rules mode should be VerdictDangerous, got %v", verdict)
	}
}

func TestUserPatterns(t *testing.T) {
	cfg := &config.Config{
		ForbiddenPatterns: []string{"shutdown", "reboot"},
		DangerousPatterns: []string{"git push --force", "docker rm -f"},
	}
	e := NewEngine(cfg)

	// 用户定义的 forbidden 规则生效
	verdict := e.Classify("shutdown -h now", "ai", executor.CatUnknown)
	if verdict != VerdictForbidden {
		t.Errorf("user forbidden pattern 'shutdown' should be VerdictForbidden, got %v", verdict)
	}

	// 用户定义的 dangerous 规则生效
	verdict = e.Classify("git push --force origin main", "ai", executor.CatUnknown)
	if verdict != VerdictDangerous {
		t.Errorf("user dangerous pattern 'git push --force' should be VerdictDangerous, got %v", verdict)
	}
}

func TestForbiddenReason(t *testing.T) {
	e := NewEngine(&config.Config{})

	reason := e.ForbiddenReason("rm -rf /")
	if reason == "" {
		t.Error("ForbiddenReason should return non-empty string for forbidden command")
	}

	reason = e.ForbiddenReason("ls -la")
	if reason != "" {
		t.Errorf("ForbiddenReason should return empty string for safe command, got %q", reason)
	}
}

func TestEdgeCases(t *testing.T) {
	e := NewEngine(&config.Config{})

	// 空命令
	verdict := e.Classify("", "ai", executor.CatUnknown)
	if verdict != VerdictDangerous {
		t.Errorf("empty command should be VerdictDangerous, got %v", verdict)
	}

	// 纯空格命令
	verdict = e.Classify("   ", "ai", executor.CatUnknown)
	if verdict != VerdictDangerous {
		t.Errorf("whitespace command should be VerdictDangerous, got %v", verdict)
	}

	// 前导空格命令
	verdict = e.Classify("  ls -la", "ai", executor.CatUnknown)
	if verdict != VerdictDangerous {
		t.Errorf("command with leading space should be VerdictDangerous (not in whitelist), got %v", verdict)
	}
}
