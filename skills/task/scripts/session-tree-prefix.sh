#!/usr/bin/env bash
# Print the tree prefix for a child agterm session opened with
# `--after <anchor>`: "├─" when the anchor already has a child session below it
# (the new one is not the last child), "└─" when the new one will be the last.
# A child is a session whose name starts with "├─" or "└─".
#
# Usage: bash session-tree-prefix.sh [anchor-session-id]   (default: $AGTERM_SESSION_ID)
#   name="$(bash session-tree-prefix.sh) infra core DEV-12"
#
# Outside agterm, or on any lookup failure, it prints "└─".
set -u

anchor="${1:-${AGTERM_SESSION_ID:-}}"
tree=""
[ -n "$anchor" ] && command -v agtermctl >/dev/null 2>&1 && tree="$(agtermctl tree --json 2>/dev/null)"

printf '%s' "$tree" | ANCHOR="$anchor" python3 -c '
import json, os, sys
try:
    anchor = os.environ["ANCHOR"].lower()
    for ws in json.load(sys.stdin)["result"]["tree"]["workspaces"]:
        ids = [s["id"].lower() for s in ws.get("sessions", [])]
        if anchor in ids:
            i = ids.index(anchor) + 1
            nxt = ws["sessions"][i]["name"] if i < len(ids) else ""
            print("├─" if nxt.startswith(("├─", "└─")) else "└─")
            sys.exit(0)
except Exception:
    pass
print("└─")
' 2>/dev/null || echo "└─"
