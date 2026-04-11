package main

import (
	"context"
	"fmt"
	"testing"
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
		cfg: DefaultConfig(),
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
		cfg: DefaultConfig(),
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
		cfg: DefaultConfig(), // severity_threshold = "error"
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
		cfg:  DefaultConfig(),
	}
	_, err := reviewer.Review(context.Background(), "+ code")
	if err == nil {
		t.Error("expected error from API failure")
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
	if !containsString(result, "truncated") {
		t.Error("truncated diff should contain truncation notice")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
