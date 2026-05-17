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
	"github.com/AbinavACV/code-review-hook/internal/compressdiff"
	"github.com/AbinavACV/code-review-hook/internal/config"
	"github.com/AbinavACV/code-review-hook/internal/diff"
	"github.com/AbinavACV/code-review-hook/internal/output"
	"github.com/AbinavACV/code-review-hook/internal/repocontext"
	"github.com/AbinavACV/code-review-hook/internal/review"
	"github.com/AbinavACV/code-review-hook/internal/tokens"
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
	stagedDiff = compressdiff.StripNoise(stagedDiff)

	if strings.TrimSpace(stagedDiff) == "" {
		output.PrintInfo("No reviewable changes after filtering. Skipping AI review.")
		os.Exit(0)
	}

	fileNames, _ := diff.StagedFiles(repoRoot)
	if len(fileNames) > 0 {
		output.PrintInfo(fmt.Sprintf("Reviewing %d file(s): %s", len(fileNames), strings.Join(fileNames, ", ")))
	}

	allHunks := diff.Hunks(stagedDiff)
	collapsedHunks, collapseSummaries := compressdiff.CollapseDuplicates(allHunks)
	if len(collapseSummaries) > 0 {
		output.PrintInfo(fmt.Sprintf("Collapsed %d duplicate hunk cluster(s)", len(collapseSummaries)))
	}

	repoCtx := buildRepoContext(repoRoot, cfg, fileNames)

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
	result, err := reviewer.Review(ctx, review.ReviewInput{
		Hunks:             collapsedHunks,
		CollapseSummaries: collapseSummaries,
		RepoContext:       repoCtx,
	})
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

// buildRepoContext returns a compressed repo skeleton scoped to the changed
// files plus files that reference any symbol defined in the changed files.
// All failures are fail-open: warn and return "" so the review still runs.
func buildRepoContext(repoRoot string, cfg config.Config, changedFiles []string) string {
	if !cfg.RepoContextEnabled || len(changedFiles) == 0 {
		return ""
	}

	symbols := repocontext.DetectSymbols(repoRoot, changedFiles)
	referencing, err := repocontext.ReferencingFiles(repoRoot, symbols, cfg.FileExcludePatterns)
	if err != nil {
		output.PrintWarning("Could not scan for referencing files: " + err.Error())
		referencing = nil
	}

	seen := make(map[string]struct{}, len(changedFiles)+len(referencing))
	paths := make([]string, 0, len(changedFiles)+len(referencing))
	for _, f := range changedFiles {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		paths = append(paths, f)
	}
	for _, f := range referencing {
		if _, ok := seen[f]; ok {
			continue
		}
		if len(paths) >= cfg.RepoContextMaxFiles {
			break
		}
		seen[f] = struct{}{}
		paths = append(paths, f)
	}

	skeletons, err := repocontext.Build(repoRoot, paths)
	if err != nil {
		output.PrintWarning("Could not build repo context: " + err.Error())
		return ""
	}

	body, truncated := repocontext.Assemble(skeletons, cfg.RepoContextMaxTokens)
	suffix := ""
	if truncated {
		suffix = " (truncated to fit token budget)"
	}
	output.PrintInfo(fmt.Sprintf("Repo context: %d files, ~%d tokens%s", len(skeletons), tokens.Estimate(body), suffix))
	return body
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
