#!/bin/sh
# fake agtermctl: records argv and answers from files in $FAKE_AGTERM_DIR
dir="$FAKE_AGTERM_DIR"
line="$1"; shift; for a in "$@"; do line="$line	$a"; done
printf '%s\n' "$line" >> "$dir/argv.log"
env | grep '^AGTERM_' | cut -d= -f1 >> "$dir/env.log"
sub="$(printf '%s\n' "$line" | cut -f1,2)"
case "$sub" in
"session	new")
  [ -f "$dir/new-fails" ] && { echo "boom" >&2; exit 1; }
  if [ -f "$dir/new-refused" ]; then echo '{"ok":false,"error":"no such workspace"}'; exit 0; fi
  n=$(grep -c '^session	new' "$dir/argv.log")
  echo "{\"ok\":true,\"result\":{\"id\":\"sess-$n\"}}" ;;
"session	wait")
  id=$(printf '%s\n' "$line" | tr '\t' '\n' | grep -A1 '^--target$' | tail -n 1)
  if [ -f "$dir/wait-$id.stderr" ]; then cat "$dir/wait-$id.stderr" >&2; fi
  if [ -f "$dir/wait-$id.exit" ]; then exit "$(cat "$dir/wait-$id.exit")"; fi
  if [ -f "$dir/wait-$id.status" ]; then cat "$dir/wait-$id.status"; exit 0; fi
  exit 124 ;;
"msg	send")
  [ -f "$dir/msg-fails" ] && { echo "no such pane" >&2; exit 1; }
  echo 1 ;;
"tree	--socket"*|"tree	"*)
  n=$(grep -c '^tree' "$dir/argv.log")
  if [ -f "$dir/tree-$n.json" ]; then cat "$dir/tree-$n.json"; else cat "$dir/tree.json"; fi ;;
"session	text")
  n=$(grep -c '^session	text' "$dir/argv.log")
  if [ -f "$dir/text-$n.txt" ]; then cat "$dir/text-$n.txt"; elif [ -f "$dir/text.txt" ]; then cat "$dir/text.txt"; fi ;;
"session	context")
  [ -f "$dir/context-fails" ] && { echo "boom" >&2; exit 1; }
  echo ok ;;
"ask	open")
  [ -f "$dir/open-fails" ] && { echo "boom" >&2; exit 1; }
  [ -f "$dir/open-pending" ] && { echo '{"ok":false,"error":"ask already pending"}'; exit 1; }
  n=$(grep -c '^ask	open' "$dir/argv.log")
  # agterm 0.40 answers the ask verbs bare, without the ok/result envelope
  echo "{\"id\":\"dlg-$n\"}" ;;
"ask	result")
  id=$(printf '%s\n' "$line" | cut -f3)
  if [ -f "$dir/result-$id.json" ]; then cat "$dir/result-$id.json"; else echo '{"result":"pending"}'; exit 1; fi ;;
"session	type")
  [ -f "$dir/type-fails" ] && { echo "no such session" >&2; exit 1; }
  echo ok ;;
*) echo ok ;;
esac
