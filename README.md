# code-review-hook

AI-powered pre-commit hook that reviews your staged changes before every commit and blocks on issues above a configurable severity threshold.

---

## Requirements

- Go 1.25+
- A C toolchain (Xcode Command Line Tools on macOS, `build-essential` on Linux) — `tree-sitter` is CGo
- [pre-commit](https://pre-commit.com/)
- An API key for any OpenAI-compatible endpoint (OpenAI, Azure OpenAI, LiteLLM, Ollama, etc.)

---

## Quick Start

Add to your repo's `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/AbinavACV/code-review-hook
    rev: v0.2.0
    hooks:
      - id: ai-code-review
```

Then install and set your API key:

```bash
pre-commit install
export LLM_API_KEY=sk-...
```

On the next `git commit`, the hook runs automatically.

---

## Configuration

There are two ways to configure the hook. CLI flags (via `args:`) take precedence over the YAML config file.

### Option 1 — `args:` in `.pre-commit-config.yaml`

Best for simple per-repo settings. No extra files to commit.

```yaml
repos:
  - repo: https://github.com/AbinavACV/code-review-hook
    rev: v0.2.0
    hooks:
      - id: ai-code-review
        args:
          - --model=gpt-4o
          - --severity-threshold=warning
          - --base-url=https://my-gateway.example.com/v1
          - --rules-file=rules.md
```

### Option 2 — `.code-review-hook.yaml` in the repo root

Best for teams sharing a full config, including file exclusions, custom prompts, and team rules.

```yaml
model: gpt-4o-mini
base_url: https://api.openai.com/v1
severity_threshold: error
max_diff_lines: 500
timeout_seconds: 30
fail_on_warning: false
rules_file: rules.md

file_exclude_patterns:
  - "*.lock"
  - "go.sum"
  - "*.pb.go"
  - "vendor/**"

# custom_prompt: |
#   This project follows the Google Go style guide.
#   Flag any exported function without a godoc comment as a warning.
```

Copy `.code-review-hook.yaml.example` from this repo as a starting point.

### All options

| Option | Type | Default | CLI flag | Description |
|---|---|---|---|---|
| `model` | string | `gpt-4o-mini` | `--model` | Model name for the endpoint |
| `base_url` | string | `https://api.openai.com/v1` | `--base-url` | Base URL of any OpenAI-compatible API |
| `api_key` | string | `""` | — | API key (prefer `LLM_API_KEY` env var) |
| `severity_threshold` | string | `error` | `--severity-threshold` | Minimum severity to block: `error`, `warning`, `info` |
| `fail_on_warning` | bool | `false` | `--fail-on-warning` | Shorthand for `severity_threshold: warning` |
| `max_diff_lines` | int | `500` | `--max-diff-lines` | Truncate diffs longer than N lines (min 50) |
| `timeout_seconds` | int | `30` | `--timeout` | API call timeout in seconds (5-120) |
| `file_exclude_patterns` | []string | `["*.lock", "go.sum", "*.pb.go", "vendor/**"]` | — | Glob patterns for files to skip |
| `rules_file` | string | `""` | `--rules-file` | Path to a markdown file with team review rules (relative to repo root) |
| `custom_prompt` | string | `""` | — | Extra instructions appended to the system prompt (after rules) |
| `save_comments` | bool | `true` | `--save-comments` | Write the latest review to `<comments_dir>/<branch>.md` |
| `comments_dir` | string | `"comments"` | `--comments-dir` | Directory for review comment files (relative to repo root) |
| `repo_context_enabled` | bool | `true` | `--repo-context` | Send a compressed whole-repo skeleton (signatures only) alongside the diff |
| `repo_context_max_files` | int | `50` | `--repo-context-max-files` | Cap on files included in the skeleton |
| `repo_context_max_tokens` | int | `8000` | `--repo-context-max-tokens` | Token budget for the skeleton |
| `summarizer_enabled` | bool | `true` | `--summarize-hunks` | Run a cheap-model summarizer on each hunk in parallel before review |
| `summarizer_model` | string | `gpt-4o-mini` | `--summarizer-model` | Model used for parallel hunk summaries |
| `summarizer_concurrency` | int | `8` | — | Max parallel summarizer calls (1-32) |

### Precedence

```
CLI flags (--model, --severity-threshold, etc.)
    ↓ overrides
.code-review-hook.yaml
    ↓ overrides
built-in defaults
```

For the API key specifically: `LLM_API_KEY` env var > `api_key` in `.code-review-hook.yaml`.

### API key

The API key must be provided via the `LLM_API_KEY` environment variable or the `api_key` field in `.code-review-hook.yaml`. If neither is set, the hook exits with an error.

```bash
export LLM_API_KEY=sk-...
```

Do not commit `.code-review-hook.yaml` if it contains a real key. Use the env var instead.

---

## Team Rules

For teams with specific coding standards, create a markdown file (e.g., `rules.md`) with your review rules and point the hook at it:

```yaml
# .code-review-hook.yaml
rules_file: rules.md
```

Or via CLI flag:

```yaml
# .pre-commit-config.yaml
hooks:
  - id: ai-code-review
    args: [--rules-file=rules.md]
```

The file content is injected into the AI's system prompt as a "Team rules" section, giving the model team-specific context for every review. Rules are loaded before `custom_prompt` in the prompt -- rules define the baseline, `custom_prompt` adds ad-hoc tweaks on top.

See `rules.md.example` in this repo for a Python-focused starting point you can adapt to your stack.

If the rules file is missing, the hook warns and continues without rules (fail-open).

---

## Repo context (compressed)

By default the hook sends a **compressed skeleton of the surrounding repository** alongside the diff. The skeleton contains only signatures and type declarations — no function bodies — so a 50k-LOC repo collapses to a few thousand tokens.

The set of files in the skeleton is **diff-relative**: it always includes the changed files, then expands to files that reference any symbol defined in the changed files (whole-word match). This gives the reviewer cross-file awareness — callers of changed functions, types referenced by the diff, etc. — without sending the whole repo.

Supported languages for skeleton extraction: **Go, Python, JavaScript, TypeScript (incl. TSX), Rust**. Other files contribute a one-line placeholder (`// path/file.ext (lang, N lines, no skeleton extractor)`).

The grammars are compiled into the binary via CGo, so building the hook requires a C toolchain (Xcode Command Line Tools on macOS, `build-essential` on Linux). Most developer machines have this already.

Tune via `repo_context_max_files` and `repo_context_max_tokens`. Set `repo_context_enabled: false` (or `--repo-context=false`) to disable.

## Parallel hunk summarization

Before the main review, every hunk in the diff is sent in parallel to a cheap model (`summarizer_model`, default `gpt-4o-mini`) which returns a 1-3 sentence summary. The reviewer then sees both the raw hunk and its summary side-by-side. This helps on large or mechanical diffs where the reviewer might otherwise miss the intent of a change.

Trade-off: every commit pays for N+1 LLM calls (N summarizer + 1 reviewer). Set `summarizer_enabled: false` (or `--summarize-hunks=false`) to disable.

Hunks that look identical across many files (mass renames, lint sweeps) are collapsed before summarization so you don't pay N times for the same edit.

---

## Review Comments

After every commit attempt — pass or fail — the hook writes the LLM review to `comments/<branch>.md` (overwriting any previous review for that branch). Each file contains:

- The verdict and a one-line summary
- One section per issue: severity, file:line, the LLM's message, and the relevant patch hunk extracted from the staged diff

This gives you a persistent artifact to address while iterating on the change, even when the commit is allowed through.

```yaml
# .code-review-hook.yaml
save_comments: true        # default
comments_dir: comments     # default
```

To disable, set `save_comments: false` or pass `--save-comments=false`. To change the location, set `comments_dir` or pass `--comments-dir=.ai-reviews`.

Branch names are sanitized for filesystem safety: `feature/login` becomes `feature-login.md`. On detached HEAD the file is written as `HEAD.md`.

If you want these review files committed, leave `comments/` tracked. If they should stay local, add `comments/` to your `.gitignore` — the hook does not manage this for you.

If the comments directory is unwritable, the hook logs a warning and the commit proceeds (fail-open).

---

## LLM Endpoint Setup

### OpenAI (default)

```bash
export LLM_API_KEY=sk-...
# No base_url needed — defaults to https://api.openai.com/v1
```

### Azure OpenAI

```yaml
# .code-review-hook.yaml
base_url: https://YOUR_RESOURCE.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT
model: gpt-4o
```

```bash
export LLM_API_KEY=your-azure-api-key
```

### LiteLLM / custom gateway

```yaml
# .code-review-hook.yaml
base_url: https://my-litellm-proxy.internal/v1
model: gpt-4o-mini
```

Or via args:

```yaml
args:
  - --base-url=https://my-litellm-proxy.internal/v1
  - --model=gpt-4o-mini
```

### Ollama (local)

```yaml
args:
  - --base-url=http://localhost:11434/v1
  - --model=llama3
```

```bash
export LLM_API_KEY=ollama   # Ollama requires a non-empty key; any value works
```

---

## How It Works

```
git commit
    │
    ▼
pre-commit triggers the hook
    │
    ▼
Parse CLI flags, load .code-review-hook.yaml, load rules file
    │
    ▼
Resolve API key (LLM_API_KEY env var or config file)
    │
    ▼
git diff --cached → strip binary files → filter excluded paths
    │
    ▼
Build system prompt (base + team rules + custom prompt)
    │
    ▼
Send diff to LLM → parse structured JSON response
    │
    ▼
Display issues to terminal (stderr, colored)
    │
    ▼
Issues at/above severity_threshold → exit 1 (BLOCK)
No blocking issues                 → exit 0 (ALLOW)
```

### Error handling

The hook follows a **fail-open** principle for infrastructure failures. API errors, timeouts, rate limits, network issues, missing rules files, and malformed config files all result in a warning and `exit 0` (allow commit). The only scenarios that block a commit (`exit 1`) are:

- The AI explicitly identifies code issues at or above the severity threshold
- The API key is missing entirely (configuration error, not infrastructure failure)
- CLI flags or config values are invalid (e.g., `--severity-threshold=critical`)

---

## Severity Levels

| Severity | Examples | Blocks by default |
|---|---|---|
| `error` | Null pointer dereference, SQL injection, auth bypass | Yes |
| `warning` | N+1 query, missing error handling, shadowed variable | No (use `--fail-on-warning` to enable) |
| `info` | Missing comment, style nit | No |

---

## Bypassing

```bash
git commit --no-verify
```

Skips all pre-commit hooks entirely.

---

## Contributing

Issues and PRs welcome at [github.com/AbinavACV/code-review-hook](https://github.com/AbinavACV/code-review-hook).

### Prerequisites

- Go 1.25+ (see `go.mod`)
- A C toolchain (Xcode Command Line Tools on macOS, `build-essential` on Linux) — required because `tree-sitter` is CGo
- `pre-commit` if you want to test the hook end-to-end against another repo

### Build and test

```bash
make build      # bin/code-review-hook
make test       # go test ./...
make lint       # go vet ./...
make tidy       # go mod tidy
```

Run a single test:

```bash
go test ./internal/review -run TestReviewer_Review -v
```

### Run the dev binary locally

The binary expects to be invoked from inside a git repo with staged changes. After `make build`, point your shell at the local binary:

```bash
cd /path/to/some/test/repo
git add <files>
export LLM_API_KEY=sk-...
/path/to/code-review-hook/bin/code-review-hook \
  --model=gpt-4o-mini \
  --severity-threshold=warning
```

This bypasses the `pre-commit` framework entirely and is the fastest dev loop.

To test as a real pre-commit hook against a sibling repo, point `.pre-commit-config.yaml` at your local checkout instead of the GitHub URL:

```yaml
repos:
  - repo: /absolute/path/to/code-review-hook
    rev: HEAD
    hooks:
      - id: ai-code-review
```

Then `pre-commit install && pre-commit run --all-files` (or just `git commit`).

### Pull request flow

1. Branch from `main`.
2. Make your change. Wire any new flag/YAML field through all three places (`Config` struct, `ParseFlags`/`ApplyFlags`, `Validate`) and update the "All options" table above + `.code-review-hook.yaml.example`.
3. `make test && make lint`.
4. Open a PR. The hook reviews itself on commit — fix or justify any blocking issues it raises.

### Cutting a release

Releases are plain git tags following semver (`vMAJOR.MINOR.PATCH`). `pre-commit` consumers pin to a tag via `rev:`, so once a tag is pushed it must not move.

```bash
# 1. Make sure main is clean and CI is green
git checkout main
git pull --ff-only

# 2. Bump the example pin in README.md (Quick Start + Option 1) to the new version
#    and commit that change on main first.

# 3. Tag and push
git tag -a v0.3.0 -m "v0.3.0"
git push origin v0.3.0
```

Versioning rules of thumb:
- **Patch** (`v0.2.0` → `v0.2.1`): bug fixes, prompt tweaks, no flag/YAML changes
- **Minor** (`v0.2.0` → `v0.3.0`): new flags, new YAML fields, new defaults that change behavior
- **Major**: removed/renamed flags, changed exit-code semantics, breaking the fail-open contract

After tagging, bump the `rev:` example in `README.md` for the next consumer copy-paste.
