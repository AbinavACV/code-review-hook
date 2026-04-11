package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseURL == "" {
		t.Error("base_url should have a default")
	}
	if cfg.Model == "" {
		t.Error("model should have a default")
	}
	if cfg.MaxDiffLines <= 0 {
		t.Error("max_diff_lines should be positive")
	}
	if cfg.SeverityThreshold == "" {
		t.Error("severity_threshold should have a default")
	}
	if cfg.TimeoutSeconds <= 0 {
		t.Error("timeout_seconds should be positive")
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Errorf("missing config file should not error: %v", err)
	}
	defaults := DefaultConfig()
	if cfg.Model != defaults.Model {
		t.Error("should use default model")
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	yaml := []byte("model: gpt-4o\nmax_diff_lines: 200\nbase_url: https://my-gateway.example.com/v1\n")
	os.WriteFile(filepath.Join(tmpDir, ".code-review-hook.yaml"), yaml, 0644)

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", cfg.Model)
	}
	if cfg.MaxDiffLines != 200 {
		t.Errorf("expected 200, got %d", cfg.MaxDiffLines)
	}
	if cfg.BaseURL != "https://my-gateway.example.com/v1" {
		t.Errorf("expected custom base_url, got %s", cfg.BaseURL)
	}
	if cfg.TimeoutSeconds != DefaultConfig().TimeoutSeconds {
		t.Error("non-overridden field should remain default")
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}

	bad := DefaultConfig()
	bad.SeverityThreshold = "critical"
	if err := bad.Validate(); err == nil {
		t.Error("invalid severity_threshold should fail validation")
	}

	bad = DefaultConfig()
	bad.MaxDiffLines = 10
	if err := bad.Validate(); err == nil {
		t.Error("max_diff_lines below 50 should fail validation")
	}
}

func TestShouldExcludeFile(t *testing.T) {
	patterns := []string{"*.lock", "go.sum", "vendor/**"}

	if !ShouldExcludeFile("yarn.lock", patterns) {
		t.Error("*.lock should match yarn.lock")
	}
	if ShouldExcludeFile("package-lock.json", patterns) {
		t.Error("*.lock should NOT match package-lock.json (different extension)")
	}
	if !ShouldExcludeFile("go.sum", patterns) {
		t.Error("go.sum should be excluded")
	}
	if ShouldExcludeFile("main.go", patterns) {
		t.Error("main.go should not be excluded")
	}
}

func TestResolveAPIKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIKey = "from-config"

	// Config fallback
	os.Unsetenv("LLM_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	if got := cfg.ResolveAPIKey(); got != "from-config" {
		t.Errorf("expected from-config, got %s", got)
	}

	// OPENAI_API_KEY takes precedence over config
	os.Setenv("OPENAI_API_KEY", "from-openai")
	defer os.Unsetenv("OPENAI_API_KEY")
	if got := cfg.ResolveAPIKey(); got != "from-openai" {
		t.Errorf("expected from-openai, got %s", got)
	}

	// LLM_API_KEY takes precedence over everything
	os.Setenv("LLM_API_KEY", "from-llm")
	defer os.Unsetenv("LLM_API_KEY")
	if got := cfg.ResolveAPIKey(); got != "from-llm" {
		t.Errorf("expected from-llm, got %s", got)
	}
}
