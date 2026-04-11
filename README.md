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

There are two ways to configure the hook. CLI flags take precedence over the YAML file.

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
```

### Option 2 — `.code-review-hook.yaml` in the repo root

Best for teams sharing a full config, including file exclusions and custom prompts.

```yaml
model: gpt-4o-mini
severity_threshold: error
max_diff_lines: 500
timeout_seconds: 30
fail_on_warning: false

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
| `timeout_seconds` | int | `30` | `--timeout` | API call timeout in seconds (5–120) |
| `file_exclude_patterns` | []string | `["*.lock", "go.sum", "*.pb.go", "vendor/**"]` | — | Glob patterns for files to skip |
| `custom_prompt` | string | `""` | — | Extra instructions appended to the system prompt |

### API key

The API key is the only setting that must be provided as an environment variable (to keep it out of version control and shell history):

```bash
export LLM_API_KEY=sk-...
```

Alternatively, add `api_key` to `.code-review-hook.yaml` — but do not commit that file if it contains a real key.

**Precedence:** `LLM_API_KEY` env var → `api_key` in `.code-review-hook.yaml`

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
Load config (.code-review-hook.yaml + CLI flags)
    │
    ▼
git diff --cached → filter binary files and excluded paths
    │
    ▼
Send diff to LLM → parse structured JSON response
    │
    ▼
Display issues to terminal (stderr, colored)
    │
    ▼
Issues at/above severity_threshold → exit 1 (BLOCK)
No blocking issues              → exit 0 (ALLOW)
```

**Fail-open:** API errors, timeouts, and network failures never block a commit. Only explicit AI findings above the threshold block.

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
