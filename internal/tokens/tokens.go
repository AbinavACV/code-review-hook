// Package tokens provides cheap, dependency-free token-count estimation.
//
// Estimates only — used for budget gating, not billing. A real tokenizer would
// require a model-specific dependency we don't want in this single-binary tool.
package tokens

import "strings"

// Estimate returns an approximate token count for s.
// Heuristic: ~4 characters per token, with a word-count floor.
func Estimate(s string) int {
	if s == "" {
		return 0
	}
	byChars := len(s) / 4
	byWords := len(strings.Fields(s))
	if byWords > byChars {
		return byWords
	}
	return byChars
}
