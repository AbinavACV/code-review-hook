package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// emptyTreeSHA is the well-known SHA for an empty git tree.
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// runGit executes a git command and returns its stdout.
func runGit(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// GetRepoRoot returns the top-level directory of the git repository.
func GetRepoRoot() (string, error) {
	out, err := runGit(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}
	return out, nil
}

// IsFirstCommit returns true if HEAD does not exist (i.e., no commits yet).
func IsFirstCommit(repoRoot string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = repoRoot
	return cmd.Run() != nil
}

// HasStagedChanges returns true if there are staged changes.
func HasStagedChanges(repoRoot string) (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = repoRoot
	err := cmd.Run()
	if err == nil {
		return false, nil // exit 0 = no differences
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true, nil // exit 1 = differences exist
	}
	return false, fmt.Errorf("checking staged changes: %w", err)
}

// GetStagedDiff returns the unified diff of all staged changes.
func GetStagedDiff(repoRoot string) (string, error) {
	base := "HEAD"
	if IsFirstCommit(repoRoot) {
		base = emptyTreeSHA
	}

	diff, err := runGit(repoRoot, "diff", "--cached", "--no-color", "--diff-filter=ACMR", base)
	if err != nil {
		return "", fmt.Errorf("getting staged diff: %w", err)
	}
	return diff, nil
}

// GetStagedFileNames returns the list of staged file names.
func GetStagedFileNames(repoRoot string) ([]string, error) {
	base := "HEAD"
	if IsFirstCommit(repoRoot) {
		base = emptyTreeSHA
	}

	out, err := runGit(repoRoot, "diff", "--cached", "--name-only", "--diff-filter=ACMR", base)
	if err != nil {
		return nil, fmt.Errorf("getting staged file names: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// StripBinaryHunks removes diff sections for binary files.
func StripBinaryHunks(diff string) string {
	lines := strings.Split(diff, "\n")
	var result []string
	var currentHunk []string
	isBinary := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if len(currentHunk) > 0 && !isBinary {
				result = append(result, currentHunk...)
			}
			currentHunk = []string{line}
			isBinary = false
		} else {
			currentHunk = append(currentHunk, line)
			if strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ") {
				isBinary = true
			}
		}
	}
	if len(currentHunk) > 0 && !isBinary {
		result = append(result, currentHunk...)
	}

	return strings.Join(result, "\n")
}

// FilterExcludedFiles removes diff sections for files matching exclude patterns.
func FilterExcludedFiles(diff string, patterns []string) string {
	if len(patterns) == 0 {
		return diff
	}

	lines := strings.Split(diff, "\n")
	var result []string
	var currentHunk []string
	excluded := false

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if len(currentHunk) > 0 && !excluded {
				result = append(result, currentHunk...)
			}
			currentHunk = []string{line}
			// Extract filename from "diff --git a/path b/path"
			filename := extractFilename(line)
			excluded = ShouldExcludeFile(filename, patterns)
		} else {
			currentHunk = append(currentHunk, line)
		}
	}
	if len(currentHunk) > 0 && !excluded {
		result = append(result, currentHunk...)
	}

	return strings.Join(result, "\n")
}

// extractFilename extracts the destination filename from a "diff --git a/... b/..." line.
func extractFilename(diffLine string) string {
	// Format: "diff --git a/path b/path"
	parts := strings.SplitN(diffLine, " b/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}
