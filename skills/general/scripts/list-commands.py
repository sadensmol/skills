#!/usr/bin/env python3
"""List the commands of one skill namespace, or resolve one of them.

usage:
  list-commands.py NAMESPACE            print the namespace's commands as a markdown table
  list-commands.py NAMESPACE COMMAND    print the skill id NAMESPACE-COMMAND, exit 1 if it is not installed

A command is an installed skill whose frontmatter `name` starts with `NAMESPACE-` and that is not
`user-invocable: false`. Skills are read from the harness skill directories under $HOME, so a new
skill shows up without editing anything. Stdlib only.
"""
import re
import sys
from pathlib import Path

SKILL_ROOTS = [Path.home() / ".claude" / "skills", Path.home() / ".agents" / "skills"]
COMMAND_DIRS = [Path.home() / ".claude" / "commands", Path.home() / ".config" / "opencode" / "command"]
LISTER_MARK = "list-commands.py"
SHARED_MARK = "`sadensmol-"


def frontmatter(path: Path) -> dict:
    try:
        text = path.read_text(encoding="utf-8")
    except OSError:
        return {}
    if not text.startswith("---\n"):
        return {}
    end = text.find("\n---", 4)
    if end < 0:
        return {}
    fields, key, block = {}, None, []
    for line in text[4:end].splitlines():
        m = re.match(r"^([A-Za-z0-9_-]+):\s*(.*)$", line)
        if m:
            if key and block:
                fields[key] = " ".join(block)
            key, value = m.group(1), m.group(2).strip()
            block = []
            if value in (">", ">-", "|", "|-"):
                continue
            fields[key] = value.strip("\"'")
            key = None
        elif key and line.strip():
            block.append(line.strip())
    if key and block:
        fields[key] = " ".join(block)
    return fields


def summary(description: str) -> str:
    first = re.split(r"(?<=[.!?])\s|\s—\s", description, maxsplit=1)[0].strip()
    return first if len(first) <= 140 else f"{first[:137]}…"


def installed_skills() -> dict:
    skills = {}
    for root in SKILL_ROOTS:
        if not root.is_dir():
            continue
        for path in sorted(root.glob("*/SKILL.md")) + sorted(root.glob("*/*/SKILL.md")):
            fm = frontmatter(path)
            name = fm.get("name")
            if name and name not in skills:
                skills[name] = fm
    return skills


def namespace_commands(namespace: str) -> dict:
    prefix = f"{namespace}-"
    return {
        name[len(prefix):]: fm
        for name, fm in installed_skills().items()
        if name.startswith(prefix) and fm.get("user-invocable", "true").lower() != "false"
    }


def shared_commands() -> dict:
    found = {}
    for directory in COMMAND_DIRS:
        if not directory.is_dir():
            continue
        for path in sorted(directory.glob("*.md")):
            if path.stem in found:
                continue
            try:
                body = path.read_text(encoding="utf-8")
            except OSError:
                continue
            # a shared command dispatches to a generic skill; a namespace command runs this lister
            if LISTER_MARK in body or SHARED_MARK not in body:
                continue
            found[path.stem] = frontmatter(path).get("description", "")
    return found


def cell(text: str) -> str:
    return text.replace("|", "\\|")


def main() -> int:
    if len(sys.argv) not in (2, 3):
        print(__doc__.strip(), file=sys.stderr)
        return 2
    namespace = sys.argv[1]
    commands = namespace_commands(namespace)

    if len(sys.argv) == 3:
        command = sys.argv[2]
        if command in commands:
            print(f"{namespace}-{command}")
            return 0
        print(f"unknown command: /{namespace} {command}", file=sys.stderr)
        return 1

    if not commands:
        print(f"No user-invocable `{namespace}-*` skills are installed.")
        return 0
    print(f"**/{namespace}** — run one as `/{namespace} <command> [args]`\n")
    print("| Command | What it does |\n|---|---|")
    for command in sorted(commands):
        print(f"| `/{namespace} {command}` | {cell(summary(commands[command].get('description', '')))} |")
    shared = shared_commands()
    if shared:
        print("\nShared commands (they resolve the workspace themselves):\n")
        print("| Command | What it does |\n|---|---|")
        for name, description in shared.items():
            print(f"| `/{name}` | {cell(summary(description))} |")
    return 0


if __name__ == "__main__":
    sys.exit(main())
