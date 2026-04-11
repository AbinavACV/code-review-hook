package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	// Step 1: Determine repository root.
	repoRoot, err := GetRepoRoot()
	if err != nil {
		PrintWarning("Could not find git repository: " + err.Error())
		os.Exit(0)
	}

	// Step 2: Load configuration (fail open if config is bad).
	cfg, err := LoadConfig(repoRoot)
	if err != nil {
		PrintWarning("Config error (using defaults): " + err.Error())
		cfg = DefaultConfig()
	}
	if err := cfg.Validate(); err != nil {
		PrintWarning("Config validation error (using defaults): " + err.Error())
		cfg = DefaultConfig()
	}

	// Step 3: Check for staged changes.
	hasChanges, err := HasStagedChanges(repoRoot)
	if err != nil {
		PrintWarning("Could not check staged changes: " + err.Error())
		os.Exit(0)
	}
	if !hasChanges {
		PrintInfo("No staged changes found. Skipping AI review.")
		os.Exit(0)
	}

	// Step 4: Get the staged diff.
	diff, err := GetStagedDiff(repoRoot)
	if err != nil {
		PrintWarning("Could not get staged diff: " + err.Error())
		os.Exit(0)
	}

	diff = StripBinaryHunks(diff)
	diff = FilterExcludedFiles(diff, cfg.FileExcludePatterns)

	if strings.TrimSpace(diff) == "" {
		PrintInfo("No reviewable changes after filtering. Skipping AI review.")
		os.Exit(0)
	}

	// Step 5: Resolve API key.
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		PrintWarning("No API key found (set LLM_API_KEY or OPENAI_API_KEY). Skipping AI review.")
		os.Exit(0)
	}

	// Step 6: Initialize reviewer.
	reviewer, err := NewReviewer(cfg)
	if err != nil {
		PrintWarning("Could not initialize reviewer: " + err.Error())
		os.Exit(0)
	}

	// Step 7: Run review with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	PrintInfo("Running AI code review...")
	result, err := reviewer.Review(ctx, diff)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			PrintWarning(fmt.Sprintf("AI review timed out after %ds. Allowing commit.", cfg.TimeoutSeconds))
		} else {
			PrintWarning("AI review failed (allowing commit): " + err.Error())
		}
		os.Exit(0)
	}

	// Step 8: Display results.
	displayResults(result)

	// Step 9: Exit with appropriate code.
	if reviewer.ShouldBlock(result) {
		PrintError("Commit blocked by AI code review. Use --no-verify to bypass.")
		os.Exit(1)
	}

	PrintSuccess("AI code review passed.")
}

func displayResults(result *ReviewResult) {
	PrintSection("AI Code Review", result.Summary)
	for _, issue := range result.Issues {
		location := issue.File
		if issue.Line > 0 {
			location += ":" + strconv.Itoa(issue.Line)
		}
		switch issue.Severity {
		case "error":
			PrintError("[ERROR] " + location + " — " + issue.Message)
		case "warning":
			PrintWarning("[WARN]  " + location + " — " + issue.Message)
		case "info":
			PrintInfo("[INFO]  " + location + " — " + issue.Message)
		}
	}
}
