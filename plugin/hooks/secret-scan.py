#!/usr/bin/env python3
"""Secret / private-info scanner for staged git changes.

Runs in two modes:

  --staged      Scan `git diff --cached` (real git pre-commit hook). Exit 1 if
                anything suspicious is found, 0 otherwise.

  --pretooluse  Read a Claude Code PreToolUse JSON payload on stdin. If the tool
                call is a `git commit`, scan the staged (and, for `-a`, the
                unstaged tracked) diff. Exit 2 to BLOCK the commit, 0 to allow.

Only ADDED lines are scanned. Findings are reported as file:line with a reason.

Suppress a false positive with any of:
  * an inline marker on the line:  `pragma: allowlist secret`  (or `secret-scan: allow`)
  * a regex in `.secret-scan-allow` at the repo root (one ERE per line, `#` comments)
  * env `SECRET_SCAN_DISABLE=1`      (skip everything)
  * env `SECRET_SCAN_NO_PATHS=1`     (skip the local-path detector only)

Nothing here is committed by the hook; it only reads the diff.
"""
import json
import os
import re
import subprocess
import sys

INLINE_ALLOW = re.compile(r"pragma:\s*allowlist\s*secret|secret-scan:\s*allow|gitleaks:\s*allow", re.I)

# Values that are already generalized / masked / env-referenced — NOT leaks.
PLACEHOLDER = re.compile(
    r"\*{2,}"                       # ***  masked
    r"|x{5,}"                        # xxxxx
    r"|redacted|changeme|placeholder|example|dummy|sample|test[_-]?only"
    r"|your[_-]?\w+"                 # your_token
    r"|<[^>]{1,40}>"                # <your-key>
    r"|\$\{?[A-Za-z_]"              # ${VAR} / $VAR
    r"|os\.Getenv|process\.env|getenv\(|System\.getenv"
    r"|%[sv]"                        # printf placeholder
    r"|\{\{"                          # {{ template }}
    , re.I,
)


def looks_masked(value: str) -> bool:
    value = value.strip().strip("\"'")
    if not value:
        return True
    if PLACEHOLDER.search(value):
        return True
    if set(value) <= set("*xX•.-_"):
        return True
    return False


# (name, compiled regex, group-index-of-secret-value-or-0)
DETECTORS = [
    ("private key block", re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH |PGP |DSA )?PRIVATE KEY-----"), 0),
    ("AWS access key id", re.compile(r"\bAKIA[0-9A-Z]{16}\b"), 0),
    ("GitHub token", re.compile(r"\bgh[oprsu]_[A-Za-z0-9]{36,}\b"), 0),
    ("Slack token", re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{10,}\b"), 0),
    ("Google API key", re.compile(r"\bAIza[0-9A-Za-z_\-]{35}\b"), 0),
    ("JWT", re.compile(r"\beyJ[A-Za-z0-9_\-]+\.eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+"), 0),
    ("credentials in URL", re.compile(r"[a-zA-Z][\w+.\-]*://[^/\s:@]+:([^/\s@]{2,})@"), 1),
    (
        "hardcoded secret/password/token",
        re.compile(
            r"(?i)\b(pass(?:word|wd|phrase)|secret|token|api[_-]?key|access[_-]?key"
            r"|private[_-]?key|client[_-]?secret|auth[_-]?token|credentials?|bearer)\b"
            r"\s*[:=]\s*[\"']?([^\"'\s,;)]{4,})"
        ),
        2,
    ),
]

HOME_PATH = re.compile(r"/(?:Users|home)/(?!shared/|linuxbrew/)([A-Za-z0-9._\-]+)/")


def load_allow(repo_root: str):
    patterns = []
    path = os.path.join(repo_root, ".secret-scan-allow")
    try:
        with open(path, "r", encoding="utf-8", errors="ignore") as fh:
            for line in fh:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                try:
                    patterns.append(re.compile(line))
                except re.error:
                    pass
    except OSError:
        pass
    return patterns


def added_lines(diff: str):
    """Yield (path, lineno, text) for every '+' line in a unified diff (-U0)."""
    path = None
    newline = 0
    for raw in diff.splitlines():
        if raw.startswith("+++ "):
            p = raw[4:].strip()
            path = None if p == "/dev/null" else re.sub(r"^b/", "", p)
            continue
        if raw.startswith("--- "):
            continue
        if raw.startswith("@@"):
            m = re.search(r"\+(\d+)", raw)
            newline = int(m.group(1)) if m else 0
            continue
        if raw.startswith("+"):
            yield path, newline, raw[1:]
            newline += 1
        elif raw.startswith("-"):
            continue
        else:  # context (rare with -U0)
            newline += 1


def scan(diff: str, repo_root: str):
    allow = load_allow(repo_root)
    no_paths = os.environ.get("SECRET_SCAN_NO_PATHS") == "1"
    findings = []
    for path, lineno, text in added_lines(diff):
        if path is None:
            continue
        if INLINE_ALLOW.search(text):
            continue
        if any(p.search(text) for p in allow):
            continue
        for name, rx, gi in DETECTORS:
            m = rx.search(text)
            if not m:
                continue
            value = m.group(gi) if gi and m.groups() else m.group(0)
            if gi and looks_masked(value):
                continue
            findings.append((path, lineno, name, text.strip()[:120]))
            break
        else:
            if not no_paths:
                m = HOME_PATH.search(text)
                if m and m.group(1) not in ("runner", "vsts", "vcap"):
                    findings.append((path, lineno, "private local path", text.strip()[:120]))
    return findings


def git(args):
    return subprocess.run(["git", *args], capture_output=True, text=True).stdout


def repo_root():
    r = git(["rev-parse", "--show-toplevel"]).strip()
    return r or os.getcwd()


def report(findings):
    root = repo_root()
    sys.stderr.write("\n⛔ secret-scan: possible private information in staged changes\n\n")
    for path, lineno, reason, snippet in findings:
        sys.stderr.write(f"  {path}:{lineno}: {reason}\n      {snippet}\n")
    sys.stderr.write(
        "\nGeneralize the value, mask it with ***, or reference an env var.\n"
        "False positive? add `# pragma: allowlist secret` on the line, a regex to\n"
        f"{root}/.secret-scan-allow, or commit with SECRET_SCAN_DISABLE=1.\n\n"
    )


def main():
    if os.environ.get("SECRET_SCAN_DISABLE") == "1":
        return 0

    mode = sys.argv[1] if len(sys.argv) > 1 else "--staged"

    if mode == "--pretooluse":
        try:
            payload = json.load(sys.stdin)
        except (json.JSONDecodeError, ValueError):
            return 0
        if payload.get("tool_name") != "Bash":
            return 0
        command = (payload.get("tool_input") or {}).get("command", "")
        if not re.search(r"\bgit\b.*\bcommit\b", command):
            return 0
        diff = git(["diff", "--cached", "-U0", "--no-color"])
        if re.search(r"(^|\s)(-a\b|--all\b|-[a-zA-Z]*a[a-zA-Z]*\b)", command):
            diff += "\n" + git(["diff", "-U0", "--no-color"])
        findings = scan(diff, repo_root())
        if findings:
            report(findings)
            return 2  # block the tool call
        return 0

    # default: real git pre-commit hook
    diff = git(["diff", "--cached", "-U0", "--no-color"])
    findings = scan(diff, repo_root())
    if findings:
        report(findings)
        return 1  # abort the commit
    return 0


if __name__ == "__main__":
    sys.exit(main())
