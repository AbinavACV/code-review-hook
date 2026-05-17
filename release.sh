#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 [patch|minor|major]"
  exit 1
}

[[ $# -ne 1 ]] && usage
[[ "$1" != patch && "$1" != minor && "$1" != major ]] && usage

BUMP="$1"

# Must be on main
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" != "main" ]]; then
  echo "error: must be on main (currently on '$BRANCH')" >&2
  exit 1
fi

# Must be clean
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "error: working tree is dirty — commit or stash changes first" >&2
  exit 1
fi

# Pull latest
git pull --ff-only

# Compute next version
LATEST=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1)
if [[ -z "$LATEST" ]]; then
  LATEST="v0.0.0"
fi

MAJOR=$(echo "$LATEST" | cut -d. -f1 | tr -d 'v')
MINOR=$(echo "$LATEST" | cut -d. -f2)
PATCH=$(echo "$LATEST" | cut -d. -f3)

case "$BUMP" in
  major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
  minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
  patch) PATCH=$((PATCH + 1)) ;;
esac

NEXT="v${MAJOR}.${MINOR}.${PATCH}"

echo "Current: $LATEST → Next: $NEXT"
read -r -p "Proceed? [y/N] " CONFIRM
[[ "$CONFIRM" != y && "$CONFIRM" != Y ]] && { echo "Aborted."; exit 0; }

# Bump rev: pins in README.md
sed -i '' "s/rev: ${LATEST}/rev: ${NEXT}/g" README.md

git add README.md
git commit -m "chore: release ${NEXT}"

git tag -a "$NEXT" -m "$NEXT"
git push origin main
git push origin "$NEXT"

echo "Released $NEXT"
