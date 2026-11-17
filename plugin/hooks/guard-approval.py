#!/usr/bin/env python3
"""PreToolUse guard: two guard skips need the user's ask-user answer first.

1. `gh pr merge` while a check of that PR fails.
2. A bypass of the secret scan: writing `.secret-scan-allow`, or `SECRET_SCAN_DISABLE`.

A skip passes when the current turn (since the last message the user typed) holds an
answered ask-user question. A chat reply never counts. On any error the hook allows
the call: it must never break normal work.
"""
from __future__ import annotations

import json
import re
import shlex
import subprocess
import sys
from pathlib import Path

ASK_USER_TOOLS = ("AskUserQuestion",)
MERGE_RE = re.compile(r"\bgh\s+pr\s+merge\b")
ALLOW_FILE = ".secret-scan-allow"
READ_ONLY_RE = re.compile(r"^\s*(cat|less|head|tail|grep|rg|ls|wc|git\s+(diff|log|show|status))\b")


def asked_this_turn(transcript: str) -> bool:
    """True when an ask-user question was answered after the user's last typed message."""
    try:
        lines = Path(transcript).read_text(encoding="utf-8").splitlines()
    except (OSError, ValueError):
        return False
    asked_ids: set[str] = set()
    answered = False
    for raw in reversed(lines):
        try:
            entry = json.loads(raw)
        except ValueError:
            continue
        content = (entry.get("message") or {}).get("content")
        if entry.get("type") == "user":
            if isinstance(content, str):
                typed = content
            else:
                typed = " ".join(b.get("text", "") for b in content or []
                                 if isinstance(b, dict) and b.get("type") == "text")
                for b in content or []:
                    if (isinstance(b, dict) and b.get("type") == "tool_result"
                            and not b.get("is_error")):
                        asked_ids.add(b.get("tool_use_id", ""))
            if typed and not typed.startswith("<task-notification>"):
                break
            continue
        if entry.get("type") == "assistant" and isinstance(content, list):
            for b in content:
                if (isinstance(b, dict) and b.get("type") == "tool_use"
                        and b.get("name") in ASK_USER_TOOLS and b.get("id") in asked_ids):
                    answered = True
    return answered


def failing_checks(command: str) -> list[str]:
    """Names of the failing checks of the PR that `gh pr merge` targets; [] when unknown."""
    try:
        argv = shlex.split(command[MERGE_RE.search(command).start():].split("&&")[0]
                           .split(";")[0].split("|")[0])
    except (ValueError, AttributeError):
        return []
    target, repo, i = [], [], 3
    while i < len(argv):
        arg = argv[i]
        if arg in ("-R", "--repo") and i + 1 < len(argv):
            repo = ["--repo", argv[i + 1]]
            i += 2
            continue
        if arg.startswith("--repo="):
            repo = ["--repo", arg.split("=", 1)[1]]
        elif not arg.startswith("-") and not target:
            target = [arg]
        i += 1
    try:
        out = subprocess.run(["gh", "pr", "checks", *target, *repo, "--json", "name,bucket"],
                             capture_output=True, text=True, timeout=20)
        checks = json.loads(out.stdout or "[]")
    except (OSError, ValueError, subprocess.TimeoutExpired):
        return []
    return [c.get("name", "?") for c in checks if c.get("bucket") == "fail"]


def touches_allow_file(tool: str, tool_input: dict) -> bool:
    if tool in ("Write", "Edit", "MultiEdit"):
        return Path(str(tool_input.get("file_path", ""))).name == ALLOW_FILE
    command = str(tool_input.get("command", ""))
    if "SECRET_SCAN_DISABLE" in command:
        return True
    return ALLOW_FILE in command and not READ_ONLY_RE.match(command)


def block(reason: str) -> int:
    sys.stderr.write(reason + "\n")
    return 2


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        return 0
    tool = payload.get("tool_name", "")
    tool_input = payload.get("tool_input") or {}
    transcript = str(payload.get("transcript_path", ""))
    command = str(tool_input.get("command", "")) if tool == "Bash" else ""

    if touches_allow_file(tool, tool_input) and not asked_this_turn(transcript):
        return block("⛔ guard: bypassing the secret scan (.secret-scan-allow or "
                     "SECRET_SCAN_DISABLE) is a guard skip. Ask the user through ask-user "
                     "first, naming the flagged lines; run it only after a yes.")
    if command and MERGE_RE.search(command):
        failed = failing_checks(command)
        if failed and not asked_this_turn(transcript):
            return block("⛔ guard: this PR has failing checks: " + ", ".join(failed[:8]) +
                         ". Merging with red checks is a guard skip. Ask the user through "
                         "ask-user first, naming the checks and the risk. A chat reply is "
                         "never permission.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
