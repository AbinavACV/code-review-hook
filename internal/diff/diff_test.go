package diff

import (
	"os/exec"
	"strings"
	"testing"
)

func TestStripBinaryHunks_NoBinary(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n+ added line\n"
	result := StripBinaryHunks(diff)
	if result != diff {
		t.Error("diff without binary files should be unchanged")
	}
}

func TestStripBinaryHunks_AllBinary(t *testing.T) {
	diff := "diff --git a/image.png b/image.png\nBinary files /dev/null and b/image.png differ"
	result := StripBinaryHunks(diff)
	if strings.TrimSpace(result) != "" {
		t.Errorf("all-binary diff should produce empty result, got: %q", result)
	}
}

func TestStripBinaryHunks_Mixed(t *testing.T) {
	diff := "diff --git a/image.png b/image.png\nBinary files /dev/null and b/image.png differ\ndiff --git a/main.go b/main.go\n+ real code"
	result := StripBinaryHunks(diff)
	if strings.Contains(result, "Binary files") {
		t.Error("binary hunk should have been stripped")
	}
	if !strings.Contains(result, "real code") {
		t.Error("non-binary content should be preserved")
	}
}

func TestFilterExcludedFiles_NoPatterns(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n+ code\n"
	result := FilterExcludedFiles(diff, nil)
	if result != diff {
		t.Error("no patterns should leave diff unchanged")
	}
}

func TestFilterExcludedFiles_ExcludesMatch(t *testing.T) {
	diff := "diff --git a/go.sum b/go.sum\n+ hash\ndiff --git a/main.go b/main.go\n+ code"
	result := FilterExcludedFiles(diff, []string{"go.sum"})
	if strings.Contains(result, "go.sum") {
		t.Error("go.sum should have been excluded")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("main.go should be preserved")
	}
}

func TestFilterExcludedFiles_AllExcluded(t *testing.T) {
	diff := "diff --git a/go.sum b/go.sum\n+ hash"
	result := FilterExcludedFiles(diff, []string{"go.sum"})
	if strings.TrimSpace(result) != "" {
		t.Errorf("fully excluded diff should be empty, got: %q", result)
	}
}

func TestCurrentBranch(t *testing.T) {
	tmpDir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "init"},
		{"checkout", "-b", "my-feature"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, out)
		}
	}

	branch, err := CurrentBranch(tmpDir)
	if err != nil {
		t.Fatalf("CurrentBranch error: %v", err)
	}
	if branch != "my-feature" {
		t.Errorf("expected my-feature, got %q", branch)
	}
}

func TestCurrentBranch_NotARepo(t *testing.T) {
	if _, err := CurrentBranch(t.TempDir()); err == nil {
		t.Error("expected error on non-git directory")
	}
}

func TestHunks_Empty(t *testing.T) {
	if got := Hunks(""); got != nil {
		t.Errorf("expected nil for empty diff, got %v", got)
	}
	if got := Hunks("   \n  "); got != nil {
		t.Errorf("expected nil for whitespace diff, got %v", got)
	}
}

func TestHunks_SingleFile(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,4 @@ func main() {
 ctx := context.Background()
+ doStuff(ctx)
 fmt.Println("done")
`
	hunks := Hunks(diff)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	h := hunks[0]
	if h.File != "main.go" {
		t.Errorf("file: got %q want main.go", h.File)
	}
	if h.OldStart != 10 || h.OldLen != 3 || h.NewStart != 10 || h.NewLen != 4 {
		t.Errorf("offsets wrong: %+v", h)
	}
	if !strings.Contains(h.Body, "doStuff(ctx)") {
		t.Errorf("body missing added line: %q", h.Body)
	}
	if !strings.HasPrefix(h.Body, "@@") {
		t.Errorf("body should start with @@, got %q", h.Body)
	}
}

func TestHunks_MultipleHunks(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
@@ -1,2 +1,3 @@
 line1
+added
 line2
@@ -20,1 +21,2 @@
 tail
+more
`
	hunks := Hunks(diff)
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}
	if hunks[0].File != "a.go" || hunks[1].File != "a.go" {
		t.Errorf("both hunks should belong to a.go")
	}
	if hunks[0].NewStart != 1 || hunks[1].NewStart != 21 {
		t.Errorf("starts wrong: %d %d", hunks[0].NewStart, hunks[1].NewStart)
	}
}

func TestHunks_MultipleFiles(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
@@ -1,1 +1,2 @@
 a
+aa
diff --git a/b.go b/b.go
@@ -5,1 +5,2 @@
 b
+bb
`
	hunks := Hunks(diff)
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}
	if hunks[0].File != "a.go" {
		t.Errorf("first hunk file: got %q", hunks[0].File)
	}
	if hunks[1].File != "b.go" {
		t.Errorf("second hunk file: got %q", hunks[1].File)
	}
}

func TestHunks_SingleLineHeader(t *testing.T) {
	diff := `diff --git a/x.go b/x.go
@@ -5 +5 @@
-old
+new
`
	hunks := Hunks(diff)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if hunks[0].OldLen != 1 || hunks[0].NewLen != 1 {
		t.Errorf("expected default len 1, got old=%d new=%d", hunks[0].OldLen, hunks[0].NewLen)
	}
}

func TestExtractFilename(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"diff --git a/main.go b/main.go", "main.go"},
		{"diff --git a/src/utils.go b/src/utils.go", "src/utils.go"},
		{"diff --git a/old.go b/new.go", "new.go"},
		{"not a diff line", ""},
	}
	for _, tt := range tests {
		got := extractFilename(tt.line)
		if got != tt.expected {
			t.Errorf("extractFilename(%q) = %q, want %q", tt.line, got, tt.expected)
		}
	}
}
