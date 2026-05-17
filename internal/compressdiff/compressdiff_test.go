package compressdiff

import (
	"strings"
	"testing"

	"github.com/AbinavACV/code-review-hook/internal/diff"
)

func TestStripNoise(t *testing.T) {
	in := `diff --git a/foo.go b/foo.go
index abcdef1..1234567 100644
old mode 100644
new mode 100755
similarity index 95%
rename from old.go
rename to new.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
 ctx
-old
+new
`
	out := StripNoise(in)
	for _, banned := range []string{"index ", "old mode", "new mode", "similarity index", "rename from", "rename to"} {
		if strings.Contains(out, banned) {
			t.Errorf("StripNoise output still contains %q\n%s", banned, out)
		}
	}
	for _, kept := range []string{"diff --git", "--- a/foo.go", "+++ b/foo.go", "@@ -1,3 +1,3 @@", "-old", "+new"} {
		if !strings.Contains(out, kept) {
			t.Errorf("StripNoise output missing %q\n%s", kept, out)
		}
	}
}

func TestStripNoiseEmpty(t *testing.T) {
	if got := StripNoise(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestCollapseDuplicates_MassRename(t *testing.T) {
	body := "@@ -1,3 +1,3 @@\n ctx\n-GetFoo()\n+getFoo()\n"
	hunks := []diff.Hunk{
		{File: "a.go", Body: body, NewStart: 1},
		{File: "b.go", Body: body, NewStart: 1},
		{File: "c.go", Body: body, NewStart: 1},
		{File: "d.go", Body: "@@ -10,3 +10,3 @@\n ctx\n-OldName\n+NewName\n", NewStart: 10},
	}
	collapsed, summaries := CollapseDuplicates(hunks)
	if len(collapsed) != 2 {
		t.Fatalf("expected 2 distinct clusters, got %d", len(collapsed))
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary (for the 3-member cluster), got %d", len(summaries))
	}
	if !strings.Contains(summaries[0], "3 locations") {
		t.Errorf("summary should mention 3 locations: %q", summaries[0])
	}
	for _, expectFile := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(summaries[0], expectFile) {
			t.Errorf("summary missing file %s: %q", expectFile, summaries[0])
		}
	}
}

func TestCollapseDuplicates_NoDuplicates(t *testing.T) {
	hunks := []diff.Hunk{
		{File: "a.go", Body: "@@ -1,1 +1,1 @@\n-x\n+y\n"},
		{File: "b.go", Body: "@@ -1,1 +1,1 @@\n-p\n+q\n"},
	}
	collapsed, summaries := CollapseDuplicates(hunks)
	if len(collapsed) != 2 {
		t.Fatalf("expected 2 hunks unchanged, got %d", len(collapsed))
	}
	if len(summaries) != 0 {
		t.Fatalf("expected 0 summaries, got %d", len(summaries))
	}
}

func TestCollapseDuplicates_LineNumbersIgnored(t *testing.T) {
	hunks := []diff.Hunk{
		{File: "a.go", Body: "@@ -1,1 +1,1 @@\n-old\n+new\n"},
		{File: "b.go", Body: "@@ -50,1 +50,1 @@\n-old\n+new\n"},
	}
	collapsed, _ := CollapseDuplicates(hunks)
	if len(collapsed) != 1 {
		t.Fatalf("hunks differing only in line numbers should cluster: got %d clusters", len(collapsed))
	}
}

func TestCollapseDuplicates_Empty(t *testing.T) {
	if collapsed, summaries := CollapseDuplicates(nil); collapsed != nil || summaries != nil {
		t.Fatalf("nil input should pass through")
	}
	one := []diff.Hunk{{File: "a", Body: "x"}}
	if collapsed, summaries := CollapseDuplicates(one); len(collapsed) != 1 || summaries != nil {
		t.Fatalf("single hunk should pass through")
	}
}
