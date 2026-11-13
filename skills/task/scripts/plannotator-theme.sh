#!/usr/bin/env bash
# Print the plannotator theme mode: dark, light or system.
#
# It reads `theme.mode` from ${PLANNOTATOR_CONFIG:-$HOME/.plannotator/config.json}.
# A missing file, malformed JSON or an unknown value prints `dark`.
#
# Usage: bash plannotator-theme.sh
#   e.g. python3 qa-export.py render ... --theme "$(bash plannotator-theme.sh)"
set -u

python3 - "${PLANNOTATOR_CONFIG:-$HOME/.plannotator/config.json}" <<'PY'
import json, sys
try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        mode = json.load(fh)["theme"]["mode"]
except Exception:
    mode = None
print(mode if mode in ("dark", "light", "system") else "dark")
PY
