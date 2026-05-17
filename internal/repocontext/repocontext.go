// Package repocontext builds a compressed, diff-relative skeleton of a git
// repository for inclusion in LLM code-review prompts.
//
// The skeleton contains signatures and type declarations only — no function
// bodies — so a 50k-LOC repo collapses to a few thousand tokens.
//
// Public surface:
//
//   - Build(repoRoot, paths)        — produce skeletons for the given files
//   - DetectSymbols(repoRoot, paths) — names of top-level symbols in the files
//   - ReferencingFiles(repoRoot, syms, exclude) — files that mention any sym
//   - SupportedExtensions()         — list of file extensions we can skeletonize
//
// Per-language extraction is delegated to extractor implementations registered
// in extractors.go. Files in unsupported languages get a one-line placeholder.
package repocontext

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AbinavACV/code-review-hook/internal/config"
	"github.com/AbinavACV/code-review-hook/internal/tokens"
)

// Skeleton is the compressed representation of one source file.
type Skeleton struct {
	Path     string
	Language string
	Body     string
	Tokens   int
}

// Build returns a Skeleton for each readable path. Unreadable files and files
// in unsupported languages still get a Skeleton (placeholder body) so the
// caller can see what was excluded vs missing.
func Build(repoRoot string, paths []string) ([]Skeleton, error) {
	out := make([]Skeleton, 0, len(paths))
	for _, rel := range paths {
		full := filepath.Join(repoRoot, rel)
		src, err := os.ReadFile(full)
		if err != nil {
			out = append(out, Skeleton{
				Path:     rel,
				Language: "unknown",
				Body:     fmt.Sprintf("// %s (unreadable: %v)", rel, err),
				Tokens:   0,
			})
			continue
		}
		ext := strings.ToLower(filepath.Ext(rel))
		ex, lang := extractorFor(ext)
		var body string
		if ex == nil {
			body = fmt.Sprintf("// %s (%s, %d lines, no skeleton extractor)",
				rel, languageNameForExt(ext), countLines(src))
		} else {
			extracted, err := ex(src)
			if err != nil || strings.TrimSpace(extracted) == "" {
				body = fmt.Sprintf("// %s (%s, parse failed)", rel, lang)
			} else {
				body = fmt.Sprintf("// %s (%s)\n%s", rel, lang, strings.TrimRight(extracted, "\n"))
			}
		}
		out = append(out, Skeleton{
			Path:     rel,
			Language: lang,
			Body:     body,
			Tokens:   tokens.Estimate(body),
		})
	}
	return out, nil
}

// Assemble joins skeletons into one block, truncating at maxTokens.
// Larger files are added first so we don't waste budget on stubs.
// Returns the assembled body and a flag indicating whether truncation occurred.
func Assemble(skeletons []Skeleton, maxTokens int) (string, bool) {
	if len(skeletons) == 0 {
		return "", false
	}
	sorted := make([]Skeleton, len(skeletons))
	copy(sorted, skeletons)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Tokens > sorted[j].Tokens
	})

	var b strings.Builder
	used := 0
	truncated := false
	for _, s := range sorted {
		if used+s.Tokens > maxTokens {
			truncated = true
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(s.Body)
		used += s.Tokens
	}
	return b.String(), truncated
}

// DetectSymbols extracts the top-level symbol names defined in the given files.
// Used to seed ReferencingFiles. Best-effort: silently skips unreadable files
// and unsupported languages.
func DetectSymbols(repoRoot string, paths []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, rel := range paths {
		ext := strings.ToLower(filepath.Ext(rel))
		sd := symbolDetectorFor(ext)
		if sd == nil {
			continue
		}
		src, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			continue
		}
		for _, sym := range sd(src) {
			if sym == "" {
				continue
			}
			if _, dup := seen[sym]; dup {
				continue
			}
			seen[sym] = struct{}{}
			out = append(out, sym)
		}
	}
	sort.Strings(out)
	return out
}

// ReferencingFiles walks the repo and returns paths of files that contain any
// of the given symbol names as whole words. Excludes files matching exclude
// patterns and files whose extension we can't skeletonize.
//
// Cheap whole-word grep (no regex compile per symbol — substring + boundary
// check). Good enough to widen review context; not a precise call graph.
func ReferencingFiles(repoRoot string, symbols []string, exclude []string) ([]string, error) {
	if len(symbols) == 0 {
		return nil, nil
	}
	supported := SupportedExtensions()
	supportedSet := make(map[string]struct{}, len(supported))
	for _, e := range supported {
		supportedSet[e] = struct{}{}
	}

	var matches []string
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "target" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := supportedSet[ext]; !ok {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return nil
		}
		if config.ShouldExcludeFile(rel, exclude) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if containsAnyWord(src, symbols) {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// SupportedExtensions returns file extensions we can skeletonize (incl. dot).
func SupportedExtensions() []string {
	return supportedExts()
}

func containsAnyWord(src []byte, words []string) bool {
	s := string(src)
	for _, w := range words {
		i := 0
		for {
			j := strings.Index(s[i:], w)
			if j == -1 {
				break
			}
			start := i + j
			end := start + len(w)
			if isWordBoundary(s, start, end) {
				return true
			}
			i = start + 1
		}
	}
	return false
}

func isWordBoundary(s string, start, end int) bool {
	leftOK := start == 0 || !isWordChar(s[start-1])
	rightOK := end == len(s) || !isWordChar(s[end])
	return leftOK && rightOK
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func countLines(src []byte) int {
	n := 1
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	return n
}
