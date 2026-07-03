package docs

import (
	"context"
	"testing"

	"github.com/AbinavACV/code-review-hook/internal/config"
)

// mockChatClient is a test double for documentation generation.
type mockChatClient struct {
	responses []string
	err       error
	callCount int
}

func (m *mockChatClient) Complete(ctx context.Context, systemMsg, userMsg string) (string, error) {
	defer func() { m.callCount++ }()
	if m.callCount < len(m.responses) {
		return m.responses[m.callCount], m.err
	}
	return "", m.err
}

func TestGeneratorGenerate(t *testing.T) {
	mock := &mockChatClient{
		responses: []string{
			"# System Overview\n\nThis is the architecture.",
			"- Added new feature X\n- Fixed bug Y",
		},
	}

	cfg := config.Default()
	gen := NewGenerator(mock, cfg)

	input := GeneratorInput{
		RepoContext:  "func Foo() {}",
		StagedDiff:   "--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@",
		ChangedFiles: []string{"foo.go"},
	}

	result, err := gen.Generate(context.Background(), input)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if result.Architecture == "" {
		t.Error("Architecture is empty")
	}
	if result.Changelog == "" {
		t.Error("Changelog is empty")
	}
}

func TestGeneratorGenerateArchitecture(t *testing.T) {
	mock := &mockChatClient{
		responses: []string{"# Architecture\n\nSystem design here."},
	}

	cfg := config.Default()
	gen := NewGenerator(mock, cfg)

	arch, err := gen.GenerateArchitecture(context.Background(), "context", "diff")
	if err != nil {
		t.Fatalf("GenerateArchitecture failed: %v", err)
	}

	if arch == "" {
		t.Error("Architecture is empty")
	}
}

func TestGeneratorGenerateChangelogEntry(t *testing.T) {
	mock := &mockChatClient{
		responses: []string{"- Feature added"},
	}

	cfg := config.Default()
	gen := NewGenerator(mock, cfg)

	changelog, err := gen.GenerateChangelogEntry(context.Background(), "diff", []string{"file.go"})
	if err != nil {
		t.Fatalf("GenerateChangelogEntry failed: %v", err)
	}

	if changelog == "" {
		t.Error("Changelog is empty")
	}
}
