# code-review-hook

AI-powered pre-commit hook that reviews your staged changes before every commit and blocks on issues above a configurable severity threshold.

---

## Requirements

- Go 1.21+
- [pre-commit](https://pre-commit.com/)
- An API key for any OpenAI-compatible endpoint (OpenAI, Azure OpenAI, LiteLLM, Ollama, etc.)

---

## Quick Start

Add to your repo's `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/AbinavACV/code-review-hook
    rev: v0.1.0
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
    rev: v0.1.0
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
