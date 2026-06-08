package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected string
	}{
		{
			name:     "single line error",
			input:    errors.New("this is an error"),
			expected: "this is an error",
		},
		{
			name:     "multi line error with newlines",
			input:    errors.New("line 1\nline 2\nline 3"),
			expected: "line 1 line 2 line 3",
		},
		{
			name:     "multi line error with carriage returns",
			input:    errors.New("line 1\r\nline 2\r\nline 3"),
			expected: "line 1 line 2 line 3",
		},
		{
			name:     "extra spaces",
			input:    errors.New("line 1  \n  line 2 \n\n line 3"),
			expected: "line 1 line 2 line 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatError(tt.input)
			if result != tt.expected {
				t.Errorf("FormatError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetEditor(t *testing.T) {
	t.Run("EDITOR set", func(t *testing.T) {
		t.Setenv("EDITOR", "vim")
		t.Setenv("VISUAL", "nano")
		if got := GetEditor(); got != "vim" {
			t.Errorf("GetEditor() = %v, want %v", got, "vim")
		}
	})

	t.Run("VISUAL set", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		t.Setenv("VISUAL", "nano")
		if got := GetEditor(); got != "nano" {
			t.Errorf("GetEditor() = %v, want %v", got, "nano")
		}
	})

	t.Run("neither set", func(t *testing.T) {
		t.Setenv("EDITOR", "")
		t.Setenv("VISUAL", "")
		if got := GetEditor(); got != "vi" {
			t.Errorf("GetEditor() = %v, want %v", got, "vi")
		}
	})
}

func TestResolveDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get user home dir: %v", err)
	}

	tests := []struct {
		name     string
		logDir   string
		expected string
	}{
		{
			name:     "empty dir",
			logDir:   "",
			expected: filepath.Join(home, ".aicli", "logs"),
		},
		{
			name:     "starts with ~",
			logDir:   "~/test/logs",
			expected: filepath.Join(home, "/test/logs"),
		},
		{
			name:     "absolute path",
			logDir:   "/tmp/aicli/logs",
			expected: "/tmp/aicli/logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveDir(tt.logDir)
			if got != tt.expected {
				t.Errorf("ResolveDir() = %v, want %v", got, tt.expected)
			}
		})
	}
}
