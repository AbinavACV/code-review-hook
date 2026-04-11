package main

import (
	"strings"
	"testing"
)

func TestStripBinaryHunks_NoBinary(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n+ added line\n"
	result := StripBinaryHunks(diff)
	if result != diff {
		t.Error("diff without binary files should be unchanged")
	}
}

func TestStripBinaryHunks_AllBinary(t *testing.T) {
	diff := "diff --git a/image.png b/image.png\nBinary files /dev/null and b/image.png differ"
	result := StripBinaryHunks(diff)
	if strings.TrimSpace(result) != "" {
		t.Errorf("all-binary diff should produce empty result, got: %q", result)
	}
}

func TestStripBinaryHunks_Mixed(t *testing.T) {
	diff := "diff --git a/image.png b/image.png\nBinary files /dev/null and b/image.png differ\ndiff --git a/main.go b/main.go\n+ real code"
	result := StripBinaryHunks(diff)
	if strings.Contains(result, "Binary files") {
		t.Error("binary hunk should have been stripped")
	}
	if !strings.Contains(result, "real code") {
		t.Error("non-binary content should be preserved")
	}
}

func TestFilterExcludedFiles_NoPatterns(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n+ code\n"
	result := FilterExcludedFiles(diff, nil)
	if result != diff {
		t.Error("no patterns should leave diff unchanged")
	}
}

func TestFilterExcludedFiles_ExcludesMatch(t *testing.T) {
	diff := "diff --git a/go.sum b/go.sum\n+ hash\ndiff --git a/main.go b/main.go\n+ code"
	result := FilterExcludedFiles(diff, []string{"go.sum"})
	if strings.Contains(result, "go.sum") {
		t.Error("go.sum should have been excluded")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("main.go should be preserved")
	}
}

func TestFilterExcludedFiles_AllExcluded(t *testing.T) {
	diff := "diff --git a/go.sum b/go.sum\n+ hash"
	result := FilterExcludedFiles(diff, []string{"go.sum"})
	if strings.TrimSpace(result) != "" {
		t.Errorf("fully excluded diff should be empty, got: %q", result)
	}
}

func TestExtractFilename(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"diff --git a/main.go b/main.go", "main.go"},
		{"diff --git a/src/utils.go b/src/utils.go", "src/utils.go"},
		{"diff --git a/old.go b/new.go", "new.go"},
	}
	for _, tt := range tests {
		got := extractFilename(tt.line)
		if got != tt.expected {
			t.Errorf("extractFilename(%q) = %q, want %q", tt.line, got, tt.expected)
		}
	}
}
