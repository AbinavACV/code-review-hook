package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/AbinavACV/code-review-hook/internal/output"
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
	RulesFile           string   `yaml:"rules_file"`
	RulesContent        string   `yaml:"-"`
	FailOnWarning       bool     `yaml:"fail_on_warning"`
	TimeoutSeconds      int      `yaml:"timeout_seconds"`
	SaveComments        bool     `yaml:"save_comments"`
	CommentsDir         string   `yaml:"comments_dir"`
}

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		BaseURL:           "https://api.openai.com/v1",
		Model:             "gpt-4o-mini",
		MaxDiffLines:      500,
		SeverityThreshold: "error",
		FileExcludePatterns: []string{
			"*.lock",
			"go.sum",
			"*.pb.go",
			"vendor/**",
		},
		TimeoutSeconds: 30,
		SaveComments:   true,
		CommentsDir:    "comments",
	}
}

// Load reads .code-review-hook.yaml from repoRoot and overlays onto defaults.
func Load(repoRoot string) (Config, error) {
	cfg := Default()

	path := filepath.Join(repoRoot, ".code-review-hook.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		applyFailOnWarning(&cfg)
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}

	applyFailOnWarning(&cfg)
	return cfg, nil
}

func applyFailOnWarning(cfg *Config) {
	if cfg.FailOnWarning {
		cfg.SeverityThreshold = "warning"
	}
}

// ResolveAPIKey returns the API key from LLM_API_KEY env var or the config file.
func (c Config) ResolveAPIKey() string {
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		return v
	}
	return c.APIKey
}

// Flags holds values parsed from os.Args.
// Pointer fields distinguish "explicitly set" from "not provided".
type Flags struct {
	Model             *string
	SeverityThreshold *string
	TimeoutSeconds    *int
	FailOnWarning     *bool
	BaseURL           *string
	MaxDiffLines      *int
	RulesFile         *string
	SaveComments      *bool
	CommentsDir       *string
}

// ParseFlags parses args using fs and returns only the flags that were
// explicitly set. Use flag.NewFlagSet for testability.
func ParseFlags(fs *flag.FlagSet, args []string) (Flags, error) {
	model := fs.String("model", "", "LLM model name")
	severity := fs.String("severity-threshold", "", "Minimum severity to block: error, warning, or info")
	timeout := fs.Int("timeout", 0, "API timeout in seconds (5–120)")
	failOnWarn := fs.Bool("fail-on-warning", false, "Block commits on warnings (shorthand for --severity-threshold=warning)")
	baseURL := fs.String("base-url", "", "Base URL of any OpenAI-compatible API")
	maxDiff := fs.Int("max-diff-lines", 0, "Truncate diffs longer than N lines (min 50)")
	rulesFile := fs.String("rules-file", "", "Path to a markdown file with team code review rules (relative to repo root)")
	saveComments := fs.Bool("save-comments", true, "Save review comments to a markdown file under <comments-dir>/<branch>.md")
	commentsDir := fs.String("comments-dir", "", "Directory for review comment files (relative to repo root)")

	if err := fs.Parse(args); err != nil {
		return Flags{}, err
	}

	var flags Flags
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "model":
			flags.Model = model
		case "severity-threshold":
			flags.SeverityThreshold = severity
		case "timeout":
			flags.TimeoutSeconds = timeout
		case "fail-on-warning":
			flags.FailOnWarning = failOnWarn
		case "base-url":
			flags.BaseURL = baseURL
		case "max-diff-lines":
			flags.MaxDiffLines = maxDiff
		case "rules-file":
			flags.RulesFile = rulesFile
		case "save-comments":
			flags.SaveComments = saveComments
		case "comments-dir":
			flags.CommentsDir = commentsDir
		}
	})
	return flags, nil
}

// ApplyFlags overlays explicitly-set CLI flags onto cfg.
// Only non-nil pointer fields are applied.
func ApplyFlags(cfg *Config, flags Flags) {
	if flags.Model != nil {
		cfg.Model = *flags.Model
	}
	if flags.SeverityThreshold != nil {
		cfg.SeverityThreshold = *flags.SeverityThreshold
	}
	if flags.TimeoutSeconds != nil {
		cfg.TimeoutSeconds = *flags.TimeoutSeconds
	}
	if flags.FailOnWarning != nil {
		cfg.FailOnWarning = *flags.FailOnWarning
	}
	if flags.BaseURL != nil {
		cfg.BaseURL = *flags.BaseURL
	}
	if flags.MaxDiffLines != nil {
		cfg.MaxDiffLines = *flags.MaxDiffLines
	}
	if flags.RulesFile != nil {
		cfg.RulesFile = *flags.RulesFile
	}
	if flags.SaveComments != nil {
		cfg.SaveComments = *flags.SaveComments
	}
	if flags.CommentsDir != nil {
		cfg.CommentsDir = *flags.CommentsDir
	}
	applyFailOnWarning(cfg)
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

// LoadRules reads the rules file specified by cfg.RulesFile (relative to repoRoot)
// and stores the trimmed content in cfg.RulesContent. Warns and continues if the file
// is missing or unreadable (fail-open).
func LoadRules(repoRoot string, cfg *Config) {
	if cfg.RulesFile == "" {
		return
	}
	path := filepath.Join(repoRoot, cfg.RulesFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		output.PrintWarning("rules_file not found: " + path)
		return
	}
	if err != nil {
		output.PrintWarning("Could not read rules_file: " + err.Error())
		return
	}
	cfg.RulesContent = strings.TrimSpace(string(data))
}

// ShouldExcludeFile returns true if the file path matches any of the exclude patterns.
// Supports ** for recursive directory matching (e.g., "vendor/**").
func ShouldExcludeFile(path string, patterns []string) bool {
	baseName := filepath.Base(path)
	for _, pattern := range patterns {
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
