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

	"github.com/AbinavACV/code-review-hook/internal/comments"
	"github.com/AbinavACV/code-review-hook/internal/config"
	"github.com/AbinavACV/code-review-hook/internal/diff"
	"github.com/AbinavACV/code-review-hook/internal/output"
	"github.com/AbinavACV/code-review-hook/internal/review"
)

func main() {
	cliFlags, err := config.ParseFlags(flag.CommandLine, os.Args[1:])
	if err != nil {
		output.PrintError("Invalid flags: " + err.Error())
		os.Exit(1)
	}

	repoRoot, err := diff.RepoRoot()
	if err != nil {
		output.PrintWarning("Could not find git repository: " + err.Error())
		os.Exit(0)
	}

	cfg, err := config.Load(repoRoot)
	if err != nil {
		output.PrintWarning("Config error (using defaults): " + err.Error())
		cfg = config.Default()
	}

	config.ApplyFlags(&cfg, cliFlags)
	config.LoadRules(repoRoot, &cfg)

	if err := cfg.Validate(); err != nil {
		output.PrintError("Invalid configuration: " + err.Error() + "\nCheck your --flags or .code-review-hook.yaml.")
		os.Exit(1)
	}

	hasChanges, err := diff.HasStaged(repoRoot)
	if err != nil {
		output.PrintWarning("Could not check staged changes: " + err.Error())
		os.Exit(0)
	}
	if !hasChanges {
		output.PrintInfo("No staged changes found. Skipping AI review.")
		os.Exit(0)
	}

	stagedDiff, err := diff.Staged(repoRoot)
	if err != nil {
		output.PrintWarning("Could not get staged diff: " + err.Error())
		os.Exit(0)
	}

	stagedDiff = diff.StripBinaryHunks(stagedDiff)
	stagedDiff = diff.FilterExcludedFiles(stagedDiff, cfg.FileExcludePatterns)

	if strings.TrimSpace(stagedDiff) == "" {
		output.PrintInfo("No reviewable changes after filtering. Skipping AI review.")
		os.Exit(0)
	}

	fileNames, _ := diff.StagedFiles(repoRoot)
	if len(fileNames) > 0 {
		output.PrintInfo(fmt.Sprintf("Reviewing %d file(s): %s", len(fileNames), strings.Join(fileNames, ", ")))
	}

	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		output.PrintError("No API key found. Set the LLM_API_KEY environment variable or add 'api_key' to .code-review-hook.yaml.")
		os.Exit(1)
	}

	reviewer, err := review.NewReviewer(cfg)
	if err != nil {
		output.PrintWarning("Could not initialize reviewer: " + err.Error())
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	output.PrintInfo("Running AI code review...")
	result, err := reviewer.Review(ctx, stagedDiff)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			output.PrintWarning(fmt.Sprintf("AI review timed out after %ds. Allowing commit.", cfg.TimeoutSeconds))
		} else {
			var apierr *openai.Error
			if errors.As(err, &apierr) {
				switch apierr.StatusCode {
				case 401:
					output.PrintWarning("API authentication failed (invalid API key). Allowing commit.")
				case 429:
					output.PrintWarning("API rate limit exceeded. Allowing commit. Try again shortly.")
				default:
					output.PrintWarning(fmt.Sprintf("API error (HTTP %d): %s. Allowing commit.", apierr.StatusCode, apierr.Message))
				}
			} else {
				output.PrintWarning("AI review failed (allowing commit): " + err.Error())
			}
		}
		os.Exit(0)
	}

	displayResults(result)

	if cfg.SaveComments {
		branch, err := diff.CurrentBranch(repoRoot)
		if err != nil {
			output.PrintWarning("Could not detect branch for comment file: " + err.Error())
		} else {
			hunks := diff.Hunks(stagedDiff)
			if err := comments.Write(repoRoot, cfg.CommentsDir, branch, result, hunks); err != nil {
				output.PrintWarning("Could not save review comments: " + err.Error())
			} else {
				output.PrintInfo("Review comments saved to " + cfg.CommentsDir + "/" + comments.SanitizeBranch(branch) + ".md")
			}
		}
	}

	if reviewer.ShouldBlock(result) {
		output.PrintError("Commit blocked by AI code review. Use --no-verify to bypass.")
		os.Exit(1)
	}

	output.PrintSuccess("AI code review passed.")
}

func displayResults(result *review.ReviewResult) {
	output.PrintSection("AI Code Review", result.Summary)
	for _, issue := range result.Issues {
		location := issue.File
		if issue.Line > 0 {
			location += ":" + strconv.Itoa(issue.Line)
		}
		switch issue.Severity {
		case "error":
			output.PrintError("[ERROR] " + location + " — " + issue.Message)
		case "warning":
			output.PrintWarning("[WARN]  " + location + " — " + issue.Message)
		case "info":
			output.PrintInfo("[INFO]  " + location + " — " + issue.Message)
		}
	}
}
