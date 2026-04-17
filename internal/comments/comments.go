package comments

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AbinavACV/code-review-hook/internal/diff"
	"github.com/AbinavACV/code-review-hook/internal/review"
)

var unsafeCharsRE = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// SanitizeBranch makes a branch name filesystem-safe.
// Replaces "/" with "-", strips chars outside [a-zA-Z0-9._-]. Empty result -> "unknown".
func SanitizeBranch(branch string) string {
	s := strings.ReplaceAll(branch, "/", "-")
	s = unsafeCharsRE.ReplaceAllString(s, "")
	if s == "" {
		return "unknown"
	}
	return s
}

// Write overwrites <repoRoot>/<commentsDir>/<sanitized-branch>.md with the latest review.
// Fail-open: caller treats any error as non-fatal.
func Write(repoRoot, commentsDir, branch string, result *review.ReviewResult, hunks []diff.Hunk) error {
	dir := filepath.Join(repoRoot, commentsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating comments dir: %w", err)
	}
	path := filepath.Join(dir, SanitizeBranch(branch)+".md")
	body := formatMarkdown(branch, result, hunks, time.Now().UTC())
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return fmt.Errorf("writing comments file: %w", err)
	}
	return nil
}

func formatMarkdown(branch string, result *review.ReviewResult, hunks []diff.Hunk, now time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Review for branch `%s`\n\n", branch)
	fmt.Fprintf(&b, "_Generated: %s — verdict: **%s**_\n\n", now.Format(time.RFC3339), result.Verdict)

	fmt.Fprintf(&b, "## Summary\n\n%s\n\n", strings.TrimSpace(result.Summary))

	if len(result.Issues) == 0 {
		b.WriteString("_No issues — verdict: approve._\n")
		return b.String()
	}

	b.WriteString("## Issues\n\n")
	for i, issue := range result.Issues {
		location := issue.File
		if issue.Line > 0 {
			location += ":" + strconv.Itoa(issue.Line)
		}
		fmt.Fprintf(&b, "### %d. %s — `%s`\n\n", i+1, issue.Severity, location)
		fmt.Fprintf(&b, "> %s\n\n", issue.Message)

		if h := matchHunk(issue.File, issue.Line, hunks); h != nil {
			b.WriteString("```diff\n")
			b.WriteString(strings.TrimRight(h.Body, "\n"))
			b.WriteString("\n```\n\n")
		}
	}
	return b.String()
}

// matchHunk finds the hunk containing the issue's line for the given file.
// Returns nil if file/line not found or line == 0.
func matchHunk(file string, line int, hunks []diff.Hunk) *diff.Hunk {
	if line <= 0 || file == "" {
		return nil
	}
	for i := range hunks {
		h := &hunks[i]
		if h.File != file {
			continue
		}
		end := h.NewStart + h.NewLen
		if line >= h.NewStart && line < end {
			return h
		}
	}
	return nil
}
