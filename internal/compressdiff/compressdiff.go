// Package compressdiff applies lossless compression to a unified git diff
// before it reaches the LLM reviewer.
//
// Two operations:
//
//   - StripNoise: drop git metadata lines that carry no review signal
//     (index hashes, file mode chmods).
//   - CollapseDuplicates: cluster hunks whose body shape is identical and
//     replace runs with a single representative hunk + a one-line summary.
//     Targets mass renames and lint sweeps where the same edit repeats
//     across many files.
package compressdiff

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/AbinavACV/code-review-hook/internal/diff"
)

// StripNoise removes git metadata lines that don't help review:
//   - "index abc123..def456 100644" lines
//   - "old mode" / "new mode" lines
//   - "similarity index" / "dissimilarity index" lines
//   - "rename from" / "rename to" header lines (the diff body still shows the change)
//
// All other lines (diff --git, ---/+++, @@, body) are preserved verbatim.
func StripNoise(d string) string {
	if d == "" {
		return d
	}
	lines := strings.Split(d, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if isNoise(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func isNoise(line string) bool {
	switch {
	case strings.HasPrefix(line, "index "):
		return true
	case strings.HasPrefix(line, "old mode "), strings.HasPrefix(line, "new mode "):
		return true
	case strings.HasPrefix(line, "similarity index "), strings.HasPrefix(line, "dissimilarity index "):
		return true
	case strings.HasPrefix(line, "rename from "), strings.HasPrefix(line, "rename to "):
		return true
	}
	return false
}

// CollapseDuplicates clusters hunks by body shape. For each cluster of size
// >= 2, the first hunk is kept and a summary line describes the omitted ones.
// Hunks with unique shapes pass through unchanged.
//
// Body shape ignores file paths and absolute line numbers — only the +/- edit
// content matters. This catches mass renames and identical lint fixes.
func CollapseDuplicates(hunks []diff.Hunk) ([]diff.Hunk, []string) {
	if len(hunks) < 2 {
		return hunks, nil
	}

	type cluster struct {
		first   diff.Hunk
		members []diff.Hunk
		order   int
	}

	clusters := make(map[string]*cluster)
	var order []string

	for i, h := range hunks {
		key := shapeKey(h.Body)
		c, ok := clusters[key]
		if !ok {
			c = &cluster{first: h, order: i}
			clusters[key] = c
			order = append(order, key)
		}
		c.members = append(c.members, h)
	}

	collapsed := make([]diff.Hunk, 0, len(clusters))
	var summaries []string
	for _, key := range order {
		c := clusters[key]
		collapsed = append(collapsed, c.first)
		if len(c.members) > 1 {
			summaries = append(summaries, summarizeCluster(c.members))
		}
	}
	return collapsed, summaries
}

// shapeKey returns a hash that is invariant to file paths and absolute line
// numbers. It hashes only +/- lines, with leading whitespace preserved.
func shapeKey(body string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(body, "\n") {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+', '-':
			// Skip file headers (+++ / ---) which start with +/- but have second char + or -.
			if len(line) >= 2 && (line[1] == '+' || line[1] == '-') {
				continue
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	sum := sha1.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func summarizeCluster(members []diff.Hunk) string {
	files := make([]string, 0, len(members))
	for _, m := range members {
		files = append(files, m.File)
	}
	return fmt.Sprintf(
		"Identical edit appears in %d locations (showing 1, omitting %d): %s",
		len(members), len(members)-1, strings.Join(files, ", "),
	)
}
