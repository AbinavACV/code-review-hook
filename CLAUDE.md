# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```
make build      # builds bin/code-review-hook
make test       # go test ./...
make lint       # go vet ./...
make install    # go install ./cmd/code-review-hook
make tidy       # go mod tidy
make clean      # rm -rf bin/ coverage.out coverage.html
```

Run a single test:
```
go test ./internal/review -run TestReviewer_Review -v
```

Manual end-to-end run (bypasses pre-commit framework, requires staged changes + `LLM_API_KEY`):
```
./bin/code-review-hook --base-url=<url> --model=<model>
```

Go 1.25+ required (see `go.mod`). Single external runtime dep: `github.com/openai/openai-go/v3`.

## Architecture

Single-binary Go CLI invoked by [pre-commit](https://pre-commit.com/) on `pre-commit` stage. Hook ID `ai-code-review` declared in `.pre-commit-hooks.yaml`. Entry point `cmd/code-review-hook/main.go` orchestrates a strict pipeline; each stage lives in its own `internal/` package and is independently testable.

Pipeline (`main.go` top-to-bottom):

1. `config.ParseFlags` → CLI flags into a `cliFlags` struct (no side effects on cfg yet).
2. `diff.RepoRoot` → locate git root (fail-open exit 0 if not a repo).
3. `config.Load(repoRoot)` → read `.code-review-hook.yaml`, overlay onto `config.Default()`.
4. `config.ApplyFlags` → CLI flags overwrite YAML values. Precedence is **CLI > YAML > defaults**, enforced here.
5. `config.LoadRules` → read `rules_file` into `cfg.RulesContent` (fail-open warn).
6. `cfg.Validate` → invalid config = exit 1 (one of two non-fail-open exits, the other is missing API key).
7. `diff.HasStaged` → `git diff --cached --quiet`.
8. `diff.Staged` → `git diff --cached`, then `StripBinaryHunks` and `FilterExcludedFiles` (glob match against `cfg.FileExcludePatterns`).
9. `cfg.ResolveAPIKey` → `LLM_API_KEY` env var > `cfg.APIKey`. Empty = exit 1.
10. `review.NewReviewer(cfg)` → builds `OpenAIChatClient` (openai-go SDK pointed at `cfg.BaseURL`).
11. `reviewer.Review(ctx, diff)` → system prompt = base + team rules (`RulesContent`) + `CustomPrompt`; sends diff as user message with `ResponseFormat: JSONObject`; parses into `ReviewResult{Verdict, Summary, Issues[]}`.
12. `displayResults` → colored stderr output via `internal/output`.
13. If `cfg.SaveComments`, `comments.Write(repoRoot, cfg.CommentsDir, branch, result, hunks)` writes `<commentsDir>/<sanitized-branch>.md` (overwrites). Branch from `diff.CurrentBranch`; sanitized via `comments.SanitizeBranch` (`feature/x` → `feature-x`, detached HEAD → `HEAD`).
14. `reviewer.ShouldBlock(result)` checks issue severities against `cfg.SeverityThreshold` → exit 1 (block) or 0 (allow).

### Fail-open principle

Infrastructure failures (API errors, timeouts, rate limits, missing rules file, malformed YAML, unwritable comments dir, git failures) → warn + `exit 0`. Only **two** code paths block a commit non-substantively:
- Missing API key
- Invalid CLI flag / config value (e.g., `severity_threshold: critical`)

Substantive blocks come from `ShouldBlock(result)` when the LLM reports issues at/above threshold. When changing pipeline behavior, preserve this — bypassing fail-open turns infrastructure flakes into broken commits across the team.

### Package boundaries

- `internal/config` owns Config struct, defaults, YAML load, flag parsing, flag application, validation, and rules loading. **Single source of truth for the precedence chain.**
- `internal/diff` wraps git: `RepoRoot`, `HasStaged`, `Staged`, `StagedFiles`, `CurrentBranch`, `StripBinaryHunks`, `FilterExcludedFiles`, `Hunks` (splits unified diff into per-file/per-hunk pieces, used by comments writer to embed patches in the markdown).
- `internal/review` defines the `ChatClient` interface so tests inject mocks; `OpenAIChatClient` is the production impl. `Reviewer.Review` builds the prompt and parses the structured JSON. `ShouldBlock` compares issue severity to `cfg.SeverityThreshold`.
- `internal/comments` handles markdown serialization and branch-name sanitization. Independent of LLM/API concerns.
- `internal/output` is the only package that writes to stderr (colored), gated by `golang.org/x/term` for TTY detection.

### Severity ordering

`info < warning < error`. `--fail-on-warning` is sugar for `--severity-threshold=warning`. When adding a new severity, update `ShouldBlock`, the validator in `config.Validate`, and the LLM system prompt that enumerates allowed values.

## Conventions

- All flag/YAML field additions must be wired through three places: `Config` struct (`internal/config/config.go`), `ParseFlags`/`ApplyFlags` (same file), and `Validate`. Then update `README.md` "All options" table and `.code-review-hook.yaml.example`.
- Tests live next to code (`*_test.go`); `internal/review/review_test.go` shows the mock-`ChatClient` pattern — use it rather than hitting a real LLM.
- The `comments/` directory is **not** in `.gitignore` by default; review files for the current branch land there during local commits. Don't accidentally commit them when working on this repo.
