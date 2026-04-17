package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/AbinavACV/code-review-hook/internal/config"
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
	chat ChatClient
	cfg  config.Config
}

// NewReviewer creates a Reviewer using the provided config.
func NewReviewer(cfg config.Config) (*Reviewer, error) {
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("no API key found (set LLM_API_KEY, OPENAI_API_KEY, or api_key in config)")
	}

	client := openai.NewClient(
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey(apiKey),
	)

	return &Reviewer{
		chat: &OpenAIChatClient{client: client, model: cfg.Model},
		cfg:  cfg,
	}, nil
}

// Review sends the diff to the LLM and returns a structured review result.
func (r *Reviewer) Review(ctx context.Context, diff string) (*ReviewResult, error) {
	truncated, wasTruncated := truncateDiff(diff, r.cfg.MaxDiffLines)
	if wasTruncated {
		output.PrintWarning("Diff truncated to " + strconv.Itoa(r.cfg.MaxDiffLines) + " lines")
	}

	systemPrompt := buildSystemPrompt(r.cfg.RulesContent, r.cfg.CustomPrompt)
	content, err := r.chat.Complete(ctx, systemPrompt, "Please review this git diff:\n\n"+truncated)
	if err != nil {
		return nil, fmt.Errorf("LLM API call failed: %w", err)
	}

	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}
	return &result, nil
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

func truncateDiff(diff string, maxLines int) (string, bool) {
	lines := strings.Split(diff, "\n")
	if len(lines) <= maxLines {
		return diff, false
	}
	return strings.Join(lines[:maxLines], "\n") + "\n\n[... diff truncated at " + strconv.Itoa(maxLines) + " lines ...]", true
}

func buildSystemPrompt(rulesContent, customPrompt string) string {
	base := `You are an expert code reviewer. You will be given a git diff of staged changes.

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

	if rulesContent != "" {
		base += "\n\nTeam rules:\n" + rulesContent
	}
	if customPrompt != "" {
		base += "\n\nAdditional instructions:\n" + customPrompt
	}
	return base
}
