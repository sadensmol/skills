#!/usr/bin/env python3
"""Questions and answers of one agent session, for `task reviewq`.

The flow is in ../references/reviewq.md.

  normalize --transcript <jsonl> [--out <json>]
      Reads a Claude Code session transcript. Keeps the active parentUuid chain only,
      and prints its human prompts, AskUserQuestion items and assistant texts as JSON,
      each with its source id. Assistant texts are split at their `> ` quote lines, and
      each part lists the earlier prompts that contain its quote (a candidate aid only).

  render --entries <json> --out <html> --theme dark|light|system
         (--transcript <jsonl> | --partial "<reason>") [--log <md>]...
      Writes the static review page from the entries the agent associated. With
      --transcript, every excerpt must be found verbatim in its source, else it fails.
      Each --log (PLAN.md, .task-qa.md) adds its `### Q<n> — ` entries first, verbatim:
      the Q&A logged during the task, across every session of it.
      With --partial, the page is marked partial with the reason. --theme is the
      plannotator theme mode (scripts/plannotator-theme.sh). The page inlines
      scripts/plannotator-palette.css; system alone follows prefers-color-scheme.

Python 3, stdlib only.
"""

from __future__ import annotations

import argparse
import html
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

ASK_TOOL = "AskUserQuestion"
COMMAND_NAME = re.compile(r"<command-name>(.*?)</command-name>", re.S)
COMMAND_ARGS = re.compile(r"<command-args>(.*?)</command-args>", re.S)


LOG_ENTRY = re.compile(r"^### (Q\d+ — .*)$")


class ExportError(Exception):
    pass


def log_entries(path: Path) -> list[dict]:
    """The `### Q<n> — ` entries of a task Q&A log, each up to the next heading."""
    entries: list[dict] = []
    current = None
    fenced = False
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.startswith("```"):
            fenced = not fenced
        heading = not fenced and line.startswith(("## ", "### "))
        if heading:
            match = LOG_ENTRY.match(line)
            current = {"title": match.group(1), "body": [], "log": path.name} if match else None
            if current:
                entries.append(current)
        elif current is not None:
            current["body"].append(line)
    return [{**e, "body": "\n".join(e["body"]).strip("\n")} for e in entries]


def load_records(path: Path) -> list[dict]:
    records = []
    with path.open(encoding="utf-8") as fh:
        for n, line in enumerate(fh, 1):
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError as exc:
                raise ExportError(f"{path}:{n}: not JSON: {exc}") from exc
    return records


def active_chain(records: list[dict]) -> tuple[list[dict], str | None]:
    """The records of the active branch, oldest first, and the reason the chain is
    partial (None when it reaches its root)."""
    by_uuid = {r["uuid"]: r for r in records if r.get("uuid") and not r.get("isSidechain")}
    leaf = None
    for r in reversed(records):
        if r.get("type") in ("user", "assistant") and not r.get("isSidechain") and r.get("uuid"):
            leaf = r
            break
    if leaf is None:
        raise ExportError("no user or assistant record in the transcript")
    chain, seen, partial = [], set(), None
    cur = leaf
    while cur is not None:
        if cur["uuid"] in seen:
            partial = f"the chain loops at {cur['uuid']}"
            break
        seen.add(cur["uuid"])
        chain.append(cur)
        parent = cur.get("parentUuid")
        if parent is None and cur.get("subtype") == "compact_boundary":
            parent = cur.get("logicalParentUuid")
        if parent is None:
            break
        cur = by_uuid.get(parent)
        if cur is None:
            partial = f"the chain stops at a parent that is not in the file: {parent}"
    chain.reverse()
    return chain, partial


def is_human(origin: object) -> bool:
    return isinstance(origin, dict) and origin.get("kind") == "human"


def content_text(content: object) -> str:
    if isinstance(content, str):
        return content
    parts = []
    for block in content or []:
        if not isinstance(block, dict):
            continue
        if block.get("type") == "text":
            parts.append(block.get("text", ""))
        elif block.get("type") == "image":
            parts.append("[image]")
    return "\n".join(parts)


def prompt_text(raw: str) -> tuple[str, bool]:
    """The prompt text, and whether it is a slash command."""
    name = COMMAND_NAME.search(raw)
    if not name:
        return raw.strip(), False
    args = COMMAND_ARGS.search(raw)
    return f"{name.group(1).strip()} {args.group(1).strip() if args else ''}".strip(), True


def fold(text: str) -> str:
    return " ".join(text.lower().split())


def split_quotes(text: str) -> list[dict]:
    """Split an assistant text at its `> ` quote blocks. Each part starts at a quote
    block (or is the text before the first one) and runs to the next quote block."""
    blocks: list[tuple[list[str], list[str]]] = []
    quote: list[str] = []
    lines: list[str] = []
    prev_quoted = False
    for line in text.split("\n"):
        quoted = line.startswith(">")
        if quoted and not prev_quoted and lines:
            blocks.append((quote, lines))
            quote, lines = [], []
        if quoted and len(quote) == len(lines):
            quote.append(line[1:].strip())
        lines.append(line)
        prev_quoted = quoted
    blocks.append((quote, lines))
    return [{"quote": "\n".join(q) or None, "text": "\n".join(ls)}
            for q, ls in blocks if "\n".join(ls).strip()]


def quote_candidates(quote: str, prompts: list[dict]) -> list[str]:
    pieces = [fold(p) for p in re.split(r"…|\.\.\.", quote) if fold(p)]
    if not pieces:
        return []
    return [p["id"] for p in prompts if all(piece in fold(p["text"]) for piece in pieces)]


def normalize(path: Path) -> dict:
    records = load_records(path)
    chain, partial = active_chain(records)
    items: list[dict] = []
    replies: dict[str, dict] = {}
    asks: dict[str, list[dict]] = {}
    prompts: list[dict] = []
    seen_delivery: set[str] = set()

    for r in chain:
        kind, ts = r.get("type"), r.get("timestamp")
        if kind == "user":
            msg = r.get("message") or {}
            content = msg.get("content")
            if isinstance(content, list):
                for block in content:
                    if isinstance(block, dict) and block.get("type") == "tool_result":
                        ask_answer(r, block, asks)
            if not is_human(r.get("origin")) or r.get("isMeta") or r.get("isCompactSummary"):
                continue
            if r["uuid"] in seen_delivery:
                continue
            text, command = prompt_text(content_text(content))
            if not text:
                continue
            item = {"kind": "prompt", "id": r["uuid"], "via": "typed", "command": command,
                    "time": ts, "text": text}
            items.append(item)
            prompts.append(item)
            seen_delivery.add(r["uuid"])
        elif kind == "attachment":
            att = r.get("attachment") or {}
            if att.get("type") != "queued_command":
                continue
            if not (is_human(att.get("origin")) or is_human(r.get("origin"))):
                continue
            keys = {k for k in (att.get("delivery_id"), att.get("source_uuid")) if k}
            if keys & seen_delivery:
                continue
            seen_delivery |= keys
            raw = att.get("prompt")
            text, command = prompt_text(raw if isinstance(raw, str) else content_text(raw))
            if not text:
                continue
            item = {"kind": "prompt", "id": r["uuid"], "via": "queued", "command": command,
                    "time": att.get("timestamp") or ts, "text": text}
            items.append(item)
            prompts.append(item)
        elif kind == "assistant":
            msg = r.get("message") or {}
            mid = msg.get("id") or r["uuid"]
            for block in msg.get("content") or []:
                if not isinstance(block, dict):
                    continue
                if block.get("type") == "text" and block.get("text", "").strip():
                    reply = replies.get(mid)
                    if reply is None:
                        reply = {"kind": "reply", "id": mid, "time": ts, "text": block["text"],
                                 "prompts_before": [p["id"] for p in prompts]}
                        replies[mid] = reply
                        items.append(reply)
                    else:
                        reply["text"] += "\n\n" + block["text"]
                elif block.get("type") == "tool_use" and block.get("name") == ASK_TOOL:
                    questions = (block.get("input") or {}).get("questions") or []
                    group = []
                    for n, q in enumerate(questions, 1):
                        item = {"kind": "ask", "id": f"{block.get('id')}#{n}", "time": ts,
                                "header": q.get("header"), "text": q.get("question", ""),
                                "options": [o.get("label") for o in q.get("options") or []],
                                "answer": None, "notes": None}
                        group.append(item)
                        items.append(item)
                    asks[block.get("id")] = group

    by_id = {p["id"]: p for p in prompts}
    for reply in replies.values():
        before = [by_id[i] for i in reply.pop("prompts_before")]
        reply["parts"] = [
            {**part, "candidates": quote_candidates(part["quote"], before) if part["quote"] else []}
            for part in split_quotes(reply["text"])
        ]

    return {
        "transcript": str(path),
        "session_ids": sorted({r["sessionId"] for r in chain if r.get("sessionId")}),
        "complete": partial is None,
        "partial_reason": partial,
        "counts": {k: sum(1 for i in items if i["kind"] == k) for k in ("prompt", "ask", "reply")},
        "items": items,
    }


def ask_answer(record: dict, block: dict, asks: dict[str, list[dict]]) -> None:
    group = asks.get(block.get("tool_use_id"))
    if not group:
        return
    result = record.get("toolUseResult")
    if block.get("is_error") or not isinstance(result, dict):
        for item in group:
            item["notes"] = content_text(block.get("content")) or "no answer"
        return
    answers = result.get("answers") or {}
    notes = result.get("annotations") or {}
    for item in group:
        item["answer"] = answers.get(item["text"])
        note = notes.get(item["text"]) if isinstance(notes, dict) else None
        if isinstance(note, dict):
            note = note.get("notes")
        item["notes"] = note if isinstance(note, str) and note else None


def source_texts(norm: dict) -> dict[str, list[str]]:
    texts: dict[str, list[str]] = {}
    for item in norm["items"]:
        texts[item["id"]] = [t for t in (item.get("text"), item.get("answer"), item.get("notes")) if t]
    return texts


def local_time(ts: str | None) -> str:
    if not ts:
        return ""
    try:
        return datetime.fromisoformat(ts.replace("Z", "+00:00")).astimezone().strftime("%H:%M")
    except ValueError:
        return ""


PALETTE = Path(__file__).resolve().parent / "plannotator-palette.css"
THEMES = ("dark", "light", "system")


CSS = """
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif}
main{max-width:900px;margin:0 auto;padding:24px 16px 64px}
h1{font-size:22px;margin:0 0 8px}
header p,dl{color:var(--muted);margin:4px 0}
dl{display:grid;grid-template-columns:max-content 1fr;gap:2px 12px;font-size:13px}
dt{font-weight:600}dd{margin:0;overflow-wrap:anywhere}
.partial{color:var(--partial);font-weight:600}
section{border:1px solid var(--line);border-radius:8px;margin:20px 0;overflow:hidden}
h2{font-size:15px;margin:0;padding:8px 12px;border-bottom:1px solid var(--line);background:var(--q)}
.label{font-size:12px;color:var(--muted);text-transform:uppercase;letter-spacing:.04em;margin:0 0 4px}
.q,.a{padding:10px 12px}
.q{background:var(--q)}
.a{background:var(--a);border-top:1px solid var(--line)}
.text{white-space:pre-wrap;overflow-wrap:anywhere;margin:0;font:inherit}
.mark{color:var(--mark);font-weight:600;margin:0;padding:10px 12px;border-top:1px solid var(--line)}
"""


def render(entries_doc: dict, norm: dict | None, partial: str | None, out: Path, theme: str,
           logs: list[Path]) -> int:
    entries = entries_doc.get("entries")
    if not isinstance(entries, list):
        raise ExportError("the entries file has no `entries` list")
    texts = source_texts(norm) if norm else None
    times = {i["id"]: i.get("time") for i in norm["items"]} if norm else {}

    def check(ref: object, where: str) -> dict:
        if not isinstance(ref, dict) or not isinstance(ref.get("text"), str) or not ref["text"].strip():
            raise ExportError(f"{where}: needs a non-empty `text`")
        if texts is not None:
            src = ref.get("source")
            if src not in texts:
                raise ExportError(f"{where}: source {src!r} is not in the transcript")
            if not any(ref["text"] in t for t in texts[src]):
                raise ExportError(f"{where}: the text is not found verbatim in source {src}")
        return ref

    reasons = [r for r in (norm and norm.get("partial_reason"), partial) if r]
    session = entries_doc.get("session") or (", ".join(norm["session_ids"]) if norm else "")
    source = f"transcript {Path(norm['transcript']).name}" if norm else "the agent's context, not the transcript"
    generated = datetime.now(timezone.utc).astimezone().strftime("%Y-%m-%d %H:%M %Z")
    e = html.escape

    body = []
    logged = [entry for path in logs for entry in log_entries(path)]
    for n, entry in enumerate(logged, 1):
        body.append("\n".join([
            f'<section id="l{n}" data-source="{e(entry["log"])}">',
            f"<h2>{e(entry['title'])} · {e(entry['log'])}</h2>",
            f'<div class="q"><pre class="text">{e(entry["body"])}</pre></div>',
            "</section>"]))
    for n, entry in enumerate(entries, 1):
        where = f"entry {n}"
        question = entry.get("question")
        answers = [check(a, f"{where} answer {k}") for k, a in enumerate(entry.get("answers") or [], 1)]
        if question is not None:
            check(question, f"{where} question")
        speaker = "unassociated" if question is None else entry.get("speaker", "user")
        if speaker not in ("user", "agent", "unassociated"):
            raise ExportError(f"{where}: speaker must be user or agent")
        first = question or (answers[0] if answers else None)
        if first is None:
            raise ExportError(f"{where}: needs a question or an answer")
        when = local_time(times.get(first.get("source")) or entry.get("time"))
        title = f"Q{n} · {speaker}" + (f" · {when}" if when else "")
        parts = [f'<section id="q{n}" data-source="{e(str(first.get("source", "")))}">', f"<h2>{e(title)}</h2>"]
        if question is not None:
            parts.append(f'<div class="q"><p class="label">Question</p><pre class="text">{e(question["text"])}</pre></div>')
        else:
            parts.append('<p class="mark">Not tied to a question.</p>')
        for a in answers:
            parts.append(f'<div class="a"><p class="label">Answer</p><pre class="text">{e(a["text"])}</pre></div>')
        if question is not None and not answers:
            parts.append('<p class="mark">No answer yet.</p>')
        parts.append("</section>")
        body.append("\n".join(parts))

    status = ("partial: " + "; ".join(reasons)) if reasons else "complete"
    scheme = "light dark" if theme == "system" else theme
    page = f"""<!doctype html>
<html lang="en" data-theme="{theme}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="{scheme}">
<title>Questions review</title>
<style>
{PALETTE.read_text(encoding="utf-8")}{CSS}</style>
</head>
<body>
<main>
<header>
<h1>Questions and answers</h1>
<p>Annotate an entry to ask about it or to ask for a change. An entry without an annotation is kept.</p>
<dl>
<dt>Session</dt><dd>{e(session or "unknown")}</dd>
<dt>Source</dt><dd>{e(source)}</dd>
<dt>Status</dt><dd{' class="partial"' if reasons else ''}>{e(status)}</dd>
<dt>Generated</dt><dd>{e(generated)}</dd>
<dt>Logged</dt><dd>{len(logged)} from {e(", ".join(p.name for p in logs) or "no log")}</dd>
<dt>Entries</dt><dd>{len(entries)}</dd>
</dl>
</header>
{chr(10).join(body)}
</main>
</body>
</html>
"""
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(page, encoding="utf-8")
    print(f"WROTE {out} ({len(logged)} logged, {len(entries)} entries, {status}, theme {theme})")
    return 0


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    n = sub.add_parser("normalize", help="print the session's prompts, questions and replies as JSON")
    n.add_argument("--transcript", required=True, type=Path)
    n.add_argument("--out", type=Path)
    r = sub.add_parser("render", help="write the review page from the associated entries")
    r.add_argument("--entries", required=True, type=Path)
    r.add_argument("--out", required=True, type=Path)
    r.add_argument("--theme", required=True, choices=THEMES,
                   help="plannotator theme mode, from scripts/plannotator-theme.sh")
    src = r.add_mutually_exclusive_group(required=True)
    src.add_argument("--transcript", type=Path)
    src.add_argument("--partial", metavar="REASON")
    r.add_argument("--log", type=Path, action="append", default=[],
                   help="a task Q&A log (PLAN.md, .task-qa.md); repeatable")
    args = ap.parse_args(argv)
    try:
        if args.cmd == "normalize":
            data = json.dumps(normalize(args.transcript), ensure_ascii=False, indent=1)
            if args.out:
                args.out.parent.mkdir(parents=True, exist_ok=True)
                args.out.write_text(data + "\n", encoding="utf-8")
                print(f"WROTE {args.out}")
            else:
                print(data)
            return 0
        entries = json.loads(args.entries.read_text(encoding="utf-8"))
        norm = normalize(args.transcript) if args.transcript else None
        return render(entries, norm, args.partial, args.out, args.theme, args.log)
    except (ExportError, OSError, json.JSONDecodeError) as exc:
        print(f"qa-export: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
