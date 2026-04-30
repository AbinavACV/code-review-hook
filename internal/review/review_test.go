package review

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AbinavACV/code-review-hook/internal/config"
	"github.com/AbinavACV/code-review-hook/internal/diff"
)

// mockChatClient implements ChatClient for testing.
type mockChatClient struct {
	response  string
	err       error
	calls     atomic.Int32
	lastSys   string
	lastUser  string
	respondFn func(systemMsg, userMsg string) (string, error) // optional override
}

func (m *mockChatClient) Complete(ctx context.Context, systemMsg, userMsg string) (string, error) {
	m.calls.Add(1)
	m.lastSys = systemMsg
	m.lastUser = userMsg
	if m.respondFn != nil {
		return m.respondFn(systemMsg, userMsg)
	}
	return m.response, m.err
}

func sampleHunks() []diff.Hunk {
	return []diff.Hunk{
		{File: "main.go", Body: "@@ -1,1 +1,1 @@\n-old\n+new\n", NewStart: 1},
	}
}

func TestReview_Approve(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{
			response: `{"verdict":"approve","summary":"Looks good","issues":[]}`,
		},
		cfg: config.Default(),
	}
	result, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
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
	result, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
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
	result, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
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
	_, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
	if err == nil {
		t.Error("expected error from API failure")
	}
}

func TestReview_MalformedResponse(t *testing.T) {
	reviewer := &Reviewer{
		chat: &mockChatClient{response: "not valid json {{{"},
		cfg:  config.Default(),
	}
	_, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
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
	result, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
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
	result, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reviewer.ShouldBlock(result) {
		t.Error("warning should block when threshold is warning")
	}
}

func TestReview_SummarizerFanOut(t *testing.T) {
	cfg := config.Default()
	cfg.SummarizerConcurrency = 4
	summ := &mockChatClient{
		respondFn: func(_, user string) (string, error) {
			return "summary for: " + strings.Split(user, "\n")[0], nil
		},
	}
	main := &mockChatClient{
		response: `{"verdict":"approve","summary":"ok","issues":[]}`,
	}
	reviewer := &Reviewer{chat: main, summarizer: summ, cfg: cfg}

	hunks := []diff.Hunk{
		{File: "a.go", Body: "@@ -1,1 +1,1 @@\n-x\n+y\n"},
		{File: "b.go", Body: "@@ -2,1 +2,1 @@\n-p\n+q\n"},
		{File: "c.go", Body: "@@ -3,1 +3,1 @@\n-m\n+n\n"},
	}
	_, err := reviewer.Review(context.Background(), ReviewInput{Hunks: hunks})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := summ.calls.Load(); got != 3 {
		t.Errorf("expected 3 summarizer calls, got %d", got)
	}
	for _, want := range []string{"a.go", "b.go", "c.go", "[SUMMARY:"} {
		if !strings.Contains(main.lastUser, want) {
			t.Errorf("reviewer user message missing %q\n%s", want, main.lastUser)
		}
	}
}

func TestReview_NoSummarizer(t *testing.T) {
	main := &mockChatClient{response: `{"verdict":"approve","summary":"ok","issues":[]}`}
	reviewer := &Reviewer{chat: main, summarizer: nil, cfg: config.Default()}
	_, err := reviewer.Review(context.Background(), ReviewInput{Hunks: sampleHunks()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Boilerplate mentions [SUMMARY: ...] as instruction; check no actual
	// summary lines appear by counting opening brackets.
	if strings.Count(main.lastUser, "[SUMMARY:") > 1 {
		t.Errorf("no summarizer = no per-hunk SUMMARY entries expected\n%s", main.lastUser)
	}
}

func TestReview_RepoContextInSystemPrompt(t *testing.T) {
	main := &mockChatClient{response: `{"verdict":"approve","summary":"ok","issues":[]}`}
	reviewer := &Reviewer{chat: main, cfg: config.Default()}
	_, err := reviewer.Review(context.Background(), ReviewInput{
		Hunks:       sampleHunks(),
		RepoContext: "// foo.go (go)\nfunc Bar() error",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Repository context", "func Bar() error"} {
		if !strings.Contains(main.lastSys, want) {
			t.Errorf("system prompt missing %q\n%s", want, main.lastSys)
		}
	}
}

func TestReview_CollapseSummariesAppended(t *testing.T) {
	main := &mockChatClient{response: `{"verdict":"approve","summary":"ok","issues":[]}`}
	reviewer := &Reviewer{chat: main, cfg: config.Default()}
	_, err := reviewer.Review(context.Background(), ReviewInput{
		Hunks:             sampleHunks(),
		CollapseSummaries: []string{"Identical edit appears in 5 locations: a.go, b.go, c.go, d.go, e.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(main.lastUser, "Cross-file collapsed edits") {
		t.Errorf("collapse summaries section missing\n%s", main.lastUser)
	}
	if !strings.Contains(main.lastUser, "5 locations") {
		t.Errorf("collapse summary content missing\n%s", main.lastUser)
	}
}

func TestBuildSystemPrompt_WithCustomPrompt(t *testing.T) {
	prompt := buildSystemPrompt("", "Focus on security only.", "")
	if !strings.Contains(prompt, "Focus on security only.") {
		t.Error("custom prompt should be appended to system prompt")
	}
	if !strings.Contains(prompt, "Additional instructions") {
		t.Error("should include 'Additional instructions' label before custom prompt")
	}
}

func TestBuildSystemPrompt_NoCustomPrompt(t *testing.T) {
	prompt := buildSystemPrompt("", "", "")
	if strings.Contains(prompt, "Additional instructions") {
		t.Error("should not include 'Additional instructions' when no custom prompt")
	}
	if strings.Contains(prompt, "Team rules") {
		t.Error("should not include 'Team rules' when no rules content")
	}
	if strings.Contains(prompt, "Repository context") {
		t.Error("should not include 'Repository context' when none provided")
	}
}

func TestBuildSystemPrompt_WithRulesContent(t *testing.T) {
	prompt := buildSystemPrompt("Do not use eval().", "", "")
	if !strings.Contains(prompt, "Team rules:") {
		t.Error("should include 'Team rules:' section header")
	}
	if !strings.Contains(prompt, "Do not use eval().") {
		t.Error("rules content should appear in prompt")
	}
}

func TestBuildSystemPrompt_RulesBeforeCustom(t *testing.T) {
	prompt := buildSystemPrompt("Rule: no eval", "Extra: be strict", "")
	rulesIdx := strings.Index(prompt, "Team rules:")
	customIdx := strings.Index(prompt, "Additional instructions:")
	if rulesIdx == -1 || customIdx == -1 {
		t.Fatal("both sections should be present")
	}
	if rulesIdx >= customIdx {
		t.Error("Team rules should appear before Additional instructions")
	}
}

func TestBuildSystemPrompt_RepoContextBeforeRules(t *testing.T) {
	prompt := buildSystemPrompt("Rule: no eval", "", "// skel.go (go)")
	ctxIdx := strings.Index(prompt, "Repository context")
	rulesIdx := strings.Index(prompt, "Team rules:")
	if ctxIdx == -1 || rulesIdx == -1 {
		t.Fatal("both sections should be present")
	}
	if ctxIdx >= rulesIdx {
		t.Error("Repository context should appear before Team rules")
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
