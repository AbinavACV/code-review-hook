package docs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/AbinavACV/code-review-hook/internal/config"
	"github.com/AbinavACV/code-review-hook/internal/output"
)

// GeneratorInput bundles data needed for documentation generation.
type GeneratorInput struct {
	RepoContext  string
	StagedDiff   string
	ChangedFiles []string
	Branch       string
}

// DocumentationResult contains generated documentation.
type DocumentationResult struct {
	Architecture string
	Changelog    string
}

// ChatClient wraps OpenAI API calls for documentation generation.
type ChatClient interface {
	Complete(ctx context.Context, systemMsg, userMsg string) (string, error)
}

// plainChatClient sends a normal chat completion (no enforced JSON schema).
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

// Generator generates documentation using an LLM.
type Generator struct {
	client ChatClient
	cfg    config.Config
}

// NewGenerator creates a Generator with a chat client.
func NewGenerator(client ChatClient, cfg config.Config) *Generator {
	return &Generator{client: client, cfg: cfg}
}

// NewGeneratorFromConfig creates a Generator from config (sets up OpenAI client).
func NewGeneratorFromConfig(cfg config.Config) (*Generator, error) {
	apiKey := cfg.ResolveAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("no API key found")
	}

	client := openai.NewClient(
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey(apiKey),
	)

	chatClient := &plainChatClient{client: client, model: cfg.Model}
	return &Generator{client: chatClient, cfg: cfg}, nil
}

// Generate produces architecture and changelog documentation.
func (g *Generator) Generate(ctx context.Context, input GeneratorInput) (*DocumentationResult, error) {
	result := &DocumentationResult{}

	// Generate architecture
	arch, err := g.GenerateArchitecture(ctx, input.RepoContext, input.StagedDiff)
	if err != nil {
		output.PrintWarning("Failed to generate architecture docs: " + err.Error())
	} else {
		result.Architecture = arch
	}

	// Generate changelog entry
	changelog, err := g.GenerateChangelogEntry(ctx, input.StagedDiff, input.ChangedFiles)
	if err != nil {
		output.PrintWarning("Failed to generate changelog entry: " + err.Error())
	} else {
		result.Changelog = changelog
	}

	return result, nil
}

// GenerateArchitecture generates or updates architecture documentation.
func (g *Generator) GenerateArchitecture(ctx context.Context, repoContext, stagedDiff string) (string, error) {
	systemPrompt := buildArchitectureSystemPrompt()
	userMsg := fmt.Sprintf(`Repository context (code signatures):
%s

Recent code changes (diff):
%s

Generate comprehensive architecture documentation that includes:
1. High-level system overview
2. Component descriptions
3. Key modules and their responsibilities
4. How the recent changes fit into the architecture
5. Important dependencies

Keep it concise but thorough.`, repoContext, stagedDiff)

	content, err := g.client.Complete(ctx, systemPrompt, userMsg)
	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}

	return strings.TrimSpace(content), nil
}

// GenerateChangelogEntry generates a changelog entry for the current changes.
func (g *Generator) GenerateChangelogEntry(ctx context.Context, stagedDiff string, changedFiles []string) (string, error) {
	systemPrompt := buildChangelogSystemPrompt()
	fileList := strings.Join(changedFiles, ", ")
	userMsg := fmt.Sprintf(`Files changed: %s

Diff:
%s

Generate a concise changelog entry (markdown bullet format) that captures:
1. Feature changes
2. Bug fixes
3. Improvements
4. Breaking changes (if any)

Format as markdown bullets. Be specific about what changed and why.`, fileList, stagedDiff)

	content, err := g.client.Complete(ctx, systemPrompt, userMsg)
	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}

	return strings.TrimSpace(content), nil
}

// WriteDocumentation writes architecture and changelog to disk.
func WriteDocumentation(repoRoot string, docsDir string, result *DocumentationResult) error {
	// Create docs directory
	docPath := filepath.Join(repoRoot, docsDir)
	if err := os.MkdirAll(docPath, 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	// Write or update ARCHITECTURE.md
	if result.Architecture != "" {
		archPath := filepath.Join(docPath, "ARCHITECTURE.md")
		header := "# Architecture\n\nLast updated: " + time.Now().Format("2006-01-02 15:04:05 UTC") + "\n\n"
		content := header + result.Architecture
		if err := os.WriteFile(archPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing ARCHITECTURE.md: %w", err)
		}
		output.PrintInfo("Updated " + docsDir + "/ARCHITECTURE.md")
	}

	// Write or update CHANGELOG.md
	if result.Changelog != "" {
		changelogPath := filepath.Join(docPath, "CHANGELOG.md")

		// Read existing changelog
		existing := ""
		if data, err := os.ReadFile(changelogPath); err == nil {
			existing = string(data)
		}

		// Append new entry
		entry := "\n## " + time.Now().Format("2006-01-02") + "\n\n" + result.Changelog
		content := entry
		if existing != "" {
			content = entry + "\n" + existing
		} else {
			content = "# Changelog\n\nAll notable changes to this project will be documented in this file.\n" + content
		}

		if err := os.WriteFile(changelogPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing CHANGELOG.md: %w", err)
		}
		output.PrintInfo("Updated " + docsDir + "/CHANGELOG.md")
	}

	return nil
}

func buildArchitectureSystemPrompt() string {
	return `You are a technical documentation expert. Your role is to create clear, accurate architecture documentation based on code repositories.

Generate architecture documentation that:
- Describes the overall system design and structure
- Explains key components and their responsibilities
- Shows how modules interact
- Identifies important dependencies and relationships
- Is written for developers who need to understand the codebase

Use markdown formatting. Be comprehensive but concise.`
}

func buildChangelogSystemPrompt() string {
	return `You are a changelog generator. Your role is to create clear, concise changelog entries that help users understand what changed in a release.

Generate changelog entries that:
- Are written in past tense
- Use markdown bullet format
- Group related changes together
- Include what changed and why it matters
- Are user-focused, not implementation-focused

Example format:
- Added support for X feature
- Fixed bug where Y would Z
- Improved performance of A by B%
- Breaking: Removed deprecated C function`
}
