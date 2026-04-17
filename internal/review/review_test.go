package review

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/AbinavACV/code-review-hook/internal/config"
)

// mockChatClient implements ChatClient for testing.
type mockChatClient struct {
	response string
	err      error
}

func (m *mockChatClient) Complete(ctx context.Context, systemMsg, userMsg string) (string, error) {
	return m.response, m.err
}

func TestReview_Approve(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{
			response: `{"verdict":"approve","summary":"Looks good","issues":[]}`,
		},
		cfg: config.Default(),
	}
	result, err := reviewer.Review(context.Background(), "+ some code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != "approve" {
		t.Errorf("expected approve, got %s", result.Verdict)
	}
	if reviewer.ShouldBlock(result) {
		t.Error("approved review should not block")
	}
}

func TestReview_BlocksOnError(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{
			response: `{"verdict":"request_changes","summary":"Bug found","issues":[{"severity":"error","file":"main.go","line":10,"message":"null pointer dereference"}]}`,
		},
		cfg: config.Default(),
	}
	result, err := reviewer.Review(context.Background(), "+ bad code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reviewer.ShouldBlock(result) {
		t.Error("should block on error-severity issue")
	}
}

func TestReview_WarningDoesNotBlockByDefault(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{
			response: `{"verdict":"request_changes","summary":"Style issue","issues":[{"severity":"warning","file":"main.go","line":5,"message":"consider renaming"}]}`,
		},
		cfg: config.Default(),
	}
	result, err := reviewer.Review(context.Background(), "+ code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reviewer.ShouldBlock(result) {
		t.Error("warning should not block when threshold is error")
	}
}

func TestReview_APIFailure(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{err: fmt.Errorf("connection refused")},
		cfg:  config.Default(),
	}
	_, err := reviewer.Review(context.Background(), "+ code")
	if err == nil {
		t.Error("expected error from API failure")
	}
}

func TestReview_MalformedResponse(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{response: "not valid json {{{"},
		cfg:  config.Default(),
	}
	_, err := reviewer.Review(context.Background(), "+ code")
	if err == nil {
		t.Error("expected error from malformed JSON response")
	}
}

func TestReview_InfoDoesNotBlockAtErrorThreshold(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{
			response: `{"verdict":"request_changes","summary":"Style nit","issues":[{"severity":"info","file":"main.go","line":1,"message":"consider a comment"}]}`,
		},
		cfg: config.Default(),
	}
	result, err := reviewer.Review(context.Background(), "+ code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reviewer.ShouldBlock(result) {
		t.Error("info-severity issue should not block when threshold is error")
	}
}

func TestReview_BlocksAtWarningThreshold(t *testing.T) {
	cfg := config.Default()
	cfg.SeverityThreshold = "warning"
	reviewer := &Reviewer{
		chat: &mockChatClient{
			response: `{"verdict":"request_changes","summary":"Style issue","issues":[{"severity":"warning","file":"main.go","line":5,"message":"rename this"}]}`,
		},
		cfg: cfg,
	}
	result, err := reviewer.Review(context.Background(), "+ code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reviewer.ShouldBlock(result) {
		t.Error("warning should block when threshold is warning")
	}
}

func TestBuildSystemPrompt_WithCustomPrompt(t *testing.T) {
	prompt := buildSystemPrompt("", "Focus on security only.")
	if !strings.Contains(prompt, "Focus on security only.") {
		t.Error("custom prompt should be appended to system prompt")
	}
	if !strings.Contains(prompt, "Additional instructions") {
		t.Error("should include 'Additional instructions' label before custom prompt")
	}
}

func TestBuildSystemPrompt_NoCustomPrompt(t *testing.T) {
	prompt := buildSystemPrompt("", "")
	if strings.Contains(prompt, "Additional instructions") {
		t.Error("should not include 'Additional instructions' when no custom prompt")
	}
	if strings.Contains(prompt, "Team rules") {
		t.Error("should not include 'Team rules' when no rules content")
	}
}

func TestBuildSystemPrompt_WithRulesContent(t *testing.T) {
	prompt := buildSystemPrompt("Do not use eval().", "")
	if !strings.Contains(prompt, "Team rules:") {
		t.Error("should include 'Team rules:' section header")
	}
	if !strings.Contains(prompt, "Do not use eval().") {
		t.Error("rules content should appear in prompt")
	}
	if strings.Contains(prompt, "Additional instructions") {
		t.Error("should not include 'Additional instructions' when no custom prompt")
	}
}

func TestBuildSystemPrompt_RulesBeforeCustom(t *testing.T) {
	prompt := buildSystemPrompt("Rule: no eval", "Extra: be strict")
	rulesIdx := strings.Index(prompt, "Team rules:")
	customIdx := strings.Index(prompt, "Additional instructions:")
	if rulesIdx == -1 || customIdx == -1 {
		t.Fatal("both sections should be present")
	}
	if rulesIdx >= customIdx {
		t.Error("Team rules should appear before Additional instructions")
	}
}

func TestSeverityLevel_AllLevels(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"error", 3},
		{"ERROR", 3},
		{"warning", 2},
		{"Warning", 2},
		{"info", 1},
		{"INFO", 1},
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := severityLevel(tt.input)
		if got != tt.expected {
			t.Errorf("severityLevel(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestTruncateDiff(t *testing.T) {
	short := "line1\nline2\nline3"
	result, truncated := truncateDiff(short, 500)
	if truncated {
		t.Error("short diff should not be truncated")
	}
	if result != short {
		t.Error("short diff should be unchanged")
	}

	long := ""
	for i := 0; i < 600; i++ {
		long += fmt.Sprintf("line %d\n", i)
	}
	result, truncated = truncateDiff(long, 500)
	if !truncated {
		t.Error("long diff should be truncated")
	}
	if !strings.Contains(result, "truncated") {
		t.Error("truncated diff should contain truncation notice")
	}
}
