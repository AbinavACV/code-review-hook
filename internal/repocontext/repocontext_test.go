package repocontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_Go(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", `package foo

type Bar struct {
	Name string
}

func (b *Bar) Hello() string {
	return "hi " + b.Name
}

func TopLevel(x int) (string, error) {
	return "", nil
}
`)
	skels, err := Build(dir, []string{"foo.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(skels) != 1 {
		t.Fatalf("expected 1 skeleton, got %d", len(skels))
	}
	body := skels[0].Body
	for _, want := range []string{"foo.go", "go", "type Bar struct", "Hello", "TopLevel"} {
		if !strings.Contains(body, want) {
			t.Errorf("skeleton missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "return \"hi") {
		t.Errorf("skeleton should not contain function bodies\n%s", body)
	}
}

func TestBuild_Python(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.py", `class Greeter:
    def __init__(self, name):
        self.name = name

    def hello(self):
        return f"hi {self.name}"

def top_level(x):
    return x + 1
`)
	skels, _ := Build(dir, []string{"foo.py"})
	body := skels[0].Body
	for _, want := range []string{"python", "Greeter", "top_level"} {
		if !strings.Contains(body, want) {
			t.Errorf("python skeleton missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "return x + 1") {
		t.Errorf("python skeleton should not contain function body\n%s", body)
	}
}

func TestBuild_TypeScript(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.ts", `export interface User { name: string; age: number }
export type Id = string | number;
export function greet(u: User): string { return "hi " + u.name; }
export class Service { run(): void { console.log("x"); } }
`)
	skels, _ := Build(dir, []string{"foo.ts"})
	body := skels[0].Body
	for _, want := range []string{"typescript", "interface User", "type Id", "greet", "Service"} {
		if !strings.Contains(body, want) {
			t.Errorf("ts skeleton missing %q\n%s", want, body)
		}
	}
}

func TestBuild_Unsupported(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# hello\nworld\n")
	skels, _ := Build(dir, []string{"README.md"})
	body := skels[0].Body
	if !strings.Contains(body, "no skeleton extractor") {
		t.Errorf("expected placeholder for unsupported file, got: %s", body)
	}
}

func TestBuild_Unreadable(t *testing.T) {
	dir := t.TempDir()
	skels, _ := Build(dir, []string{"does-not-exist.go"})
	if !strings.Contains(skels[0].Body, "unreadable") {
		t.Errorf("expected unreadable marker, got: %s", skels[0].Body)
	}
}

func TestDetectSymbols_Go(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", `package foo
type Bar struct{}
func Baz() {}
const Pi = 3
`)
	syms := DetectSymbols(dir, []string{"foo.go"})
	got := strings.Join(syms, ",")
	for _, want := range []string{"Bar", "Baz", "Pi"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected symbol %q in %s", want, got)
		}
	}
}

func TestReferencingFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/a.go", `package src
func DoThing() {}
`)
	writeFile(t, dir, "src/b.go", `package src
func caller() { DoThing() }
`)
	writeFile(t, dir, "src/c.go", `package src
func unrelated() {}
`)
	writeFile(t, dir, "vendor/dep.go", `package dep
func DoThing() {}
`)
	files, err := ReferencingFiles(dir, []string{"DoThing"}, []string{"vendor/**"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(files, ",")
	if !strings.Contains(got, "src/a.go") {
		t.Errorf("a.go (definition) should be in references: %s", got)
	}
	if !strings.Contains(got, "src/b.go") {
		t.Errorf("b.go (caller) should be in references: %s", got)
	}
	if strings.Contains(got, "src/c.go") {
		t.Errorf("c.go (unrelated) should NOT be in references: %s", got)
	}
	if strings.Contains(got, "vendor/") {
		t.Errorf("vendor file should be excluded: %s", got)
	}
}

func TestReferencingFiles_WordBoundary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package x
func usesFoo() { Foo() }
`)
	writeFile(t, dir, "b.go", `package x
func unrelated() { _ = "FooBar" + "barFoo" }
`)
	files, _ := ReferencingFiles(dir, []string{"Foo"}, nil)
	got := strings.Join(files, ",")
	if !strings.Contains(got, "a.go") {
		t.Errorf("a.go should match (whole word Foo): %s", got)
	}
	if strings.Contains(got, "b.go") {
		t.Errorf("b.go should not match (FooBar/barFoo are not whole-word Foo): %s", got)
	}
}

func TestAssemble_TruncatesByTokenBudget(t *testing.T) {
	skels := []Skeleton{
		{Path: "big.go", Body: strings.Repeat("a b c d e f g h i j ", 200), Tokens: 600},
		{Path: "small.go", Body: "small body", Tokens: 5},
	}
	body, truncated := Assemble(skels, 100)
	if !truncated {
		t.Errorf("expected truncated=true when budget exceeded")
	}
	if !strings.Contains(body, "small body") {
		t.Errorf("smaller skeleton should fit: %s", body)
	}
}

func TestAssemble_FitsWithinBudget(t *testing.T) {
	skels := []Skeleton{
		{Path: "a", Body: "x", Tokens: 1},
		{Path: "b", Body: "y", Tokens: 1},
	}
	body, truncated := Assemble(skels, 100)
	if truncated {
		t.Errorf("should not truncate within budget")
	}
	if !strings.Contains(body, "x") || !strings.Contains(body, "y") {
		t.Errorf("both bodies should be present: %s", body)
	}
}

func TestSupportedExtensions(t *testing.T) {
	exts := SupportedExtensions()
	want := map[string]bool{".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".rs": true}
	for _, e := range exts {
		delete(want, e)
	}
	if len(want) > 0 {
		t.Errorf("missing supported extensions: %v", want)
	}
}
