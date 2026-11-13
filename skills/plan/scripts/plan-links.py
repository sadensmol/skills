#!/usr/bin/env python3
"""Turn plan item references into in-page links that plannotator follows.

    plan-links.py <plan.md> fix       rewrite [S3] and [S3](#old) to [S3](#<anchor>), in place
    plan-links.py <plan.md> check     read-only; exit 1 when a reference needs `fix`,
                                      has no or several definitions, or a (#anchor) is dead
    plan-links.py <plan.md> resolve   read-only filter: stdin -> stdout, [S3] -> [S3](#<anchor>)
                                      against <plan.md> (for a new log entry before pair.py)

A reference is an item id in brackets: [S3], [U14], [T2]. A bare S3 stays text: it can
mean something else (AWS S3). An id is defined by a heading that starts with it, by the
first cell of a table row, or by a struck line `~~S4 — …~~`. The anchor is the
plannotator 0.28.3 heading id of that heading, or of the nearest heading above the row or
the struck line. Fenced code, headings, code spans, URLs, other links, escaped brackets
and the user's words (`- asked:`, `- question:`, `- answer:` and their indented
continuation lines) are never changed.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

ID = r"[FICMOKSTVQUD]\d+"
REF = re.compile(r"(?<![\\\w\[])\[(" + ID + r")\](\(#[^)\s]*\))?(?!\()")
ANCHOR = re.compile(r"\]\(#([^)\s]*)\)")
FENCE = re.compile(r"^\s*(```|~~~)")
HEAD_ID = re.compile(r"^(?:~~)?(" + ID + r")(?![\w])")
STRUCK = re.compile(r"^\s*(?:[-*]\s+)?~~(" + ID + r")(?![\w])")
PROTECTED_FIELD = re.compile(r"^\s*- (?:asked|question|answer):")
# Spans that are never changed: code spans, URLs, and links that are not item references.
SKIP = re.compile(r"`[^`]*`|https?://\S+|\[(?!" + ID + r"\])[^\]]*\]\([^)]*\)")


def slug(text: str) -> str:
    """The heading id plannotator 0.28.3 sets (its `Aya` function)."""
    s = text.lower()
    s = re.sub(r"\[\[([^\]]+)\]\]", r"\1", s)
    s = re.sub(r"\[([^\]]+)\]\([^)]+\)", r"\1", s)
    s = re.sub(r"[*_`~]", "", s)
    s = re.sub(r"[\W_]+", "-", s)
    return s.strip("-")


def first_cell(line: str) -> str | None:
    text = line.strip()
    if not text.startswith("|"):
        return None
    cell = text[1:].split("|", 1)[0].strip()
    return re.sub(r"[*~]", "", cell).strip()


class Plan:
    """The heading anchors and the item definitions of one plan file."""

    def __init__(self, lines: list[str]):
        self.code: list[bool] = []
        self.heads: dict[int, str] = {}
        self.defs: dict[str, list[tuple[int, str]]] = {}
        in_code, seen, current = False, {}, ""
        for n, line in enumerate(lines):
            if FENCE.match(line):
                in_code = not in_code
                self.code.append(True)
                continue
            self.code.append(in_code)
            if in_code:
                continue
            text = line.strip()
            if text.startswith("#"):
                title = re.sub(r"^#+\s*", "", text)
                base = slug(title)
                if base:
                    k = seen.get(base, 0)
                    seen[base] = k + 1
                    current = base if k == 0 else f"{base}-{k}"
                    self.heads[n] = current
                    m = HEAD_ID.match(title)
                    if m:
                        self.defs.setdefault(m.group(1), []).append((n, current))
                continue
            if not current:
                continue
            cell = first_cell(line)
            if cell is not None and re.fullmatch(ID, cell):
                self.defs.setdefault(cell, []).append((n, current))
                continue
            m = STRUCK.match(line)
            if m:
                self.defs.setdefault(m.group(1), []).append((n, current))
        self.anchors = set(self.heads.values())

    def target(self, item: str) -> str | None:
        found = self.defs.get(item, [])
        return found[0][1] if len(found) == 1 else None


def protected_mask(lines: list[str]) -> list[bool]:
    """True for the user's-words fields and their indented continuation lines."""
    mask, inside = [], False
    for line in lines:
        if PROTECTED_FIELD.match(line):
            inside = True
        elif not line.strip() or not line[:1].isspace():
            inside = False
        mask.append(inside)
    return mask


def link_line(line: str, plan: Plan, where: str, errors: list[str]) -> tuple[str, int]:
    skip = [(m.start(), m.end()) for m in SKIP.finditer(line)]
    changed = 0

    def repl(m: re.Match) -> str:
        nonlocal changed
        if any(a <= m.start() < b for a, b in skip):
            return m.group(0)
        item, found = m.group(1), plan.defs.get(m.group(1), [])
        if not found:
            errors.append(f"{where}: [{item}] has no definition")
            return m.group(0)
        if len(found) > 1:
            lines = ", ".join(str(n + 1) for n, _ in found)
            errors.append(f"{where}: [{item}] has {len(found)} definitions (lines {lines})")
            return m.group(0)
        new = f"[{item}](#{found[0][1]})"
        changed += new != m.group(0)
        return new

    return REF.sub(repl, line), changed


def dead_anchors(line: str, plan: Plan, where: str, errors: list[str]) -> None:
    code = [(m.start(), m.end()) for m in re.finditer(r"`[^`]*`", line)]
    for m in ANCHOR.finditer(line):
        if not any(a <= m.start() < b for a, b in code) and m.group(1) not in plan.anchors:
            errors.append(f"{where}: dead anchor #{m.group(1)}")


def run(lines: list[str], plan: Plan, name: str, own: bool) -> tuple[list[str], int, list[str]]:
    """Link every reference in `lines`; `own` = the lines are the plan itself."""
    errors: list[str] = []
    out, changed = [], 0
    mask = protected_mask(lines)
    in_code = False
    for n, line in enumerate(lines):
        if own:
            in_code = plan.code[n]
        elif FENCE.match(line):
            in_code = not in_code
            out.append(line)
            continue
        heading = line.strip().startswith("#")
        if in_code or heading or mask[n]:
            out.append(line)
            continue
        where = f"{name}:{n + 1}"
        new, k = link_line(line, plan, where, errors)
        dead_anchors(new, plan, where, errors)
        out.append(new)
        changed += k
    return out, changed, list(dict.fromkeys(errors))


def main(argv: list[str]) -> int:
    if len(argv) != 3 or argv[2] not in ("fix", "check", "resolve"):
        print(__doc__.strip(), file=sys.stderr)
        return 2
    path, mode = Path(argv[1]), argv[2]
    lines = path.read_text(encoding="utf-8").split("\n")
    plan = Plan(lines)
    if mode == "resolve":
        text = sys.stdin.read().split("\n")
        out, _, errors = run(text, plan, "<stdin>", own=False)
        sys.stdout.write("\n".join(out))
        for e in errors:
            print(f"plan-links: {e}", file=sys.stderr)
        return 0
    out, changed, errors = run(lines, plan, str(path), own=True)
    for item, found in sorted(plan.defs.items()):
        if len(found) > 1:
            where = ", ".join(str(n + 1) for n, _ in found)
            errors.append(f"{path}: {item} has {len(found)} definitions (lines {where})")
    errors = list(dict.fromkeys(errors))
    for e in errors:
        print(e, file=sys.stderr)
    if mode == "fix":
        if changed:
            path.write_text("\n".join(out), encoding="utf-8")
        print(f"{path}: {changed} link(s) updated, {len(errors)} error(s)")
        return 1 if errors else 0
    if changed:
        print(f"{path}: {changed} reference(s) need `fix`", file=sys.stderr)
    print(f"{path}: {len(errors)} error(s), {changed} reference(s) to fix")
    return 1 if errors or changed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
