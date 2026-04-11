package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

func main() {
	// Step 1: Parse CLI flags.
	cliFlags, err := ParseFlagSet(flag.CommandLine, os.Args[1:])
	if err != nil {
		PrintError("Invalid flags: " + err.Error())
		os.Exit(1)
	}

	// Step 2: Determine repository root.
	repoRoot, err := GetRepoRoot()
	if err != nil {
		PrintWarning("Could not find git repository: " + err.Error())
		os.Exit(0)
	}

	// Step 3: Load configuration (fail open if config is bad).
	cfg, err := LoadConfig(repoRoot)
	if err != nil {
		PrintWarning("Config error (using defaults): " + err.Error())
		cfg = DefaultConfig()
	}

	// Apply CLI flags on top (highest precedence after env var for API key).
	ApplyCLIFlags(&cfg, cliFlags)

	if err := cfg.Validate(); err != nil {
		PrintError("Invalid configuration: " + err.Error() + "\nCheck your --flags or .code-review-hook.yaml.")
		os.Exit(1)
	}

	// Step 4: Check for staged changes.
	hasChanges, err := HasStagedChanges(repoRoot)
	if err != nil {
		PrintWarning("Could not check staged changes: " + err.Error())
		os.Exit(0)
	}
	if !hasChanges {
		PrintInfo("No staged changes found. Skipping AI review.")
		os.Exit(0)
	}

	// Step 5: Get the staged diff.
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

	// Log which files are being reviewed.
	fileNames, _ := GetStagedFileNames(repoRoot)
	if len(fileNames) > 0 {
		PrintInfo(fmt.Sprintf("Reviewing %d file(s): %s", len(fileNames), strings.Join(fileNames, ", ")))
	}

	// Step 6: Resolve API key.
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		PrintError("No API key found. Set the LLM_API_KEY environment variable or add 'api_key' to .code-review-hook.yaml.")
		os.Exit(1)
	}

	// Step 7: Initialize reviewer.
	reviewer, err := NewReviewer(cfg)
	if err != nil {
		PrintWarning("Could not initialize reviewer: " + err.Error())
		os.Exit(0)
	}

	// Step 8: Run review with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	PrintInfo("Running AI code review...")
	result, err := reviewer.Review(ctx, diff)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			PrintWarning(fmt.Sprintf("AI review timed out after %ds. Allowing commit.", cfg.TimeoutSeconds))
		} else {
			var apierr *openai.Error
			if errors.As(err, &apierr) {
				switch apierr.StatusCode {
				case 401:
					PrintWarning("API authentication failed (invalid API key). Allowing commit.")
				case 429:
					PrintWarning("API rate limit exceeded. Allowing commit. Try again shortly.")
				default:
					PrintWarning(fmt.Sprintf("API error (HTTP %d): %s. Allowing commit.", apierr.StatusCode, apierr.Message))
				}
			} else {
				PrintWarning("AI review failed (allowing commit): " + err.Error())
			}
		}
		os.Exit(0)
	}

	// Step 9: Display results.
	displayResults(result)

	// Step 10: Exit with appropriate code.
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
