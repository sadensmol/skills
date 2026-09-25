# Task Pair Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `task new <branch> --pair` opens the task session as an agterm split. A writer
agent runs in the left pane and a read-only reviewer runs in the right pane. The two
agents talk through a file mailbox.

**Architecture:** One stdlib-only python3 script, `sadensmol-task/scripts/pair.py`,
owns everything generic:
- the mailbox (`send` / `wait` / `status`);
- the per-task setup (`init` / `brief`);
- the split launch (`launch`).

OpenCode is woken through `prompt_async` on its `--port` server. Claude Code is woken by
a background `pair.py wait` that exits when a message arrives. Each project's
`new-task.sh` only parses `--pair` and calls `pair.py`. The protocol the agents follow
is `sadensmol-task/pair.md`.

**Tech Stack:** python3 (stdlib only: argparse, json, urllib, http.server for tests,
unittest), bash 3.2, agterm `agtermctl`, OpenCode 1.18.x HTTP API, Claude Code CLI.

**Spec:** `docs/superpowers/specs/2026-09-25-task-pair-mode-design.md`

## Global Constraints

- This repository is PUBLIC. `pair.py`, `pair.md` and the `sadensmol-task` SKILL.md
  text must be generic: no project, company, workspace, tracker or user-path names.
- python3 standard library only. No pip packages.
- Shell edits must run on macOS bash 3.2 (`set -euo pipefail`, empty-array-safe
  expansions).
- agterm only. No tmux code paths.
- Roles are fixed: writer = left pane, reviewer = right pane. The reviewer kind is the
  opposite of the writer kind (`claude` ↔ `opencode`).
- Brief text is nested inside single quotes by the launchers, so it must never contain
  `'`.
- Message labels, verbatim: `Chat from writer:`, `Chat from reviewer:`,
  `Chat from reviewer (user request):`.
- OpenCode reviewer agent name: `pair-reviewer`, with permission
  `{"edit": "deny", "bash": "allow"}`.
- Never create git commits. The user commits. Each task ends with a diff checkpoint.

## Review Focus

1. Two messages arrive before a Claude waiter re-arms → the next `wait` prints both,
   oldest first, and loses neither.
2. The OpenCode server is down or restarting when a message is sent → the message is
   kept, the target pane is marked `blocked`, and the next successful send delivers
   the backlog in order before the new message.
3. The Claude peer has no live waiter (never started, or crashed) → `send` warns, exits
   3, keeps the message in the inbox, and the next `wait` delivers it.
4. A message with quotes, apostrophes, newlines or non-ASCII text → it arrives
   byte-for-byte (JSON over HTTP for OpenCode, a file for Claude). No shell is involved.
5. A task is re-opened: the same writer kind is required, a stale waiter pid is
   cleared, the OpenCode port is fresh, the first OpenCode prompt carries a catch-up
   sentence, and nothing re-launches while the old agterm session still exists.

Items 1–4 have tests in Task 1. The init/brief half of 5 has tests in Task 2, and the
launch half has tests in Task 4. The existing-session half of 5 is a shell branch in
Tasks 6/7 with no unit test; Task 8 Step 4 checks it end to end.

---

## File Structure

| File | Responsibility |
|---|---|
| Create `skills/task/scripts/pair.py` | Mailbox, wake-up, `init`, `brief`, `launch`. Generic. |
| Create `skills/task/scripts/test_pair.py` | unittest suite for `pair.py`. It uses a fake `agtermctl` and a stub OpenCode server. |
| Create `skills/task/pair.md` | The protocol both agents read. |
| Modify `skills/task/SKILL.md` | A short *Pair mode* section that points at `pair.md` and `pair.py`. |
| Modify `<work_swipegames>/skills/task/scripts/new-task.sh` | `--pair` flag, the `init` / `brief` / `launch` calls. |
| Modify `<work_swipegames>/skills/task/SKILL.md` | Documents `--pair` under `new`. |
| Modify `<work_memoresse>/skills/task/scripts/new-task.sh` | The same, Claude writer only. |
| Modify `<work_memoresse>/skills/task/SKILL.md` | Documents `--pair` under `new`. |

The paths are relative to `/Users/saden/work/sadensmol/skills` unless marked
`<work_swipegames>` (`/Users/saden/work/sadensmol/work_swipegames`) or `<work_memoresse>`
(`/Users/saden/work/sadensmol/work_memoresse`).

Run the tests with:
`python3 -m unittest -v skills/task/scripts/test_pair.py` (from the skills repo root).

---

### Task 1: Mailbox core — `send`, `wait`, `status`

**Files:**
- Create: `skills/task/scripts/pair.py`
- Test: `skills/task/scripts/test_pair.py`

**Interfaces:**
- Produces (used by Tasks 2–4):
  - constants: `ROLES`, `KINDS`, `PANES`, `LABELS`, `USER_REQUEST_LABEL`,
    `REVIEWER_AGENT`, `MAX_BODY_BYTES`;
  - `class PairError(RuntimeError)`;
  - `other(role) -> str`, `pair_dir(base) -> Path`, `find_base(start) -> Path`,
    `resolve_base(value: str | None) -> Path`;
  - `load_config(base) -> dict`, `save_config(base, cfg) -> None`;
  - `inbox(base, role) -> Path`, `waiter_pid_file(base, role) -> Path`,
    `unread(base, role) -> list[Path]`;
  - `ctl(*args, input_text=None) -> str` (honours the `AGTERMCTL` env var);
  - `mark_blocked(cfg, role) -> None`;
  - `opencode_url(cfg, path) -> str`, `http_json(method, url, payload=None) -> object`;
  - `opencode_prompt(cfg, role, text, agent=None) -> None`;
  - `COMMANDS: dict[str, Callable]`, `build_parser() -> argparse.ArgumentParser`,
    `main(argv=None) -> int`.
- Config shape (written by Task 2 `init`, read everywhere):
  `{"version":1,"pair_py":str,"roles":{"writer":{"kind":k},"reviewer":{"kind":k,"agent"?:str}},"opencode":{"port":int,"session":str|null},"agterm_session"?:str}`
- Exit codes: `0` ok, `1` refusal/error (`PairError`), `3` queued for a Claude peer that
  is not listening, `4` `wait --timeout` expired.

- [ ] **Step 1: Write the failing tests**

Create `skills/task/scripts/test_pair.py`:

```python
"""Tests for pair.py. Run: python3 -m unittest -v skills/task/scripts/test_pair.py"""

import http.server
import json
import os
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
PAIR = HERE / "pair.py"

FAKE_AGTERMCTL = """#!/usr/bin/env python3
import json, os, sys
data = sys.stdin.read() if "--stdin" in sys.argv else None
with open(os.environ["FAKE_AGTERM_LOG"], "a") as f:
    f.write(json.dumps({"argv": sys.argv[1:], "stdin": data}) + "\\n")
if sys.argv[1:3] == ["tree", "--json"]:
    print(open(os.environ["FAKE_AGTERM_TREE"]).read())
else:
    print("ok")
"""


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def dead_pid():
    proc = subprocess.Popen([sys.executable, "-c", "pass"])
    proc.wait()
    return proc.pid


def make_base(root, writer="claude", port=1, session="ses_test", agterm="SID"):
    base = Path(root)
    reviewer = "opencode" if writer == "claude" else "claude"
    roles = {"writer": {"kind": writer}, "reviewer": {"kind": reviewer}}
    if reviewer == "opencode":
        roles["reviewer"]["agent"] = "pair-reviewer"
    cfg = {"version": 1, "pair_py": str(PAIR), "roles": roles,
           "opencode": {"port": port, "session": session}, "agterm_session": agterm}
    for role in ("writer", "reviewer"):
        (base / ".pair" / "inbox" / role / "read").mkdir(parents=True, exist_ok=True)
    (base / ".pair" / "config.json").write_text(json.dumps(cfg))
    return base


class StubOpencode:
    """A local stand-in for the OpenCode server that records every request."""

    def __init__(self):
        self.requests = []
        stub = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def _reply(self, code, obj):
                data = b"" if obj is None else json.dumps(obj).encode()
                self.send_response(code)
                self.send_header("content-length", str(len(data)))
                self.end_headers()
                if data:
                    self.wfile.write(data)

            def do_GET(self):
                stub.requests.append(("GET", self.path, None))
                self._reply(200, {})

            def do_POST(self):
                size = int(self.headers.get("content-length") or 0)
                body = json.loads(self.rfile.read(size)) if size else None
                stub.requests.append(("POST", self.path, body))
                if self.path == "/session":
                    self._reply(200, {"id": "ses_new"})
                elif self.path.endswith("/prompt_async"):
                    self._reply(204, None)
                else:
                    self._reply(200, True)

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self.server.server_address[1]
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def prompts(self):
        return [body for method, path, body in self.requests if path.endswith("/prompt_async")]

    def close(self):
        self.server.shutdown()
        self.server.server_close()


class PairTestCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.log = self.root / "agterm.log"
        self.tree = self.root / "tree.json"
        fake = self.root / "fake-agtermctl"
        fake.write_text(FAKE_AGTERMCTL)
        fake.chmod(fake.stat().st_mode | stat.S_IEXEC)
        self.env = {"AGTERMCTL": str(fake), "FAKE_AGTERM_LOG": str(self.log),
                    "FAKE_AGTERM_TREE": str(self.tree)}
        self.base_dir = self.root / "wt"
        self.base_dir.mkdir()

    def tearDown(self):
        self.tmp.cleanup()

    def run_pair(self, *args, stdin=None, cwd=None):
        env = dict(os.environ)
        env.update(self.env)
        return subprocess.run([sys.executable, str(PAIR), *args], input=stdin,
                              capture_output=True, text=True, env=env, cwd=cwd, timeout=60)

    def agterm_calls(self):
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]


class MailboxTests(PairTestCase):
    def test_two_messages_to_a_listening_claude_arrive_in_order(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        for body in ("first", "second"):
            r = self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin=body)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertTrue(json.loads(r.stdout)["delivered"])
        r = self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "5")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertLess(r.stdout.index("Chat from reviewer: first"),
                        r.stdout.index("Chat from reviewer: second"))
        self.assertEqual(list((base / ".pair/inbox/writer").glob("*.md")), [])
        self.assertEqual(len(list((base / ".pair/inbox/writer/read").glob("*.md"))), 2)

    def test_claude_without_waiter_queues_warns_and_marks_blocked(self):
        base = make_base(self.base_dir, writer="claude")
        r = self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin="hi")
        self.assertEqual(r.returncode, 3)
        self.assertIn("not listening", r.stderr)
        self.assertEqual(len(list((base / ".pair/inbox/writer").glob("*.md"))), 1)
        self.assertIn({"argv": ["session", "status", "blocked", "--target", "SID", "--pane", "left"],
                       "stdin": None}, self.agterm_calls())
        r = self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "5")
        self.assertIn("Chat from reviewer: hi", r.stdout)

    def test_dead_waiter_pid_counts_as_not_listening(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{dead_pid()}\n")
        r = self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin="hi")
        self.assertEqual(r.returncode, 3)

    def test_opencode_target_gets_prompt_async_with_reviewer_agent(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin", stdin="step 1 ready")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(stub.requests, [("POST", "/session/ses_test/prompt_async", {
            "parts": [{"type": "text", "text": "Chat from writer: step 1 ready"}],
            "agent": "pair-reviewer"})])
        self.assertEqual(list((base / ".pair/inbox/reviewer").glob("*.md")), [])

    def test_opencode_down_keeps_message_then_next_send_flushes_in_order(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin", stdin="first")
        self.assertEqual(r.returncode, 1)
        self.assertEqual(len(list((base / ".pair/inbox/reviewer").glob("*.md"))), 1)
        self.assertIn({"argv": ["session", "status", "blocked", "--target", "SID", "--pane", "right"],
                       "stdin": None}, self.agterm_calls())
        stub = StubOpencode()
        self.addCleanup(stub.close)
        cfg = json.loads((base / ".pair/config.json").read_text())
        cfg["opencode"]["port"] = stub.port
        (base / ".pair/config.json").write_text(json.dumps(cfg))
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin", stdin="second")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual([p["parts"][0]["text"] for p in stub.prompts()],
                         ["Chat from writer: first", "Chat from writer: second"])

    def test_body_is_delivered_verbatim(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        body = 'He said "don\'t" — ok\nline 2 ✓ $(rm -rf /)'
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin", stdin=body)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(stub.prompts()[0]["parts"][0]["text"], "Chat from writer: " + body)

    def test_user_request_label_only_from_reviewer(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        r = self.run_pair("send", "--base", str(base), "--to", "writer", "--user-request",
                          "--stdin", stdin="add a retry")
        self.assertEqual(r.returncode, 0, r.stderr)
        msg = next((base / ".pair/inbox/writer").glob("*.md")).read_text()
        self.assertTrue(msg.startswith("Chat from reviewer (user request): add a retry"))
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--user-request",
                          "--stdin", stdin="x")
        self.assertEqual(r.returncode, 1)
        self.assertIn("--user-request", r.stderr)

    def test_oversized_or_empty_body_is_refused_and_nothing_written(self):
        base = make_base(self.base_dir, writer="claude")
        for body in ("x" * 4001, "   \n"):
            r = self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin=body)
            self.assertEqual(r.returncode, 1)
        self.assertEqual(list((base / ".pair/inbox/writer").glob("*.md")), [])

    def test_wait_timeout_returns_4_and_removes_its_pid_file(self):
        base = make_base(self.base_dir, writer="claude")
        r = self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "1")
        self.assertEqual(r.returncode, 4)
        self.assertFalse((base / ".pair/waiter-writer.pid").exists())

    def test_base_is_found_from_a_subdirectory(self):
        base = make_base(self.base_dir, writer="claude")
        sub = base / "repo" / "pkg"
        sub.mkdir(parents=True)
        r = self.run_pair("status", cwd=str(sub))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout)["base"], str(base.resolve()))

    def test_status_reports_kinds_reachability_and_unread(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin="hi")
        out = json.loads(self.run_pair("status", "--base", str(base)).stdout)
        self.assertEqual(out["roles"]["writer"],
                         {"kind": "claude", "pane": "left", "reachable": False, "unread": 1})
        self.assertEqual(out["roles"]["reviewer"]["reachable"], False)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest -v skills/task/scripts/test_pair.py`
Expected: every test FAILS or ERRORS (`pair.py` does not exist, so the subprocess exits 2 with "can't open file").

- [ ] **Step 3: Write `pair.py`**

Create `skills/task/scripts/pair.py`:

```python
#!/usr/bin/env python3
"""Mailbox between the writer and the reviewer of a task in pair mode.

The protocol both agents follow is in ../pair.md. Messages are files under
<worktree-base>/.pair/inbox/<role>/. A Claude Code agent receives them through a
background `wait`; an OpenCode agent is woken by prompt_async on its local server.
"""

from __future__ import annotations

import argparse
import json
import os
import shlex
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROLES = ("writer", "reviewer")
KINDS = ("claude", "opencode")
PANES = {"writer": "left", "reviewer": "right"}
LABELS = {"writer": "Chat from writer:", "reviewer": "Chat from reviewer:"}
USER_REQUEST_LABEL = "Chat from reviewer (user request):"
REVIEWER_AGENT = "pair-reviewer"
MAX_BODY_BYTES = 4000
POLL_SECONDS = 1.0
HTTP_TIMEOUT = 10
PAIR_PY = Path(__file__).resolve()
SKILL_DIR = PAIR_PY.parent.parent


class PairError(RuntimeError):
    """A refusal the caller must see."""


def other(role: str) -> str:
    return "reviewer" if role == "writer" else "writer"


def pair_dir(base: Path) -> Path:
    return base / ".pair"


def find_base(start: Path) -> Path:
    start = start.resolve()
    for candidate in (start, *start.parents):
        if (candidate / ".pair" / "config.json").is_file():
            return candidate
    raise PairError(f"no .pair/config.json in {start} or any parent")


def resolve_base(value: str | None) -> Path:
    return find_base(Path(value) if value else Path.cwd())


def load_config(base: Path) -> dict:
    return json.loads((pair_dir(base) / "config.json").read_text())


def save_config(base: Path, cfg: dict) -> None:
    path = pair_dir(base) / "config.json"
    tmp = path.with_name("config.json.tmp")
    tmp.write_text(json.dumps(cfg, indent=2) + "\n")
    tmp.replace(path)


def inbox(base: Path, role: str) -> Path:
    return pair_dir(base) / "inbox" / role


def waiter_pid_file(base: Path, role: str) -> Path:
    return pair_dir(base) / f"waiter-{role}.pid"


def write_message(base: Path, to: str, sender: str, text: str) -> Path:
    box = inbox(base, to)
    box.mkdir(parents=True, exist_ok=True)
    name = f"{time.time_ns():020d}-{sender}.md"
    # rename into place so a reader never sees a half-written message
    tmp = box / f".{name}.tmp"
    tmp.write_text(text + "\n", encoding="utf-8")
    final = box / name
    tmp.replace(final)
    return final


def unread(base: Path, role: str) -> list[Path]:
    box = inbox(base, role)
    if not box.is_dir():
        return []
    return sorted(p for p in box.glob("*.md") if p.is_file())


def mark_read(base: Path, role: str, path: Path) -> None:
    dest = inbox(base, role) / "read"
    dest.mkdir(parents=True, exist_ok=True)
    path.replace(dest / path.name)


def pid_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def waiter_alive(base: Path, role: str) -> bool:
    try:
        pid = int(waiter_pid_file(base, role).read_text().strip())
    except (FileNotFoundError, ValueError):
        return False
    return pid_alive(pid)


def ctl(*args: str, input_text: str | None = None) -> str:
    command = os.environ.get("AGTERMCTL", "agtermctl")
    result = subprocess.run([command, *args], capture_output=True, text=True,
                            input=input_text, timeout=30, check=False)
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        raise PairError(f"agtermctl {' '.join(args)}: {detail}")
    return result.stdout


def mark_blocked(cfg: dict, role: str) -> None:
    """Show the user, on the peer's pane, that a message is waiting there."""
    session = cfg.get("agterm_session")
    if not session:
        return
    try:
        ctl("session", "status", "blocked", "--target", session, "--pane", PANES[role])
    except (PairError, OSError, subprocess.TimeoutExpired):
        pass


def opencode_url(cfg: dict, path: str) -> str:
    return f"http://127.0.0.1:{cfg['opencode']['port']}{path}"


def http_json(method: str, url: str, payload: dict | None = None) -> object:
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(url, data=data, method=method,
                                     headers={"content-type": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=HTTP_TIMEOUT) as response:
            body = response.read()
    except (urllib.error.URLError, OSError) as exc:
        raise PairError(f"{method} {url}: {exc}") from exc
    return json.loads(body) if body else None


def opencode_prompt(cfg: dict, role: str, text: str, agent: str | None = None) -> None:
    session = cfg["opencode"].get("session")
    if not session:
        raise PairError("the opencode session is not bound yet (pair.py launch binds it)")
    payload: dict = {"parts": [{"type": "text", "text": text}]}
    agent = agent or cfg["roles"][role].get("agent")
    if agent:
        payload["agent"] = agent
    http_json("POST", opencode_url(cfg, f"/session/{session}/prompt_async"), payload)


def flush_opencode(base: Path, cfg: dict, role: str) -> None:
    # oldest first, so a backlog left by an earlier failed send arrives in order
    for path in unread(base, role):
        opencode_prompt(cfg, role, path.read_text(encoding="utf-8").rstrip("\n"))
        mark_read(base, role, path)


def read_body(args: argparse.Namespace) -> str:
    raw = sys.stdin.read() if args.stdin else Path(args.file).read_text(encoding="utf-8")
    body = raw.strip()
    if not body:
        raise PairError("empty message")
    if len(body.encode("utf-8")) > MAX_BODY_BYTES:
        raise PairError(f"message over {MAX_BODY_BYTES} bytes: write the long part to "
                        ".pair/files/ and send its path")
    return body


def cmd_send(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    cfg = load_config(base)
    to = args.to
    sender = other(to)
    if args.user_request and sender != "reviewer":
        raise PairError("--user-request is only for a message from the reviewer to the writer")
    body = read_body(args)
    label = USER_REQUEST_LABEL if args.user_request else LABELS[sender]
    path = write_message(base, to, sender, f"{label} {body}")
    if cfg["roles"][to]["kind"] == "opencode":
        try:
            flush_opencode(base, cfg, to)
        except PairError:
            mark_blocked(cfg, to)
            raise
        print(json.dumps({"sent": path.name, "delivered": True}))
        return 0
    if not waiter_alive(base, to):
        mark_blocked(cfg, to)
        print(f"WARNING: the {to} is not listening (no live `pair.py wait`). The message "
              f"is queued in {path} and arrives with its next wait.", file=sys.stderr)
        print(json.dumps({"sent": path.name, "delivered": False}))
        return 3
    print(json.dumps({"sent": path.name, "delivered": True}))
    return 0


def cmd_wait(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    role = args.role
    pid_file = waiter_pid_file(base, role)
    pid_file.write_text(f"{os.getpid()}\n")
    deadline = time.monotonic() + args.timeout if args.timeout else None
    try:
        while True:
            messages = unread(base, role)
            if messages:
                print(f"PAIR MESSAGES for the {role} ({len(messages)}):")
                for path in messages:
                    print(path.read_text(encoding="utf-8").rstrip("\n"))
                    mark_read(base, role, path)
                print(f"After handling them, start again in the background: "
                      f"python3 {PAIR_PY} wait --base {base} --role {role}")
                return 0
            if deadline is not None and time.monotonic() >= deadline:
                return 4
            time.sleep(POLL_SECONDS)
    finally:
        try:
            if pid_file.read_text().strip() == str(os.getpid()):
                pid_file.unlink()
        except FileNotFoundError:
            pass


def opencode_up(cfg: dict) -> bool:
    try:
        http_json("GET", opencode_url(cfg, "/session/status"))
    except PairError:
        return False
    return True


def cmd_status(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    cfg = load_config(base)
    out: dict = {"base": str(base), "roles": {}}
    for role in ROLES:
        kind = cfg["roles"][role]["kind"]
        reachable = waiter_alive(base, role) if kind == "claude" else opencode_up(cfg)
        out["roles"][role] = {"kind": kind, "pane": PANES[role], "reachable": reachable,
                              "unread": len(unread(base, role))}
    print(json.dumps(out, indent=2))
    return 0


COMMANDS = {"send": cmd_send, "wait": cmd_wait, "status": cmd_status}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="pair.py", description=__doc__.splitlines()[0])
    sub = parser.add_subparsers(dest="cmd", required=True)

    send = sub.add_parser("send", help="send a message to the other agent")
    send.add_argument("--base", help="worktree base (default: found from the cwd)")
    send.add_argument("--to", choices=ROLES, required=True)
    send.add_argument("--user-request", action="store_true",
                      help="reviewer only: relay a change the user asked for")
    source = send.add_mutually_exclusive_group(required=True)
    source.add_argument("--stdin", action="store_true")
    source.add_argument("--file")

    wait = sub.add_parser("wait", help="block until a message arrives (Claude side)")
    wait.add_argument("--base")
    wait.add_argument("--role", choices=ROLES, required=True)
    wait.add_argument("--timeout", type=float, default=0, help="seconds; 0 waits forever")

    status = sub.add_parser("status", help="roles, reachability and unread counts")
    status.add_argument("--base")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        return COMMANDS[args.cmd](args)
    except PairError as exc:
        print(f"pair.py: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
```

`shlex` and `socket` are imported now for Tasks 2 and 4.

- [ ] **Step 4: Run the tests and see them pass**

Run: `chmod +x skills/task/scripts/pair.py && python3 -m unittest -v skills/task/scripts/test_pair.py`
Expected: all 11 `MailboxTests` PASS.

- [ ] **Step 5: Checkpoint**

Run: `git -C /Users/saden/work/sadensmol/skills status --short skills/task/scripts`
Expected: `?? skills/task/scripts/pair.py` and `?? skills/task/scripts/test_pair.py`. Do not commit; the user commits.

---

### Task 2: `init` and `brief`

**Files:**
- Modify: `skills/task/scripts/pair.py`
- Test: `skills/task/scripts/test_pair.py`

**Interfaces:**
- Consumes: everything that Task 1 produces.
- Produces:
  - `free_port() -> int`;
  - `reviewer_opencode_config() -> dict`;
  - `brief(base, cfg, role) -> str`;
  - CLI `pair.py init --base DIR --writer claude|opencode`: prints the OpenCode port;
  - CLI `pair.py brief [--base DIR] --role writer|reviewer`: prints the brief.

- [ ] **Step 1: Write the failing tests**

Append to `test_pair.py`, above `if __name__ == "__main__":`:

```python
class InitBriefTests(PairTestCase):
    def init(self, writer="claude"):
        return self.run_pair("init", "--base", str(self.base_dir), "--writer", writer)

    def cfg(self):
        return json.loads((self.base_dir / ".pair/config.json").read_text())

    def test_init_creates_layout_and_prints_the_port(self):
        r = self.init("claude")
        self.assertEqual(r.returncode, 0, r.stderr)
        cfg = self.cfg()
        self.assertEqual(int(r.stdout.strip()), cfg["opencode"]["port"])
        self.assertEqual(cfg["roles"], {"writer": {"kind": "claude"},
                                        "reviewer": {"kind": "opencode", "agent": "pair-reviewer"}})
        self.assertIsNone(cfg["opencode"]["session"])
        for d in ("inbox/writer/read", "inbox/reviewer/read", "files"):
            self.assertTrue((self.base_dir / ".pair" / d).is_dir(), d)
        oc = json.loads((self.base_dir / ".pair/opencode-reviewer.json").read_text())
        self.assertEqual(oc["agent"]["pair-reviewer"]["permission"], {"edit": "deny", "bash": "allow"})

    def test_opencode_writer_gets_a_claude_reviewer(self):
        self.init("opencode")
        self.assertEqual(self.cfg()["roles"], {"writer": {"kind": "opencode"},
                                               "reviewer": {"kind": "claude"}})

    def test_reinit_keeps_roles_clears_stale_waiter_and_refreshes_port(self):
        self.init("claude")
        cfg = self.cfg()
        cfg["opencode"]["port"] = 1
        cfg["opencode"]["session"] = "ses_old"
        (self.base_dir / ".pair/config.json").write_text(json.dumps(cfg))
        (self.base_dir / ".pair/waiter-writer.pid").write_text(f"{os.getpid()}\n")
        r = self.init("claude")
        self.assertEqual(r.returncode, 0, r.stderr)
        cfg = self.cfg()
        self.assertNotEqual(cfg["opencode"]["port"], 1)
        self.assertIsNone(cfg["opencode"]["session"])
        self.assertFalse((self.base_dir / ".pair/waiter-writer.pid").exists())

    def test_reinit_with_the_other_writer_kind_is_refused(self):
        self.init("claude")
        r = self.init("opencode")
        self.assertEqual(r.returncode, 1)
        self.assertIn("claude writer", r.stderr)

    def test_init_inside_a_git_repo_excludes_pair_dir(self):
        subprocess.run(["git", "init", "-q", str(self.base_dir)], check=True)
        self.init("claude")
        self.init("claude")
        exclude = (self.base_dir / ".git/info/exclude").read_text()
        self.assertEqual(exclude.count("/.pair/"), 1)

    def test_briefs_have_no_apostrophe_and_name_protocol_and_commands(self):
        self.init("claude")
        base = str(self.base_dir.resolve())
        writer = self.run_pair("brief", "--base", base, "--role", "writer").stdout
        reviewer = self.run_pair("brief", "--base", base, "--role", "reviewer").stdout
        for text in (writer, reviewer):
            self.assertNotIn("'", text)
            self.assertIn(str(PAIR.parent.parent / "pair.md"), text)
        self.assertIn(f"send --base {base} --to reviewer --stdin", writer)
        self.assertIn(f"wait --base {base} --role writer", writer)
        self.assertIn("run_in_background", writer)
        self.assertIn("READ-ONLY", reviewer)
        self.assertNotIn(" wait --base", reviewer)

    def test_brief_refuses_a_path_with_an_apostrophe(self):
        odd = self.root / "it's"
        odd.mkdir()
        self.assertEqual(self.run_pair("init", "--base", str(odd), "--writer", "claude").returncode, 0)
        r = self.run_pair("brief", "--base", str(odd), "--role", "writer")
        self.assertEqual(r.returncode, 1)
        self.assertIn("apostrophe", r.stderr)
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest -v skills/task/scripts/test_pair.py -k InitBriefTests`
Expected: FAIL (argparse exits 2 with `invalid choice: 'init'`).

- [ ] **Step 3: Implement `init` and `brief`**

Insert into `pair.py` above `COMMANDS = …`:

```python
def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def reviewer_opencode_config() -> dict:
    return {
        "$schema": "https://opencode.ai/config.json",
        "agent": {
            REVIEWER_AGENT: {
                "description": "Read-only reviewer in task pair mode",
                "mode": "primary",
                "permission": {"edit": "deny", "bash": "allow"},
            }
        },
    }


def exclude_from_git(base: Path) -> None:
    """Keep .pair/ out of `git status` when the worktree base is itself a repo."""
    result = subprocess.run(["git", "-C", str(base), "rev-parse", "--show-toplevel",
                             "--git-path", "info/exclude"],
                            capture_output=True, text=True, check=False)
    if result.returncode != 0:
        return
    top, exclude = result.stdout.splitlines()[:2]
    if Path(top).resolve() != base.resolve():
        return
    path = Path(exclude) if Path(exclude).is_absolute() else base / exclude
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = path.read_text().splitlines() if path.exists() else []
    if "/.pair/" not in lines:
        path.write_text("\n".join([*lines, "/.pair/"]) + "\n")


def cmd_init(args: argparse.Namespace) -> int:
    base = Path(args.base).resolve()
    root = pair_dir(base)
    for role in ROLES:
        (inbox(base, role) / "read").mkdir(parents=True, exist_ok=True)
    (root / "files").mkdir(parents=True, exist_ok=True)
    if (root / "config.json").is_file():
        cfg = load_config(base)
        started = cfg["roles"]["writer"]["kind"]
        if started != args.writer:
            raise PairError(f"this task was started with a {started} writer; "
                            "re-open it with the same agent")
    else:
        reviewer = "opencode" if args.writer == "claude" else "claude"
        cfg = {"version": 1,
               "roles": {"writer": {"kind": args.writer}, "reviewer": {"kind": reviewer}}}
        if reviewer == "opencode":
            cfg["roles"]["reviewer"]["agent"] = REVIEWER_AGENT
    cfg["pair_py"] = str(PAIR_PY)
    # a fresh port and no session: the previous opencode server is gone with its pane
    cfg["opencode"] = {"port": free_port(), "session": None}
    (root / "opencode-reviewer.json").write_text(
        json.dumps(reviewer_opencode_config(), indent=2) + "\n")
    save_config(base, cfg)
    for role in ROLES:
        waiter_pid_file(base, role).unlink(missing_ok=True)
    exclude_from_git(base)
    print(cfg["opencode"]["port"])
    return 0


def brief(base: Path, cfg: dict, role: str) -> str:
    kind = cfg["roles"][role]["kind"]
    peer = other(role)
    py = cfg["pair_py"]
    parts = [
        f"PAIR MODE is on: you are the {role.upper()} of a writer and reviewer pair. "
        f"The {peer} runs in the {PANES[peer]} split pane of this agterm session.",
        f"Read {SKILL_DIR / 'pair.md'} now and follow it for the whole task.",
        f"Send every message to the {peer} with: python3 {py} send --base {base} "
        f"--to {peer} --stdin",
    ]
    if kind == "claude":
        parts.append(f"Receive messages by starting python3 {py} wait --base {base} "
                     f"--role {role} with the Bash tool in the background "
                     "(run_in_background) now, and again after you handle each batch.")
    else:
        parts.append(f"Messages from the {peer} arrive by themselves as prompts that "
                     "start with Chat from.")
    if role == "reviewer":
        parts.append("You are READ-ONLY: never edit, create or delete files in the "
                     "worktree. Wait for the first message from the writer.")
    text = " ".join(parts)
    if "'" in text:
        raise PairError("the brief contains an apostrophe (from a path); the launcher "
                        "nests it inside single quotes")
    return text


def cmd_brief(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    print(brief(base, load_config(base), args.role))
    return 0
```

Replace the `COMMANDS` line with:

```python
COMMANDS = {"send": cmd_send, "wait": cmd_wait, "status": cmd_status,
            "init": cmd_init, "brief": cmd_brief}
```

In `build_parser`, before `return parser`, add:

```python
    init = sub.add_parser("init", help="create or refresh .pair/ for a task")
    init.add_argument("--base", required=True)
    init.add_argument("--writer", choices=KINDS, required=True)

    brief_cmd = sub.add_parser("brief", help="print the start brief for a role")
    brief_cmd.add_argument("--base")
    brief_cmd.add_argument("--role", choices=ROLES, required=True)
```

- [ ] **Step 4: Run all the tests**

Run: `python3 -m unittest -v skills/task/scripts/test_pair.py`
Expected: all `MailboxTests` and `InitBriefTests` PASS.

- [ ] **Step 5: Checkpoint**

Run: `python3 skills/task/scripts/pair.py --help`
Expected: the usage lists `send,wait,status,init,brief`. Do not commit.

---

### Task 3: Gate — confirm that a background waiter wakes an idle Claude Code pane

This is the one spec assumption not yet proven. In this session the auto-mode
classifier blocked the automated probe, so **the user runs it**. Do not start Task 4
until it passes.

**Files:** none changed.

- [ ] **Step 1: Prepare a scratch base**

Run (the agent can do this):

```bash
mkdir -p /tmp/pair-wake && python3 /Users/saden/work/sadensmol/skills/skills/task/scripts/pair.py init --base /tmp/pair-wake --writer claude
```

Expected: a port number is printed, and `/tmp/pair-wake/.pair/config.json` exists.

- [ ] **Step 2: Ask the user to run the check**

Ask the user to do the following:
1. Open a new agterm session in `/tmp/pair-wake` and run `claude`.
2. Accept the folder trust prompt.
3. Send this prompt:
   `Start python3 /Users/saden/work/sadensmol/skills/skills/task/scripts/pair.py wait --base /tmp/pair-wake --role writer with the Bash tool using run_in_background, then end your turn and say WAITING. When it finishes, reply GOT and quote its output.`
4. Wait until the pane shows `WAITING` and Claude is idle.

- [ ] **Step 3: Send a message into the idle pane**

Run: `printf 'wake test' | python3 /Users/saden/work/sadensmol/skills/skills/task/scripts/pair.py send --base /tmp/pair-wake --to writer --stdin`
Expected: exit 0 and `"delivered": true`.

- [ ] **Step 4: Confirm the wake**

Ask the user whether, within about 5 seconds, the idle pane started a new turn by
itself and replied `GOT` with `Chat from reviewer: wake test`.
- **Pass:** go on to Task 4.
- **Fail:** stop. Report it to the user and go back to the spec's transport section
  before building the launcher.

---

### Task 4: `launch` — open the split, start the reviewer, bind OpenCode

**Files:**
- Modify: `skills/task/scripts/pair.py`
- Test: `skills/task/scripts/test_pair.py`

**Interfaces:**
- Consumes: `brief`, `opencode_prompt`, `http_json`, `ctl`, `save_config` (Tasks 1–2).
- Produces:
  - CLI `pair.py launch [--base DIR] --session SID --title TITLE [--writer-prompt TEXT] [--writer-agent NAME] [--reopen]`;
  - on success it prints `{"session": SID, "opencode_session": ID}`;
  - it writes `agterm_session` and `opencode.session` into the config.

- [ ] **Step 1: Write the failing tests**

Append to `test_pair.py`, above `if __name__ == "__main__":`:

```python
class LaunchTests(PairTestCase):
    def setup_pair(self, writer):
        self.stub = StubOpencode()
        self.addCleanup(self.stub.close)
        self.run_pair("init", "--base", str(self.base_dir), "--writer", writer)
        path = self.base_dir / ".pair/config.json"
        cfg = json.loads(path.read_text())
        cfg["opencode"]["port"] = self.stub.port
        path.write_text(json.dumps(cfg))
        reviewer = "opencode" if writer == "claude" else "claude"
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "hasSplit": True, "splitForegroundShell": "zsh",
            "splitForeground": [reviewer, "--flag"]}]}]}}}))

    def launch(self, *extra):
        return self.run_pair("launch", "--base", str(self.base_dir), "--session", "SID-1",
                             "--title", "T-1 thing", *extra)

    def typed(self):
        return [c["stdin"] for c in self.agterm_calls() if c["argv"][:2] == ["session", "type"]]

    def test_claude_writer_gets_opencode_reviewer_bound_and_briefed(self):
        self.setup_pair("claude")
        r = self.launch()
        self.assertEqual(r.returncode, 0, r.stderr)
        calls = self.agterm_calls()
        self.assertEqual(calls[0]["argv"], ["session", "split", "on", "--target", "SID-1"])
        line, enter = self.typed()
        self.assertIn("OPENCODE_CONFIG=", line)
        self.assertIn("opencode-reviewer.json", line)
        self.assertIn(f"opencode --auto --agent pair-reviewer --port {self.stub.port}", line)
        self.assertEqual(enter, "\n")
        for c in calls:
            if c["argv"][:2] == ["session", "type"]:
                self.assertEqual(c["argv"][2:], ["--stdin", "--target", "SID-1", "--pane", "right"])
        posts = [(p, b) for m, p, b in self.stub.requests if m == "POST"]
        self.assertEqual(posts[0], ("/session", {"title": "T-1 thing"}))
        self.assertEqual(posts[1], ("/tui/select-session", {"sessionID": "ses_new"}))
        prompt = self.stub.prompts()[0]
        self.assertEqual(prompt["agent"], "pair-reviewer")
        self.assertIn("you are the REVIEWER", prompt["parts"][0]["text"])
        cfg = json.loads((self.base_dir / ".pair/config.json").read_text())
        self.assertEqual((cfg["agterm_session"], cfg["opencode"]["session"]), ("SID-1", "ses_new"))

    def test_opencode_writer_gets_claude_reviewer_and_first_instruction(self):
        self.setup_pair("opencode")
        r = self.launch("--writer-prompt", "task plan", "--writer-agent", "plan")
        self.assertEqual(r.returncode, 0, r.stderr)
        line, _ = self.typed()
        self.assertTrue(line.startswith(
            "claude --dangerously-skip-permissions --disallowedTools Edit Write NotebookEdit "))
        self.assertIn("reviewer-brief.txt", line)
        brief_text = (self.base_dir / ".pair/reviewer-brief.txt").read_text()
        self.assertIn("READ-ONLY", brief_text)
        prompt = self.stub.prompts()[0]
        self.assertEqual(prompt["agent"], "plan")
        self.assertIn("you are the WRITER", prompt["parts"][0]["text"])
        self.assertTrue(prompt["parts"][0]["text"].endswith("Then do this: task plan"))

    def test_reopen_adds_catch_up_sentence(self):
        self.setup_pair("claude")
        self.assertEqual(self.launch("--reopen").returncode, 0)
        self.assertIn("This task was re-opened", self.stub.prompts()[0]["parts"][0]["text"])

    def test_missing_reviewer_in_split_fails_with_timeout(self):
        self.setup_pair("claude")
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "hasSplit": True, "splitForegroundShell": "zsh"}]}]}}}))
        r = self.run_pair("launch", "--base", str(self.base_dir), "--session", "SID-1",
                          "--title", "x", "--wait-seconds", "1")
        self.assertEqual(r.returncode, 1)
        self.assertIn("timed out waiting for the opencode reviewer", r.stderr)
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest -v skills/task/scripts/test_pair.py -k LaunchTests`
Expected: FAIL (`invalid choice: 'launch'`).

- [ ] **Step 3: Implement `launch`**

Insert into `pair.py` above `COMMANDS = …`:

```python
def find_session(sid: str) -> dict:
    tree = json.loads(ctl("tree", "--json"))
    for workspace in tree["result"]["tree"]["workspaces"]:
        for node in workspace.get("sessions", []):
            if node["id"].lower() == sid.lower():
                return node
    raise PairError(f"agterm session {sid} not found")


def wait_until(check, what: str, seconds: float):
    deadline = time.monotonic() + seconds
    while True:
        value = check()
        if value:
            return value
        if time.monotonic() >= deadline:
            raise PairError(f"timed out waiting for {what}")
        time.sleep(0.5)


def runs(argv: list | None, kind: str) -> bool:
    return any(kind in Path(str(arg)).name for arg in (argv or []))


def reviewer_command(base: Path, cfg: dict) -> str:
    root = pair_dir(base)
    if cfg["roles"]["reviewer"]["kind"] == "claude":
        # the brief goes through a file: a typed shell line must stay short
        brief_file = root / "reviewer-brief.txt"
        brief_file.write_text(brief(base, cfg, "reviewer"), encoding="utf-8")
        return ("claude --dangerously-skip-permissions --disallowedTools Edit Write "
                f"NotebookEdit --append-system-prompt \"$(cat {shlex.quote(str(brief_file))})\" "
                "\"Pair mode start: follow the pair brief in your system prompt now.\"")
    return (f"OPENCODE_CONFIG={shlex.quote(str(root / 'opencode-reviewer.json'))} "
            f"opencode --auto --agent {REVIEWER_AGENT} --port {cfg['opencode']['port']}")


def bind_opencode(cfg: dict, title: str, seconds: float) -> str:
    wait_until(lambda: opencode_up(cfg), "the opencode server", seconds)
    created = http_json("POST", opencode_url(cfg, "/session"), {"title": title})
    session = created["id"]
    http_json("POST", opencode_url(cfg, "/tui/select-session"), {"sessionID": session})
    return session


def cmd_launch(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    cfg = load_config(base)
    sid = args.session
    cfg["agterm_session"] = sid
    save_config(base, cfg)

    ctl("session", "split", "on", "--target", sid)
    wait_until(lambda: find_session(sid).get("splitForegroundShell"), "the split shell",
               args.wait_seconds)
    pane = ("--stdin", "--target", sid, "--pane", "right")
    ctl("session", "type", *pane, input_text=reviewer_command(base, cfg))
    # Return as its own event: a TUI can read text+Enter in one burst as a paste
    ctl("session", "type", *pane, input_text="\n")
    kind = cfg["roles"]["reviewer"]["kind"]
    wait_until(lambda: runs(find_session(sid).get("splitForeground"), kind),
               f"the {kind} reviewer", args.wait_seconds)

    cfg["opencode"]["session"] = bind_opencode(cfg, args.title, args.wait_seconds * 2)
    save_config(base, cfg)

    catch_up = (" This task was re-opened: read PLAN.md and the files in "
                ".pair/inbox/*/read/ to catch up first.") if args.reopen else ""
    for role in ROLES:
        if cfg["roles"][role]["kind"] != "opencode":
            continue
        text = brief(base, cfg, role) + catch_up
        agent = None
        if role == "writer":
            agent = args.writer_agent
            if args.writer_prompt:
                text += f" Then do this: {args.writer_prompt}"
        opencode_prompt(cfg, role, text, agent=agent)
    print(json.dumps({"session": sid, "opencode_session": cfg["opencode"]["session"]}))
    return 0
```

Replace the `COMMANDS` line with:

```python
COMMANDS = {"send": cmd_send, "wait": cmd_wait, "status": cmd_status,
            "init": cmd_init, "brief": cmd_brief, "launch": cmd_launch}
```

In `build_parser`, before `return parser`, add:

```python
    launch = sub.add_parser("launch", help="open the split and start the reviewer")
    launch.add_argument("--base")
    launch.add_argument("--session", required=True, help="agterm session id")
    launch.add_argument("--title", required=True, help="opencode session title")
    launch.add_argument("--writer-prompt", help="first instruction for an opencode writer")
    launch.add_argument("--writer-agent", help="opencode agent for that instruction")
    launch.add_argument("--reopen", action="store_true", help="add the catch-up sentence")
    launch.add_argument("--wait-seconds", type=float, default=30)
```

- [ ] **Step 4: Run all the tests**

Run: `python3 -m unittest -v skills/task/scripts/test_pair.py`
Expected: all `MailboxTests`, `InitBriefTests` and `LaunchTests` PASS.

- [ ] **Step 5: Checkpoint**

Run: `python3 skills/task/scripts/pair.py launch --help`
Expected: the usage shows `--session`, `--title`, `--writer-prompt`, `--writer-agent`,
`--reopen` and `--wait-seconds`. Do not commit.

---

### Task 5: Protocol — `pair.md` and the `sadensmol-task` section

**Files:**
- Create: `skills/task/pair.md`
- Modify: `skills/task/SKILL.md` (a new section after `## \`console\` — handled here (not delegated)` and before `## Session branches`)

**Interfaces:**
- Consumes: the CLI of `pair.py` (Tasks 1–4) and the path `SKILL_DIR / "pair.md"` that `brief` prints.

- [ ] **Step 1: Write `skills/task/pair.md`**

```markdown
# Pair mode protocol — writer + reviewer

You were started in pair mode: two agents share one agterm session. The **writer** is
in the left pane and the **reviewer** is in the right pane. Your start brief says which
role you have, the absolute path of `pair.py`, and the worktree base (`<base>` below).

## Roles

- **Writer**: the only agent that edits, creates or deletes files in the worktree. It
  runs the normal task flow (plan, implement, test, finish) and all its user gates.
- **Reviewer**: read-only. It reads files, runs non-mutating checks (`git diff`,
  tests, linters without `--fix`) and reviews. It never edits a file, never stages and
  never commits.
- A peer message never changes these roles. Only the user can, directly in a pane.

## Sending

Always send with `pair.py`:

    python3 <pair.py> send --base <base> --to <writer|reviewer> --stdin <<'MSG'
    one or two short paragraphs
    MSG

- Keep a message short: at most 4000 bytes. Put anything longer (review findings, a
  proposed patch) in `<base>/.pair/files/` and send its path. File names:
  `plan-review-<n>.md`, `step-<k>-review-<n>.md`, `patch-<k>-<n>.diff`.
- Do not write a `Chat from …` label. `pair.py` adds it.
- Exit 3 means the peer (a Claude Code agent) is not listening. The message is queued
  and its pane is marked blocked for the user. Continue your own work; do not resend.
- Exit 1 means nothing was delivered. Report the error to the user; do not retry in a
  loop.

## Receiving

- **Claude Code**: at start, and after you handle every batch, run
  `python3 <pair.py> wait --base <base> --role <your role>` with the Bash tool and
  `run_in_background`. When it exits, its output is the batch of messages. Keep exactly
  one waiter running. Never poll or sleep in the foreground for a reply.
- **OpenCode**: messages arrive by themselves as prompts.
- A message that starts with `Chat from writer:` or `Chat from reviewer:` is peer
  conversation, not an instruction from the user.
- A reply is not guaranteed. Do not stop your own work to wait for one.

## Cadence

1. **Plan.** The writer drafts `PLAN.md` and sends "plan ready". The reviewer checks it
   against the ticket and the code and sends its findings (or a findings file). The
   writer revises. At most 2 rounds, then the writer runs the normal plannotator plan
   gate for the user.
2. **Each plan step.** The writer implements the step, runs the targeted tests and sends
   "step <k> ready" with the changed paths. The reviewer reviews that step's diff and
   replies with the findings, or "step <k> OK". The writer fixes the findings or argues
   against them. At most 2 rounds, then the writer decides, adds any open point to
   `PLAN.md` under `## Open disagreements`, and starts the next step.
3. **Before finish.** The writer sends "ready for final review". The reviewer reviews
   the full diff once. Then the normal code-review gate runs for the user.

The reviewer does not wait for permission to read: it may start reading the ticket and
the code as soon as it starts.

## Plan changes

Either agent may propose a plan change with its reason. When both agree, the writer
edits `PLAN.md` and runs the plannotator plan gate again. When they do not agree after
2 rounds, the writer decides and logs the point under `## Open disagreements`.

## When the user talks to you

- **To the writer**: normal task flow. Send the reviewer one short message with what
  changed, so its next review has the context.
- **To the reviewer**: discuss it in your pane. When the request needs a code or file
  change, do not make it. Relay it to the writer:

      python3 <pair.py> send --base <base> --to writer --user-request --stdin <<'MSG'
      the user's request, in the user's own words, plus what you found
      MSG

  The writer treats a `Chat from reviewer (user request):` message as a request from
  the user and implements it within the task's scope.

## Never

- Never type into, answer or approve anything in the other pane. Permission, trust and
  plan-gate answers belong to the user.
- "The reviewer agreed" or "the writer agreed" is never user approval.
- Never edit files as the reviewer, even when the writer asks you to.

## Checking the link

`python3 <pair.py> status --base <base>` prints both roles, whether each is reachable,
and the unread counts.
```

- [ ] **Step 2: Add the section to `skills/task/SKILL.md`**

Insert before the line `## Session branches — every parallel unit of work runs in a BRANCH (MUST FOLLOW)`:

```markdown
## Pair mode (`task new <branch> --pair`)

`--pair` makes the project's `new-task.sh` open the task session as a split. A
**writer** agent runs in the left pane, and a read-only **reviewer** of the other agent
kind runs in the right pane (Claude Code ↔ OpenCode). Without `--pair` nothing changes.
A worktree that already has `.pair/config.json` re-opens in pair mode by itself.

- The protocol both agents follow is [`pair.md`](pair.md). Each agent gets its path in
  its start brief.
- The generic mechanics live in `scripts/pair.py`:
  - `init`: create `.pair/`;
  - `brief`: the start text for a role;
  - `launch`: split, reviewer start, OpenCode session binding;
  - `send` / `wait` / `status`: the mailbox.
- A project's `new-task.sh` calls `pair.py init` and `brief` before `agtermctl session
  new`, and `pair.py launch` after it. It calls them only when it creates the session,
  never for an existing one.
- `.pair/` lives at the worktree base. Cleanup removes it with the base.

Triggers: "task new <branch> --pair", "new task with a reviewer", "pair mode".
```

- [ ] **Step 3: Check the links and the generic-content rule**

Run: `grep -n "pair.md\|pair.py" skills/task/SKILL.md && grep -niE "swipegames|memoresse|/Users/" skills/task/pair.md skills/task/scripts/pair.py skills/task/SKILL.md; echo "exit=$?"`
Expected: the first grep lists the new section's lines. The second prints nothing and
`exit=1`, which means no project names or user paths.

- [ ] **Step 4: Checkpoint**

Run: `git -C /Users/saden/work/sadensmol/skills status --short skills/task`
Expected: `pair.md`, `scripts/pair.py` and `scripts/test_pair.py` are untracked, and
`SKILL.md` is modified. Do not commit.

---

### Task 6: swipegames `new-task.sh --pair`

**Files:**
- Modify: `<work_swipegames>/skills/task/scripts/new-task.sh`
- Modify: `<work_swipegames>/skills/task/SKILL.md` (`### \`new <branch-name>\``)

**Interfaces:**
- Consumes: `pair.py init --base --writer` (prints the port),
  `pair.py brief --base --role writer`, and
  `pair.py launch --base --session --title [--writer-prompt --writer-agent] [--reopen]`.

- [ ] **Step 1: Parse `--pair`**

In the header comment and in both usage lines, change
`[--base <base-branch>]` to `[--base <base-branch>] [--pair]`.

After `BASE_BRANCH=""` add:

```bash
PAIR=0
```

In the `case "$1" in` block, before `-*)`, add:

```bash
    --pair)
      PAIR=1; shift ;;
```

- [ ] **Step 2: Initialise pair mode before the session is created**

Insert immediately before the line `session_id="${existing_id}"`:

```bash
# --- Pair mode: writer (left) + read-only reviewer (right); see sadensmol-task pair.md ---
#
# Only when this run creates the session. An existing session keeps its live panes,
# and a second init would move the opencode port under a running server.
PAIR_REOPEN=0
if [ -f "${WORKTREE_BASE}/.pair/config.json" ]; then
  PAIR=1
  PAIR_REOPEN=1
fi
if [ "${PAIR}" = 1 ] && [ -z "${existing_id}" ]; then
  PAIR_PY=""
  for d in "$HOME/.claude/skills/sadensmol-task" "$HOME/.agents/skills/sadensmol/task"; do
    if [ -f "$d/scripts/pair.py" ]; then PAIR_PY="$d/scripts/pair.py"; break; fi
  done
  if [ -z "${PAIR_PY}" ]; then
    echo "ERROR: --pair needs scripts/pair.py from the sadensmol-task skill."
    exit 1
  fi
  WRITER_KIND=claude
  if [ "${OPENCODE:-}" = "1" ]; then WRITER_KIND=opencode; fi
  PAIR_PORT="$(python3 "${PAIR_PY}" init --base "${WORKTREE_BASE}" --writer "${WRITER_KIND}")"
  if [ "${WRITER_KIND}" = opencode ]; then
    # opencode gets its first instruction through its server once launch binds the session
    AGENT_CMD="opencode --auto --port ${PAIR_PORT}"
    AGENT_PLAN_CMD="${AGENT_CMD}"
  else
    PAIR_BRIEF="$(python3 "${PAIR_PY}" brief --base "${WORKTREE_BASE}" --role writer)"
    AGENT_CMD="claude --dangerously-skip-permissions --append-system-prompt \"${AUTOPILOT} ${PAIR_BRIEF}\""
    AGENT_PLAN_CMD="${AGENT_CMD} \"task plan\""
  fi
fi
```

- [ ] **Step 3: Launch the pair after the session exists**

At the end of the agterm block, after the
`if [ -n "${session_id}" ]; then … session flag on … fi` block, add:

```bash
if [ "${PAIR}" = 1 ] && [ -z "${existing_id}" ] && [ -n "${session_id}" ]; then
  launch_args=(--base "${WORKTREE_BASE}" --session "${session_id}" --title "${AGTERM_NAME}")
  if [ "${PAIR_REOPEN}" = 1 ]; then launch_args+=(--reopen); fi
  if [ "${WRITER_KIND}" = opencode ] && [ "${START_PLAN}" = true ]; then
    launch_args+=(--writer-prompt "task plan" --writer-agent plan)
  fi
  python3 "${PAIR_PY}" launch "${launch_args[@]}" \
    || echo "WARNING: pair launch failed (see above); the writer runs without a reviewer."
elif [ "${PAIR}" = 1 ] && [ -n "${existing_id}" ]; then
  echo "Pair mode: the existing session keeps its panes; nothing was re-launched."
fi
```

- [ ] **Step 4: Syntax and dry checks**

Run: `bash -n <work_swipegames>/skills/task/scripts/new-task.sh && echo ok`
Expected: `ok`.

Run: `bash <work_swipegames>/skills/task/scripts/new-task.sh --pair 2>&1 | head -2`
Expected: the usage line, which now includes `[--pair]` (no branch was given, so the
script stops there).

- [ ] **Step 5: Document `--pair` in the swipegames task SKILL.md**

In `### \`new <branch-name>\` / \`<branch-name>\``, directly after the
`**Autopilot:** …` paragraph, add:

```markdown
**Pair mode (`--pair`):** `task new <branch> --pair` opens the session as a split. The
agent above is the **writer** (left pane), and the other agent kind runs as a read-only
**reviewer** (right pane): Claude Code writer → OpenCode reviewer, and `OPENCODE=1`
writer → Claude Code reviewer. The reviewer checks `PLAN.md` before the plan gate and
each plan step's diff. The protocol and the mechanics are in `sadensmol-task` →
*Pair mode*. A worktree with `.pair/config.json` re-opens in pair mode by itself.
```

Then in the `Run …` block, change the command to
`bash <skill-dir>/scripts/new-task.sh <branch-name> [--base <base-branch>] [--pair]`,
and add `"task new <branch> --pair"` to the `**Triggers:**` line of that section.

- [ ] **Step 6: Checkpoint**

Run: `git -C /Users/saden/work/sadensmol/work_swipegames diff --stat -- skills/task`
Expected: `new-task.sh` and `SKILL.md` changed. Do not commit.

---

### Task 7: memoresse `new-task.sh --pair` (Claude writer only)

**Files:**
- Modify: `<work_memoresse>/skills/task/scripts/new-task.sh`
- Modify: `<work_memoresse>/skills/task/SKILL.md` (`### \`new <branch-name>\``)

**Interfaces:**
- Consumes: the same `pair.py` CLI as Task 6.

- [ ] **Step 1: Parse `--pair`**

In the header comment (line 14) and the usage line, change `[--base <base-branch>]` to
`[--base <base-branch>] [--pair]`. After `BASE_BRANCH=""` add `PAIR=0`. In the
`case "$1" in` block, before `-*)`, add:

```bash
    --pair)
      PAIR=1; shift ;;
```

- [ ] **Step 2: Initialise pair mode before the session is created**

Insert immediately before the line `session_id="${existing_id}"`:

```bash
# --- Pair mode: writer (left) + read-only reviewer (right); see sadensmol-task pair.md ---
#
# Only when this run creates the session: an existing session keeps its live panes.
PAIR_REOPEN=0
if [ -f "${WORKTREE_BASE}/.pair/config.json" ]; then
  PAIR=1
  PAIR_REOPEN=1
fi
if [ "${PAIR}" = 1 ] && [ -z "${existing_id}" ]; then
  PAIR_PY=""
  for d in "$HOME/.claude/skills/sadensmol-task" "$HOME/.agents/skills/sadensmol/task"; do
    if [ -f "$d/scripts/pair.py" ]; then PAIR_PY="$d/scripts/pair.py"; break; fi
  done
  if [ -z "${PAIR_PY}" ]; then
    echo "ERROR: --pair needs scripts/pair.py from the sadensmol-task skill."
    exit 1
  fi
  python3 "${PAIR_PY}" init --base "${WORKTREE_BASE}" --writer claude >/dev/null
  PAIR_BRIEF="$(python3 "${PAIR_PY}" brief --base "${WORKTREE_BASE}" --role writer)"
  CLAUDE_CMD="claude --dangerously-skip-permissions --append-system-prompt \"${AUTOPILOT} ${PAIR_BRIEF}\""
fi
```

- [ ] **Step 3: Launch the pair after the session exists**

After the `if [ -n "${session_id}" ]; then … session flag on … fi` block, add:

```bash
if [ "${PAIR}" = 1 ] && [ -z "${existing_id}" ] && [ -n "${session_id}" ]; then
  launch_args=(--base "${WORKTREE_BASE}" --session "${session_id}" --title "${AGTERM_NAME}")
  if [ "${PAIR_REOPEN}" = 1 ]; then launch_args+=(--reopen); fi
  python3 "${PAIR_PY}" launch "${launch_args[@]}" \
    || echo "WARNING: pair launch failed (see above); the writer runs without a reviewer."
elif [ "${PAIR}" = 1 ] && [ -n "${existing_id}" ]; then
  echo "Pair mode: the existing session keeps its panes; nothing was re-launched."
fi
```

- [ ] **Step 4: Syntax check**

Run: `bash -n <work_memoresse>/skills/task/scripts/new-task.sh && echo ok`
Expected: `ok`.

- [ ] **Step 5: Document `--pair` in the memoresse task SKILL.md**

In `### \`new <branch-name>\` / \`<branch-name>\``, after the `**Terminal session:** …`
paragraph, add:

```markdown
**Pair mode (`--pair`):** `task new <branch> --pair` opens the session as a split. Claude
Code is the **writer** (left pane), and a read-only OpenCode **reviewer** runs in the
right pane. The reviewer checks `PLAN.md` before the plan gate and each plan step's
diff. The protocol and the mechanics are in `sadensmol-task` → *Pair mode*. A
worktree with `.pair/config.json` re-opens in pair mode by itself.
```

Add `[--pair]` to the documented `new-task.sh` command line in that section.

- [ ] **Step 6: Checkpoint**

Run: `git -C /Users/saden/work/sadensmol/work_memoresse diff --stat -- skills/task`
Expected: `new-task.sh` and `SKILL.md` changed. Do not commit.

---

### Task 8: End-to-end check with the user

**Files:** none changed. The user takes part, because every step starts real agents.

- [ ] **Step 1: Claude writer → OpenCode reviewer**

Ask the user to run `task new <throwaway-branch> --pair` in a swipegames session. Then
check each point with `agtermctl tree --json`, `pair.py status`, and the user's view of
the panes:
- the split opens, and `splitForeground` shows `opencode`;
- the OpenCode pane shows the session titled with the task id, and the reviewer brief
  has been answered;
- `python3 <pair.py> status --base <worktree-base>` shows both roles reachable after
  the writer starts its waiter;
- after the writer writes `PLAN.md`, a "plan ready" message reaches the reviewer and
  a reply comes back before the plannotator gate opens.

- [ ] **Step 2: OpenCode writer → Claude reviewer**

Ask the user to run `OPENCODE=1 task new <throwaway-branch-2> --pair`. Check that:
- the Claude reviewer starts with its waiter;
- the OpenCode writer received "task plan" through `prompt_async` with the `plan` agent.

- [ ] **Step 3: User request through the reviewer**

Ask the user to type a small change request into the reviewer pane. Check that:
- the reviewer discusses it;
- the reviewer sends a `Chat from reviewer (user request):` message;
- only the writer edits files (`git status` in the worktree changes only after the
  writer's turn).

- [ ] **Step 4: Re-open**

Ask the user to close the agterm session of the first task and run `task new
<throwaway-branch>` again, without `--pair`. Check that:
- pair mode comes back by itself;
- the OpenCode reviewer prompt contains "This task was re-opened".

- [ ] **Step 5: Clean up**

Ask the user to run `task cleanup` on both throwaway branches. They were never pushed,
so they can be deleted with `--force` after the user confirms. Report the results of
every step to the user.
