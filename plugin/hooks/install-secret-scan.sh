#!/usr/bin/env bash
# Install the sadensmol secret-scan as a GLOBAL git pre-commit hook, so it runs
# for every commit in every repo you make (including manual terminal commits).
#
#   install:    bash hooks/install-secret-scan.sh
#   uninstall:  git config --global --unset core.hooksPath
#
# It sets `git config --global core.hooksPath <plugin>/hooks/git-hooks`. That is
# ignored by any repo that sets its OWN core.hooksPath (husky, etc.) — those keep
# working untouched. Default-location repo hooks (.git/hooks/pre-commit) are
# chained by the wrapper, so they still run too.
set -euo pipefail

HOOKS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/git-hooks" && pwd)"
chmod +x "$HOOKS_DIR/pre-commit" "$(dirname "$HOOKS_DIR")/secret-scan.py" 2>/dev/null || true

current="$(git config --global --get core.hooksPath || true)"
if [ -n "$current" ] && [ "$current" != "$HOOKS_DIR" ]; then
  echo "⚠️  global core.hooksPath is already set to:"
  echo "      $current"
  echo "   Not overwriting. To use the sadensmol scanner, either merge it into"
  echo "   that directory, or run:"
  echo "      git config --global core.hooksPath \"$HOOKS_DIR\""
  exit 1
fi

git config --global core.hooksPath "$HOOKS_DIR"
echo "✅ installed: global core.hooksPath -> $HOOKS_DIR"
echo "   secret-scan will run on every 'git commit'. Uninstall with:"
echo "      git config --global --unset core.hooksPath"
