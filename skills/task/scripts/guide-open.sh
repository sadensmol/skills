#!/usr/bin/env bash
# PLANNOTATOR_BROWSER wrapper that plannotator-session.sh sets on every launch:
# plannotator calls it with the page URL once the server is up, before any page is
# open. With PLANNOTATOR_GUIDE set, it loads the guide into that server
# (review-snapshot.sh inject) and writes the result to PLANNOTATOR_GUIDE_STATUS, so the
# page starts on the built Guided Review. A failed inject still opens the page. Then it
# opens the page with the real opener. Only when the opener returns 0 does it promote
# the staged baseline (review-snapshot.sh promote PLANNOTATOR_PROMOTE). A promote
# failure exits non-zero and appends its message to the status file.
#
# Env: PLANNOTATOR_GUIDE (guide JSON, optional), PLANNOTATOR_GUIDE_STATUS (result file),
#      PLANNOTATOR_GUIDE_OPENER (the real opener command),
#      PLANNOTATOR_PROMOTE (code | plan | empty = promote nothing).
set -u
url="$1"
here="$(dirname "$0")"
status="${PLANNOTATOR_GUIDE_STATUS:-$(pwd)/.scratch/review-guide.status}"
if [ -n "${PLANNOTATOR_GUIDE:-}" ]; then
  bash "$here/review-snapshot.sh" inject "$url" "$PLANNOTATOR_GUIDE" > "$status.tmp" 2>&1
  mv "$status.tmp" "$status" 2>/dev/null
fi
"${PLANNOTATOR_GUIDE_OPENER:-open}" "$@" || exit $?
[ -n "${PLANNOTATOR_PROMOTE:-}" ] || exit 0
out="$(bash "$here/review-snapshot.sh" promote "$PLANNOTATOR_PROMOTE" 2>&1)" && exit 0
rc=$?
printf '%s\n' "$out" >> "$status"
printf '%s\n' "$out" >&2
exit $rc
