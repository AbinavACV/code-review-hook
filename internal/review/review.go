package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/AbinavACV/code-review-hook/internal/config"
	"github.com/AbinavACV/code-review-hook/internal/diff"
	"github.com/AbinavACV/code-review-hook/internal/output"
)

// ChatClient is the interface for making LLM completions.
// Extracted as an interface so tests can inject a mock.
type ChatClient interface {
	Complete(ctx context.Context, systemMsg, userMsg string) (string, error)
}

// OpenAIChatClient wraps the openai-go SDK client.
type OpenAIChatClient struct {
	client openai.Client
	model  string
}

// reviewResultSchema is the JSON Schema sent to gateways that require
// response_format=json_schema (e.g. internal LLM gateway). Mirrors ReviewResult.
var reviewResultSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"verdict", "summary", "issues"},
	"properties": map[string]any{
		"verdict": map[string]any{"type": "string", "enum": []string{"approve", "request_changes"}},
		"summary": map[string]any{"type": "string"},
		"issues": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"severity", "file", "line", "message"},
				"properties": map[string]any{
					"severity": map[string]any{"type": "string", "enum": []string{"error", "warning", "info"}},
					"file":     map[string]any{"type": "string"},
					"line":     map[string]any{"type": "integer"},
					"message":  map[string]any{"type": "string"},
				},
			},
		},
	},
}

// Complete sends a chat completion request and returns the response content.
func (c *OpenAIChatClient) Complete(ctx context.Context, systemMsg, userMsg string) (string, error) {
	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemMsg),
			openai.UserMessage(userMsg),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "review_result",
					Strict: openai.Bool(true),
					Schema: reviewResultSchema,
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

// plainChatClient sends a normal chat completion (no enforced JSON schema).
// Used by the hunk summarizer where free-text is fine.
type plainChatClient struct {
	client openai.Client
	model  string
}

func (c *plainChatClient) Complete(ctx context.Context, systemMsg, userMsg string) (string, error) {
	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemMsg),
			openai.UserMessage(userMsg),
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Message.Content, nil
}

// ReviewResult is the structured response from the LLM.
type ReviewResult struct {
	Verdict string  `json:"verdict"`
	Summary string  `json:"summary"`
	Issues  []Issue `json:"issues"`
}

// Issue represents a single review finding.
type Issue struct {
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

// Reviewer orchestrates sending diffs to an LLM for code review.
type Reviewer struct {
	chat       ChatClient // primary reviewer model
	summarizer ChatClient // cheap model for parallel hunk summaries (may be nil)
	cfg        config.Config
}

// NewReviewer creates a Reviewer using the provided config.
// If cfg.SummarizerEnabled is true, a separate ChatClient bound to
// cfg.SummarizerModel is created for hunk summarization.
func NewReviewer(cfg config.Config) (*Reviewer, error) {
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("no API key found (set LLM_API_KEY, OPENAI_API_KEY, or api_key in config)")
	}

	client := openai.NewClient(
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey(apiKey),
	)

	r := &Reviewer{
		chat: &OpenAIChatClient{client: client, model: cfg.Model},
		cfg:  cfg,
	}
	if cfg.SummarizerEnabled && cfg.SummarizerModel != "" {
		r.summarizer = &plainChatClient{client: client, model: cfg.SummarizerModel}
	}
	return r, nil
}

// ReviewInput bundles everything the Reviewer needs to produce a review.
type ReviewInput struct {
	Hunks             []diff.Hunk
	CollapseSummaries []string
	RepoContext       string
}

// Review fans out parallel summarizer calls (one per hunk), assembles the
// system + user prompts with summaries, repo context, and collapse notes,
// then calls the reviewer model. If summarization is disabled or the
// summarizer is nil, hunks are sent without per-hunk summaries.
func (r *Reviewer) Review(ctx context.Context, in ReviewInput) (*ReviewResult, error) {
	summaries := r.summarizeHunks(ctx, in.Hunks)

	systemPrompt := buildSystemPrompt(r.cfg.RulesContent, r.cfg.CustomPrompt, in.RepoContext)
	userMsg := buildUserMessage(in.Hunks, summaries, in.CollapseSummaries, r.cfg.MaxDiffLines)

	content, err := r.chat.Complete(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, fmt.Errorf("LLM API call failed: %w", err)
	}

	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}
	return &result, nil
}

// summarizeHunks runs the summarizer model in parallel over the given hunks.
// Returns a slice of length len(hunks); failed/empty entries are "".
// Concurrency is bounded by cfg.SummarizerConcurrency.
func (r *Reviewer) summarizeHunks(ctx context.Context, hunks []diff.Hunk) []string {
	out := make([]string, len(hunks))
	if r.summarizer == nil || len(hunks) == 0 {
		return out
	}

	concurrency := max(r.cfg.SummarizerConcurrency, 1)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	systemPrompt := "You summarize one git diff hunk in 1-3 short sentences. Focus on what changed and why it might matter for review (logic, correctness, side effects). No code, no markdown, no headers. If unclear, say so."

	for i, h := range hunks {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			user := fmt.Sprintf("File: %s\nHunk:\n%s", h.File, h.Body)
			s, err := r.summarizer.Complete(ctx, systemPrompt, user)
			if err == nil {
				out[i] = strings.TrimSpace(s)
			}
		}()
	}
	wg.Wait()
	return out
}

// ShouldBlock returns true if the review result should block the commit
// based on the configured severity threshold.
func (r *Reviewer) ShouldBlock(result *ReviewResult) bool {
	if result.Verdict == "approve" {
		return false
	}
	threshold := severityLevel(r.cfg.SeverityThreshold)
	for _, issue := range result.Issues {
		if severityLevel(issue.Severity) >= threshold {
			return true
		}
	}
	return false
}

func severityLevel(s string) int {
	switch strings.ToLower(s) {
	case "error":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// buildUserMessage formats the diff for the reviewer model. Each hunk is
// preceded by its summary (if any), and collapse summaries are appended below.
// The total is line-capped by maxLines as a safety net.
func buildUserMessage(hunks []diff.Hunk, summaries, collapseSummaries []string, maxLines int) string {
	var b strings.Builder
	b.WriteString("Please review this git diff. Each hunk is preceded by [SUMMARY: ...] when one is available — use both the raw diff and the summary.\n\n## Diff\n\n")
	for i, h := range hunks {
		if i < len(summaries) && summaries[i] != "" {
			b.WriteString("[SUMMARY: ")
			b.WriteString(summaries[i])
			b.WriteString("]\n")
		}
		b.WriteString("--- ")
		b.WriteString(h.File)
		b.WriteString("\n")
		b.WriteString(h.Body)
		b.WriteString("\n\n")
	}
	if len(collapseSummaries) > 0 {
		b.WriteString("## Cross-file collapsed edits\n\n")
		for _, s := range collapseSummaries {
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	body := b.String()
	if maxLines <= 0 {
		return body
	}
	truncated, did := truncateDiff(body, maxLines)
	if did {
		output.PrintWarning("Reviewer message truncated to " + strconv.Itoa(maxLines) + " lines")
	}
	return truncated
}

func truncateDiff(diff string, maxLines int) (string, bool) {
	lines := strings.Split(diff, "\n")
	if len(lines) <= maxLines {
		return diff, false
	}
	return strings.Join(lines[:maxLines], "\n") + "\n\n[... message truncated at " + strconv.Itoa(maxLines) + " lines ...]", true
}

func buildSystemPrompt(rulesContent, customPrompt, repoContext string) string {
	base := `You are an expert code reviewer. You will be given a git diff of staged changes, plus a compressed skeleton of the surrounding repository for context. Use the repository context to judge cross-file impacts (callers of changed functions, types referenced in the diff, etc.) — but only flag issues that are visible in the diff itself.

Review the changes for:
1. Bugs and logic errors (severity: error)
2. Security vulnerabilities (severity: error)
3. Performance issues (severity: warning)
4. Code style and best practices (severity: info)

Respond in this exact JSON format:
{
  "verdict": "approve" or "request_changes",
  "summary": "one sentence summary",
  "issues": [
    {
      "severity": "error" or "warning" or "info",
      "file": "filename or empty string",
      "line": line_number_or_0,
      "message": "description of the issue"
    }
  ]
}

If there are no issues, return verdict "approve" with an empty issues array.
Only return valid JSON. Do not include markdown code fences.`

	if repoContext != "" {
		base += "\n\n## Repository context (signatures only)\n" + repoContext
	}
	if rulesContent != "" {
		base += "\n\nTeam rules:\n" + rulesContent
	}
	if customPrompt != "" {
		base += "\n\nAdditional instructions:\n" + customPrompt
	}
	return base
}
