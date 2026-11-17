#!/usr/bin/env bash
# Open a companion of this session ($AGTERM_SESSION_ID): `session new --parent`,
# indented under it in the sidebar, moved and closed with it. This is the only way a
# task flow opens a session other than the task session itself. There is no fallback:
# an agterm older than 0.39 has no companions, so the call fails and prints the error.
#
# Usage: bash child-session.sh <name> [agtermctl session new options...]
#   sid="$(bash child-session.sh "test core" --cwd "$repo" --no-select --command "...")"
# Prints what `agtermctl session new` prints: the new id (JSON with --json).
set -u

name="$1"; shift
: "${AGTERM_SESSION_ID:?child-session.sh: not inside an agterm session}"

exec agtermctl session new --parent "$AGTERM_SESSION_ID" --name "$name" "$@"
