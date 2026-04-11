package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the code review hook.
type Config struct {
	BaseURL             string   `yaml:"base_url"`
	APIKey              string   `yaml:"api_key"`
	Model               string   `yaml:"model"`
	MaxDiffLines        int      `yaml:"max_diff_lines"`
	SeverityThreshold   string   `yaml:"severity_threshold"`
	FileExcludePatterns []string `yaml:"file_exclude_patterns"`
	CustomPrompt        string   `yaml:"custom_prompt"`
	FailOnWarning       bool     `yaml:"fail_on_warning"`
	TimeoutSeconds      int      `yaml:"timeout_seconds"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:          "https://api.openai.com/v1",
		Model:            "gpt-4o-mini",
		MaxDiffLines:     500,
		SeverityThreshold: "error",
		FileExcludePatterns: []string{
			"*.lock",
			"go.sum",
			"*.pb.go",
			"vendor/**",
		},
		TimeoutSeconds: 30,
	}
}

// LoadConfig reads .code-review-hook.yaml from repoRoot, overlays onto defaults,
// then applies environment variable overrides.
func LoadConfig(repoRoot string) (Config, error) {
	cfg := DefaultConfig()

	path := filepath.Join(repoRoot, ".code-review-hook.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		applyEnvOverrides(&cfg)
		applyFailOnWarning(&cfg)
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}

	applyEnvOverrides(&cfg)
	applyFailOnWarning(&cfg)
	return cfg, nil
}

// applyFailOnWarning overrides severity_threshold when fail_on_warning is set.
func applyFailOnWarning(cfg *Config) {
	if cfg.FailOnWarning {
		cfg.SeverityThreshold = "warning"
	}
}

// applyEnvOverrides applies environment variable overrides to the config.
// Resolution order: env var > config file > default.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.Model = v
	}
	// API key is resolved separately via ResolveAPIKey.
}

// ResolveAPIKey returns the API key from the resolution chain:
// LLM_API_KEY env var > OPENAI_API_KEY env var > config file value.
func (c Config) ResolveAPIKey() string {
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		return v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		return v
	}
	return c.APIKey
}

// Validate checks that config values are within acceptable ranges.
func (c Config) Validate() error {
	validSeverities := map[string]bool{"error": true, "warning": true, "info": true}
	if !validSeverities[c.SeverityThreshold] {
		return fmt.Errorf("severity_threshold must be one of: error, warning, info")
	}
	if c.MaxDiffLines < 50 {
		return fmt.Errorf("max_diff_lines must be at least 50")
	}
	if c.TimeoutSeconds < 5 || c.TimeoutSeconds > 120 {
		return fmt.Errorf("timeout_seconds must be between 5 and 120")
	}
	return nil
}

// ShouldExcludeFile returns true if the file path matches any of the exclude patterns.
// Supports ** for recursive directory matching (e.g., "vendor/**").
func ShouldExcludeFile(path string, patterns []string) bool {
	baseName := filepath.Base(path)
	for _, pattern := range patterns {
		// Handle ** patterns as prefix matching (e.g., "vendor/**" matches "vendor/lib.go")
		if strings.Contains(pattern, "**") {
			prefix := strings.SplitN(pattern, "**", 2)[0]
			if strings.HasPrefix(path, prefix) {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(pattern, baseName); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}
