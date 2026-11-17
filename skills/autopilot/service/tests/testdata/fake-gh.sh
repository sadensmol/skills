#!/bin/sh
# fake gh: records argv and answers from files in $FAKE_GH_DIR
dir="$FAKE_GH_DIR"
line="$1"; shift; for a in "$@"; do line="$line	$a"; done
printf '%s\n' "$line" >> "$dir/argv.log"
case "$(printf '%s\n' "$line" | cut -f1,2)" in
"pr	view")
  n=$(printf '%s\n' "$line" | cut -f3 | awk -F/ '{print $NF}')
  [ -f "$dir/hang-$n" ] && exec sleep 30
  [ -f "$dir/view-fails" ] && { echo "boom" >&2; exit 1; }
  cat "$dir/view-$n.json" ;;
"api	--paginate"*)
  n=$(printf '%s\n' "$line" | cut -f3 | awk -F/ '{print $(NF-1)}')
  if [ -f "$dir/comments-$n.json" ]; then cat "$dir/comments-$n.json"; else echo '[]'; fi ;;
"api	repos/"*"/git/ref/heads/"*)
  cat "$dir/head.json" ;;
"api	repos/"*"/compare/"*)
  cat "$dir/compare.json" ;;
"api	repos/"*)
  cat "$dir/repo.json" ;;
"api	-X")
  [ -f "$dir/delete-fails" ] && { cat "$dir/delete-fails" >&2; exit 1; }
  exit 0 ;;
"run	list")
  sha=$(printf '%s\n' "$line" | tr '\t' '\n' | grep -A1 '^--commit$' | tail -n 1)
  if [ -f "$dir/runs-$sha.json" ]; then cat "$dir/runs-$sha.json"
  elif [ -f "$dir/runs.json" ]; then cat "$dir/runs.json"; else echo '[]'; fi ;;
"run	rerun")
  [ -f "$dir/rerun-fails" ] && { echo "boom" >&2; exit 1; }
  exit 0 ;;
"run	view")
  id=$(printf '%s\n' "$line" | cut -f3)
  cat "$dir/run-$id.json" ;;
"pr	merge")
  [ -f "$dir/merge-fails" ] && { cat "$dir/merge-fails" >&2; exit 1; }
  exit 0 ;;
*) echo "unexpected gh call: $line" >&2; exit 1 ;;
esac
