package comments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AbinavACV/code-review-hook/internal/diff"
	"github.com/AbinavACV/code-review-hook/internal/review"
)

func TestSanitizeBranch(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"main", "main"},
		{"feature/x", "feature-x"},
		{"fix/my branch!", "fix-mybranch"},
		{"release/v2.0.0", "release-v2.0.0"},
		{"feat/AB-12_thing", "feat-AB-12_thing"},
		{"!!!", "unknown"},
		{"", "unknown"},
		{"HEAD", "HEAD"},
	}
	for _, tt := range tests {
		got := SanitizeBranch(tt.in)
		if got != tt.want {
			t.Errorf("SanitizeBranch(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func sampleHunks() []diff.Hunk {
	return []diff.Hunk{
		{File: "main.go", NewStart: 10, NewLen: 5, Body: "@@ -10,4 +10,5 @@\n line\n+added\n line"},
		{File: "main.go", NewStart: 50, NewLen: 3, Body: "@@ -50,2 +50,3 @@\n other\n+more"},
		{File: "util.go", NewStart: 1, NewLen: 4, Body: "@@ -1,3 +1,4 @@\n util\n+stuff"},
	}
}

func TestMatchHunk_Found(t *testing.T) {
	hunks := sampleHunks()
	h := matchHunk("main.go", 12, hunks)
	if h == nil || h.NewStart != 10 {
		t.Errorf("expected first main.go hunk, got %+v", h)
	}
	h = matchHunk("main.go", 51, hunks)
	if h == nil || h.NewStart != 50 {
		t.Errorf("expected second main.go hunk, got %+v", h)
	}
	h = matchHunk("util.go", 4, hunks)
	if h == nil || h.File != "util.go" {
		t.Errorf("expected util.go hunk, got %+v", h)
	}
}

func TestMatchHunk_NotFound(t *testing.T) {
	hunks := sampleHunks()
	if h := matchHunk("main.go", 0, hunks); h != nil {
		t.Errorf("line=0 should return nil, got %+v", h)
	}
	if h := matchHunk("main.go", 100, hunks); h != nil {
		t.Errorf("line out of range should return nil, got %+v", h)
	}
	if h := matchHunk("missing.go", 1, hunks); h != nil {
		t.Errorf("file not in diff should return nil, got %+v", h)
	}
	if h := matchHunk("", 5, hunks); h != nil {
		t.Errorf("empty file should return nil, got %+v", h)
	}
}

func TestFormatMarkdown_WithIssues(t *testing.T) {
	now := time.Date(2026, 4, 17, 14, 32, 5, 0, time.UTC)
	result := &review.ReviewResult{
		Verdict: "request_changes",
		Summary: "Found a bug.",
		Issues: []review.Issue{
			{Severity: "error", File: "main.go", Line: 12, Message: "nil deref"},
		},
	}
	got := formatMarkdown("feature/x", result, sampleHunks(), now)

	wants := []string{
		"# Review for branch `feature/x`",
		"_Generated: 2026-04-17T14:32:05Z — verdict: **request_changes**_",
		"## Summary",
		"Found a bug.",
		"## Issues",
		"### 1. error — `main.go:12`",
		"> nil deref",
		"```diff",
		"+added",
		"```",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("output missing %q\n---\n%s", w, got)
		}
	}
}

func TestFormatMarkdown_NoIssues(t *testing.T) {
	now := time.Date(2026, 4, 17, 14, 32, 5, 0, time.UTC)
	result := &review.ReviewResult{Verdict: "approve", Summary: "All good.", Issues: nil}
	got := formatMarkdown("main", result, nil, now)

	if !strings.Contains(got, "_No issues — verdict: approve._") {
		t.Errorf("missing no-issues marker:\n%s", got)
	}
	if strings.Contains(got, "## Issues") {
		t.Error("should not render Issues section when empty")
	}
}

func TestFormatMarkdown_UnmatchedHunk(t *testing.T) {
	now := time.Date(2026, 4, 17, 14, 32, 5, 0, time.UTC)
	result := &review.ReviewResult{
		Verdict: "request_changes",
		Summary: "Generic.",
		Issues: []review.Issue{
			{Severity: "warning", File: "missing.go", Line: 1, Message: "x"},
			{Severity: "info", File: "", Line: 0, Message: "no location"},
		},
	}
	got := formatMarkdown("main", result, sampleHunks(), now)

	if strings.Contains(got, "```diff") {
		t.Error("should not render diff fence for unmatched issues")
	}
	if !strings.Contains(got, "> x") {
		t.Error("should still render issue message")
	}
	if !strings.Contains(got, "> no location") {
		t.Error("should still render second issue message")
	}
}

func TestWrite_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	result := &review.ReviewResult{Verdict: "approve", Summary: "ok"}
	if err := Write(tmpDir, "comments", "main", result, nil); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	path := filepath.Join(tmpDir, "comments", "main.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
}

func TestWrite_Overwrites(t *testing.T) {
	tmpDir := t.TempDir()

	first := &review.ReviewResult{Verdict: "approve", Summary: "FIRST_REVIEW_MARKER"}
	if err := Write(tmpDir, "comments", "main", first, nil); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	second := &review.ReviewResult{Verdict: "approve", Summary: "SECOND_REVIEW_MARKER"}
	if err := Write(tmpDir, "comments", "main", second, nil); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "comments", "main.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "FIRST_REVIEW_MARKER") {
		t.Error("first review should be overwritten")
	}
	if !strings.Contains(body, "SECOND_REVIEW_MARKER") {
		t.Error("second review should be present")
	}
}

func TestWrite_DirAutoCreated(t *testing.T) {
	tmpDir := t.TempDir()
	result := &review.ReviewResult{Verdict: "approve", Summary: "ok"}
	if err := Write(tmpDir, "deeply/nested/comments", "feature/x", result, nil); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	path := filepath.Join(tmpDir, "deeply/nested/comments", "feature-x.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
}

func TestWrite_BranchSanitizedInFilename(t *testing.T) {
	tmpDir := t.TempDir()
	result := &review.ReviewResult{Verdict: "approve", Summary: "ok"}
	if err := Write(tmpDir, "comments", "feat/my!branch", result, nil); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	path := filepath.Join(tmpDir, "comments", "feat-mybranch.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected sanitized filename at %s: %v", path, err)
	}
}
