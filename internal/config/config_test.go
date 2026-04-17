package config

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
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
	if !cfg.SaveComments {
		t.Error("save_comments should default to true")
	}
	if cfg.CommentsDir != "comments" {
		t.Errorf("comments_dir default: got %q want comments", cfg.CommentsDir)
	}
}

func TestLoadCommentsOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	yaml := []byte("save_comments: false\ncomments_dir: .ai-reviews\n")
	os.WriteFile(filepath.Join(tmpDir, ".code-review-hook.yaml"), yaml, 0644)

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SaveComments {
		t.Error("save_comments should be false from YAML")
	}
	if cfg.CommentsDir != ".ai-reviews" {
		t.Errorf("comments_dir: got %q want .ai-reviews", cfg.CommentsDir)
	}
}

func TestParseFlags_CommentsFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	flags, err := ParseFlags(fs, []string{"--save-comments=false", "--comments-dir=foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.SaveComments == nil || *flags.SaveComments != false {
		t.Error("expected SaveComments=false")
	}
	if flags.CommentsDir == nil || *flags.CommentsDir != "foo" {
		t.Error("expected CommentsDir=foo")
	}
}

func TestApplyFlags_Comments(t *testing.T) {
	cfg := Default()
	save := false
	dir := "x"
	ApplyFlags(&cfg, Flags{SaveComments: &save, CommentsDir: &dir})
	if cfg.SaveComments {
		t.Error("expected SaveComments=false after ApplyFlags")
	}
	if cfg.CommentsDir != "x" {
		t.Errorf("expected CommentsDir=x, got %q", cfg.CommentsDir)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Errorf("missing config file should not error: %v", err)
	}
	defaults := Default()
	if cfg.Model != defaults.Model {
		t.Error("should use default model")
	}
}

func TestLoadOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	yaml := []byte("model: gpt-4o\nmax_diff_lines: 200\nbase_url: https://my-gateway.example.com/v1\n")
	os.WriteFile(filepath.Join(tmpDir, ".code-review-hook.yaml"), yaml, 0644)

	cfg, err := Load(tmpDir)
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
	if cfg.TimeoutSeconds != Default().TimeoutSeconds {
		t.Error("non-overridden field should remain default")
	}
}

func TestValidate(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}

	bad := Default()
	bad.SeverityThreshold = "critical"
	if err := bad.Validate(); err == nil {
		t.Error("invalid severity_threshold should fail validation")
	}

	bad = Default()
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
	if !ShouldExcludeFile("vendor/lib.go", patterns) {
		t.Error("vendor/** should match vendor/lib.go")
	}
	if !ShouldExcludeFile("vendor/sub/deep.go", patterns) {
		t.Error("vendor/** should match vendor/sub/deep.go")
	}
}

func TestFailOnWarningOverridesSeverity(t *testing.T) {
	tmpDir := t.TempDir()
	yaml := []byte("fail_on_warning: true\n")
	os.WriteFile(filepath.Join(tmpDir, ".code-review-hook.yaml"), yaml, 0644)

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SeverityThreshold != "warning" {
		t.Errorf("expected severity_threshold to be overridden to 'warning', got %s", cfg.SeverityThreshold)
	}
}

func TestCLIFlagsOverrideYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yaml := []byte("model: gpt-4o-mini\nseverity_threshold: error\n")
	os.WriteFile(filepath.Join(tmpDir, ".code-review-hook.yaml"), yaml, 0644)

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	model := "gpt-4o"
	severity := "warning"
	ApplyFlags(&cfg, Flags{Model: &model, SeverityThreshold: &severity})

	if cfg.Model != "gpt-4o" {
		t.Errorf("CLI --model not applied, got %s", cfg.Model)
	}
	if cfg.SeverityThreshold != "warning" {
		t.Errorf("CLI --severity-threshold not applied, got %s", cfg.SeverityThreshold)
	}
}

func TestApplyFlags_NilFieldsUnchanged(t *testing.T) {
	cfg := Default()
	original := cfg.Model
	ApplyFlags(&cfg, Flags{})
	if cfg.Model != original {
		t.Error("nil Flags should leave config unchanged")
	}
}

func TestParseFlags_ExplicitFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	flags, err := ParseFlags(fs, []string{"--model=gpt-4o", "--severity-threshold=warning"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.Model == nil || *flags.Model != "gpt-4o" {
		t.Error("expected Model to be set to gpt-4o")
	}
	if flags.SeverityThreshold == nil || *flags.SeverityThreshold != "warning" {
		t.Error("expected SeverityThreshold to be set to warning")
	}
	if flags.TimeoutSeconds != nil {
		t.Error("unprovided flag TimeoutSeconds should be nil")
	}
}

func TestParseFlags_NoFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	flags, err := ParseFlags(fs, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.Model != nil || flags.SeverityThreshold != nil || flags.TimeoutSeconds != nil {
		t.Error("no flags passed — all pointer fields should be nil")
	}
}

func TestValidateTimeoutBounds(t *testing.T) {
	tooLow := Default()
	tooLow.TimeoutSeconds = 4
	if err := tooLow.Validate(); err == nil {
		t.Error("timeout_seconds below 5 should fail validation")
	}

	tooHigh := Default()
	tooHigh.TimeoutSeconds = 121
	if err := tooHigh.Validate(); err == nil {
		t.Error("timeout_seconds above 120 should fail validation")
	}
}

func TestLoadRules_FileExists(t *testing.T) {
	tmpDir := t.TempDir()
	rules := "  Do not use eval().  \n"
	os.WriteFile(filepath.Join(tmpDir, "rules.md"), []byte(rules), 0644)

	cfg := Default()
	cfg.RulesFile = "rules.md"
	LoadRules(tmpDir, &cfg)
	if cfg.RulesContent != "Do not use eval()." {
		t.Errorf("expected trimmed content, got %q", cfg.RulesContent)
	}
}

func TestLoadRules_FileMissing(t *testing.T) {
	cfg := Default()
	cfg.RulesFile = "nonexistent.md"
	LoadRules(t.TempDir(), &cfg)
	if cfg.RulesContent != "" {
		t.Error("missing rules file should leave RulesContent empty")
	}
}

func TestLoadRules_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "rules.md"), []byte("   \n  "), 0644)

	cfg := Default()
	cfg.RulesFile = "rules.md"
	LoadRules(tmpDir, &cfg)
	if cfg.RulesContent != "" {
		t.Error("empty/whitespace-only rules file should produce empty RulesContent")
	}
}

func TestLoadRules_NotSet(t *testing.T) {
	cfg := Default()
	cfg.RulesContent = "should remain"
	LoadRules(t.TempDir(), &cfg)
	if cfg.RulesContent != "should remain" {
		t.Error("LoadRules should no-op when RulesFile is empty")
	}
}

func TestParseFlags_RulesFile(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	flags, err := ParseFlags(fs, []string{"--rules-file=rules.md"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags.RulesFile == nil || *flags.RulesFile != "rules.md" {
		t.Error("expected RulesFile to be set to rules.md")
	}
}

func TestApplyFlags_RulesFile(t *testing.T) {
	cfg := Default()
	rf := "custom-rules.md"
	ApplyFlags(&cfg, Flags{RulesFile: &rf})
	if cfg.RulesFile != "custom-rules.md" {
		t.Errorf("expected RulesFile to be custom-rules.md, got %s", cfg.RulesFile)
	}
}

func TestResolveAPIKey(t *testing.T) {
	cfg := Default()
	cfg.APIKey = "from-config"

	os.Unsetenv("LLM_API_KEY")
	if got := cfg.ResolveAPIKey(); got != "from-config" {
		t.Errorf("expected from-config, got %s", got)
	}

	os.Setenv("LLM_API_KEY", "from-llm")
	defer os.Unsetenv("LLM_API_KEY")
	if got := cfg.ResolveAPIKey(); got != "from-llm" {
		t.Errorf("expected from-llm, got %s", got)
	}
}
