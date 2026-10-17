#!/usr/bin/env bash
set -euo pipefail

# Recommends the papers of the week: ./run.sh prepare, then ./run.sh send.
#
# Runs from cron, which inherits none of the shell's environment, so the .env
# beside this script is where the credentials come from. Nothing here is
# committed: .env is gitignored.
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$REPO/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$REPO/.env"
  set +a
fi

# cron's PATH is minimal: ~/.local/bin is where llm (or the CLIs it calls) usually lives.
export PATH="$HOME/.local/bin:$PATH"

cd "$REPO"
go run . "$@"
