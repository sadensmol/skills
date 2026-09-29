#!/usr/bin/env python3
"""Mailbox between the writer and the reviewer of a task in pair mode.

The protocol both agents follow is in ../pair.md. Messages are files under
<worktree-base>/.pair/inbox/<role>/. A Claude Code agent receives them through a
background `wait`; an OpenCode agent is woken by prompt_async on its local server.
"""

from __future__ import annotations

import argparse
import fcntl
import hashlib
import json
import os
import re
import secrets
import shlex
import shutil
import signal
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
USER_SAID_LABEL = "The user said to the {role}:"
PAIR_REMINDER = (
    "PAIR MODE: {forwarded} The {peer} forms its own view and sends it to you. Do not "
    "answer the user and do not edit yet. Every user message is discussed by the pair "
    "first. Form your own view and send it to the {peer} before you read the view of the "
    "{peer}. A question goes as VIEW: user asked: <verbatim>. My answer: <your answer>. "
    "This includes a question that states a view, such as it should do X, or do you "
    "think otherwise, and a plain fact. A request or a correction goes as a VIEW of how "
    "you would do it. Then send the {peer} a MERGE: every difference between the two "
    "views and which side wins and why. Give the user the joint answer after the {peer} "
    "agrees, or both answers when you still disagree. Now tell the user in one line that "
    "you discuss it with the {peer}, send your VIEW, and end your turn: the waiter wakes "
    "you. Exception: a message that only runs a command, subcommand or skill, such as "
    "task finish, task plan or run the tests, is not discussed. The agent it was sent to "
    "runs it at once and sends the {peer} FYI: running the command. The {peer} takes no "
    "action on it.")
REVIEWER_AGENT = "pair-reviewer"
MAX_BODY_BYTES = 4000
POLL_SECONDS = 1.0
HTTP_TIMEOUT = 10
PAIR_PY = Path(__file__).resolve()
SKILL_DIR = PAIR_PY.parent.parent


class PairError(RuntimeError):
    """A refusal the caller must see; `code` is the exit status."""

    def __init__(self, message: str, code: int = 1):
        super().__init__(message)
        self.code = code


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
    return put_message(inbox(base, to), sender, text)


def put_message(box: Path, sender: str, text: str) -> Path:
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


HANDLING = "handling"


def waiter_state(base: Path, role: str) -> str:
    """listening (a live wait), handling (a batch was just delivered), or absent."""
    try:
        content = waiter_pid_file(base, role).read_text().strip()
    except FileNotFoundError:
        return "absent"
    if content.startswith(HANDLING):
        return "handling"
    try:
        return "listening" if pid_alive(int(content)) else "absent"
    except ValueError:
        return "absent"


def waiter_alive(base: Path, role: str) -> bool:
    return waiter_state(base, role) != "absent"


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


def focus_opencode(cfg: dict) -> None:
    """Show the pair session in the OpenCode pane.

    A /new in that pane, or a restart, leaves the TUI on another session while
    prompt_async keeps feeding the pair one, so the pane looks dead.
    """
    session = cfg["opencode"].get("session")
    if session:
        http_json("POST", opencode_url(cfg, "/tui/select-session"), {"sessionID": session})


def flush_opencode(base: Path, cfg: dict, role: str) -> None:
    messages = unread(base, role)
    if messages:
        try:
            focus_opencode(cfg)
        except PairError:
            pass  # the prompt below reports a server that is down
    # oldest first, so a backlog left by an earlier failed send arrives in order
    for path in messages:
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
    return deliver(base, cfg, to, path)


def deliver(base: Path, cfg: dict, to: str, path: Path) -> int:
    """Wake the recipient of a message already written to its inbox."""
    if cfg["roles"][to]["kind"] == "opencode":
        try:
            flush_opencode(base, cfg, to)
        except PairError:
            mark_blocked(cfg, to)
            raise
        print(json.dumps({"sent": path.name, "delivered": True}))
        return 0
    state = waiter_state(base, to)
    if state == "handling":
        # the peer is busy with the previous batch and re-arms its wait afterwards
        print(json.dumps({"sent": path.name, "delivered": False, "queued": True}))
        return 0
    if state == "absent":
        mark_blocked(cfg, to)
        print(f"WARNING: the {to} is not listening (no live `pair.py wait`). The message "
              f"is queued in {path} and arrives with its next wait.", file=sys.stderr)
        print(json.dumps({"sent": path.name, "delivered": False}))
        return 3
    print(json.dumps({"sent": path.name, "delivered": True}))
    return 0


def user_said(base: Path, role: str, prompt: str) -> str:
    """The message that tells the peer what the user typed into `role`'s pane."""
    body = f"{USER_SAID_LABEL.format(role=role)} {prompt}"
    if len(body.encode("utf-8")) <= MAX_BODY_BYTES:
        return body
    long_file = pair_dir(base) / "files" / f"user-to-{role}-{time.time_ns()}.md"
    long_file.parent.mkdir(parents=True, exist_ok=True)
    long_file.write_text(prompt + "\n", encoding="utf-8")
    return f"{USER_SAID_LABEL.format(role=role)} (long message, read it in {long_file})"


def cmd_user_prompt(args: argparse.Namespace) -> int:
    """UserPromptSubmit hook of a Claude Code agent in pair mode.

    Forwards what the user typed to the peer and reminds this agent, in its
    context, that the pair discusses the message before anyone answers it. It
    never blocks the prompt: on any failure it only says so in the context.
    """
    peer = other(args.role)
    try:
        prompt = str(json.load(sys.stdin).get("prompt", "")).strip()
    except (json.JSONDecodeError, AttributeError):
        prompt = ""
    # claude code also fires the hook for background-task notifications, which no user typed
    if not prompt or prompt.startswith("<task-notification>"):
        return 0
    errors = (PairError, OSError, KeyError, ValueError, subprocess.TimeoutExpired)
    try:
        base = resolve_base(args.base)
        cfg = load_config(base)
        path = write_message(base, peer, "user", user_said(base, args.role, prompt))
    except errors as exc:
        print(PAIR_REMINDER.format(
            forwarded=(f"pair.py could NOT forward this user message to the {peer} ({exc}). "
                       f"Send it to the {peer} yourself, in the user own words."),
            peer=peer))
        return 0
    try:
        with open(os.devnull, "w") as quiet:
            stdout, sys.stdout = sys.stdout, quiet
            try:
                code = deliver(base, cfg, peer, path)
            finally:
                sys.stdout = stdout
        forwarded = f"pair.py forwarded this user message verbatim to the {peer}."
        if code:
            forwarded = (f"pair.py queued this user message for the {peer}, which is not "
                         "listening now; it arrives with its next wait. Do not send it again.")
    except errors as exc:
        # the message stays in the inbox; the next flush or wait delivers it
        forwarded = (f"pair.py queued this user message for the {peer}, but could not "
                     f"wake it ({exc}). Do not send it again; tell the user in one line that "
                     f"the {peer} is not reachable.")
    print(PAIR_REMINDER.format(forwarded=forwarded, peer=peer))
    return 0


def claude_settings(base: Path, role: str) -> dict:
    args = f"--base {shlex.quote(str(base))} --role {role}"
    prompt = f"python3 {shlex.quote(str(PAIR_PY))} user-prompt {args}"
    start = f"python3 {shlex.quote(str(PAIR_PY))} session-start {args}"
    return {"hooks": {
        "UserPromptSubmit": [
            {"hooks": [{"type": "command", "command": prompt, "timeout": 20}]}],
        # /clear (typed as /clear, /new or /reset) keeps the process and its old waiter
        "SessionStart": [
            {"matcher": "clear", "hooks": [{"type": "command", "command": start, "timeout": 20}]}],
    }}


SESSION_START_NOTE = (
    "PAIR MODE resumed after /clear: you are still the {role_upper} of the pair, the "
    "{peer} runs in the {pane} pane. pair.py stopped your old waiter{focused}. Re-read "
    "{pair_md}, then PLAN.md and the latest files in {base}/.pair/inbox/*/read/ to catch "
    "up. Start python3 {py} wait --base {base} --role {role} in the background now.")


def cmd_session_start(args: argparse.Namespace) -> int:
    """SessionStart hook (matcher clear) of a Claude Code agent in pair mode."""
    errors = (PairError, OSError, KeyError, ValueError, subprocess.TimeoutExpired)
    try:
        base = resolve_base(args.base)
        cfg = load_config(base)
    except errors:
        return 0
    role, peer = args.role, other(args.role)
    retire_waiter(base, role)
    focused = ""
    if cfg["roles"][peer]["kind"] == "opencode":
        try:
            focus_opencode(cfg)
            focused = f" and put the {peer} pane back on the pair session"
        except errors:
            pass
    print(SESSION_START_NOTE.format(role_upper=role.upper(), role=role, peer=peer,
                                    pane=PANES[peer], focused=focused, base=base,
                                    pair_md=SKILL_DIR / "pair.md", py=PAIR_PY))
    return 0


def claude_settings_file(base: Path, role: str) -> Path:
    return pair_dir(base) / f"claude-{role}-settings.json"


SUPERSEDED = ("SUPERSEDED: a newer pair.py wait of the {role} took over this one. "
              "Do NOT start another wait for it.")


def retire_waiter(base: Path, role: str) -> None:
    """Stop the recorded waiter of `role`, e.g. one a /clear left running.

    Two waiters race for one inbox, and the old one hands the batch to a
    conversation that no longer exists.
    """
    try:
        pid = int(waiter_pid_file(base, role).read_text().strip())
    except (FileNotFoundError, ValueError):
        return
    if pid == os.getpid() or not pid_alive(pid):
        return
    command = subprocess.run(["ps", "-o", "command=", "-p", str(pid)], capture_output=True,
                             text=True, check=False).stdout
    # a recycled pid is someone else's process
    if "pair.py" in command and " wait " in command and f"--role {role}" in command:
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass


def cmd_wait(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    role = args.role
    retire_waiter(base, role)

    def superseded(_signum, _frame):
        print(SUPERSEDED.format(role=role))
        sys.stdout.flush()
        # skip the finally below: the pid file belongs to the newer waiter now
        os._exit(0)

    signal.signal(signal.SIGTERM, superseded)
    pid_file = waiter_pid_file(base, role)
    pid_file.write_text(f"{os.getpid()}\n")
    deadline = time.monotonic() + args.timeout if args.timeout else None
    delivered = False
    try:
        while True:
            messages = unread(base, role)
            if messages:
                print(f"PAIR MESSAGES for the {role} ({len(messages)}):")
                for path in messages:
                    print(path.read_text(encoding="utf-8").rstrip("\n"))
                    mark_read(base, role, path)
                print(f"Start the wait again in the background first, then handle them: "
                      f"python3 {PAIR_PY} wait --base {base} --role {role}")
                sys.stdout.flush()
                delivered = True
                return 0
            if deadline is not None and time.monotonic() >= deadline:
                return 4
            time.sleep(POLL_SECONDS)
    finally:
        try:
            if pid_file.read_text().strip() == str(os.getpid()):
                if delivered:
                    pid_file.write_text(f"{HANDLING} {int(time.time())}\n")
                else:
                    pid_file.unlink()
        except FileNotFoundError:
            pass


def agent_kind(argv: list | None) -> str:
    for kind in ("opencode", "claude"):
        if runs(argv, kind):
            return kind
    return "none"


def is_paired(base_arg: str | None, node: dict) -> bool:
    try:
        base = resolve_base(base_arg)
    except PairError:
        return False
    if base_arg and base.resolve() != Path(base_arg).resolve():
        return False
    reviewer = load_config(base)["roles"]["reviewer"]["kind"]
    return bool(node.get("hasSplit")) and runs(node.get("splitForeground"), reviewer)


def cmd_inspect(args: argparse.Namespace) -> int:
    node = find_session(args.session)
    if args.get == "writer":
        print(agent_kind(node.get("foreground")))
    elif args.get == "status":
        print(node.get("status") or "idle")
    else:
        print("1" if is_paired(args.base, node) else "0")
    return 0


UUID_RE = re.compile(r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}")


def cmd_respawn(args: argparse.Namespace) -> int:
    """Replace a session with a new one right after it, then close the old one."""
    out = ctl("session", "new", "--cwd", args.cwd, "--name", args.name,
              "--after", args.old, "--command", args.command)
    match = UUID_RE.search(out)
    if not match:
        raise PairError(f"agtermctl session new printed no session id: {out.strip()}")
    if os.environ.get("AGTERM_SESSION_ID", "").lower() == args.old.lower():
        # this process runs inside the old session: close it once we have returned
        command = os.environ.get("AGTERMCTL", "agtermctl")
        subprocess.Popen(["/bin/sh", "-c", f"sleep 2; {shlex.quote(command)} session close "
                          f"--target {shlex.quote(args.old)}"],
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                         start_new_session=True)
    else:
        ctl("session", "close", "--target", args.old)
    print(match.group(0))
    return 0


def cmd_flush(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    cfg = load_config(base)
    if cfg["roles"][args.to]["kind"] != "opencode":
        raise PairError(f"the {args.to} is a Claude Code agent; its next wait delivers the backlog")
    flush_opencode(base, cfg, args.to)
    print(json.dumps({"flushed": True}))
    return 0


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
                # "all": a thread's child session runs it as a subagent too
                "mode": "all",
                "permission": {"edit": "deny", "bash": "allow"},
            }
        },
        # keeps the pane on the pair session after a /new there
        "plugin": [(PAIR_PY.parent / "pair-opencode-plugin.js").as_uri()],
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
    # launch looks for the opencode session created after this moment
    cfg["started_ms"] = int(time.time() * 1000)
    # a fresh port and no session: the previous opencode server is gone with its pane
    cfg["opencode"] = {"port": free_port(), "session": None}
    (root / "opencode-reviewer.json").write_text(
        json.dumps(reviewer_opencode_config(), indent=2) + "\n")
    for role in ROLES:
        if cfg["roles"][role]["kind"] == "claude":
            # the agent starts with --settings <this file>: its hook forwards user messages
            claude_settings_file(base, role).write_text(
                json.dumps(claude_settings(base, role), indent=2) + "\n")
    save_config(base, cfg)
    for role in ROLES:
        waiter_pid_file(base, role).unlink(missing_ok=True)
    exclude_from_git(base)
    print(cfg["opencode"]["port"])
    return 0


SHELL_SPECIAL = set("'\"$`\\")
CATCH_UP = ("This task was re-opened: read PLAN.md and the files in .pair/inbox/*/read/ "
            "to catch up first.")


def brief(base: Path, cfg: dict, role: str, then: str | None = None,
          reopen: bool = False) -> str:
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
                     "(run_in_background) now, and start it again as soon as a batch "
                     "arrives, before you handle it.")
    else:
        parts.append(f"Messages from the {peer} arrive by themselves as prompts that "
                     "start with Chat from.")
    parts.append("This is pair programming: every decision is made by both agents "
                 "BEFORE the code is typed, never in a review afterwards. Each agent forms "
                 "its own view before it sees the view of the other, then the two views "
                 "are merged: one agent never decides alone while the other only signs. "
                 "A user message that only runs a command, subcommand or skill, such as "
                 "task finish, task plan or run the tests, is not a decision: run it at "
                 "once and send the peer FYI: running the command.")
    parts.append(f"Every question of the user is discussed by both agents before the user "
                 f"gets an answer: send the {peer} VIEW: user asked: the question. My "
                 "answer: your answer. Answer the user only after the two of you merge the "
                 "views, and give both answers when you still disagree.")
    if role == "writer":
        parts.append("You are the DRIVER: for each decision, write your own view to a "
                     "file, send the reviewer a DECISION with the problem only, never your "
                     "view, and wait for its VIEW. Then send your VIEW and a MERGE that takes "
                     "the best of both, and edit only after the reviewer agrees. Forward "
                     "every user request to the reviewer as a DECISION.")
        if cfg["roles"]["reviewer"]["kind"] == "opencode":
            parts.append(f"Discuss each DECISION in its own thread, in a background fork: "
                         f"python3 {py} thread open (see Threads in pair.md), so the "
                         "reviewer handles several decisions in parallel.")
    else:
        parts.append("You are the NAVIGATOR and you are READ-ONLY: never edit, create or "
                     "delete files in the worktree. You co-own every decision: answer each "
                     "DECISION first with your own VIEW, formed from the code and the "
                     "project conventions before you read the view of the writer. Answer "
                     "a MERGE with AGREE only after you name the strongest objection you "
                     "checked in the code and why it does not hold, or with COUNTER. Start "
                     "reading the ticket and the code now.")
        if kind == "opencode":
            parts.append("Threads in .pair/threads/ belong to their own reviewer subagents: "
                         "never read their files and never run thread send.")
    if reopen:
        parts.append(CATCH_UP)
    if then:
        parts.append(f"Then do this: {then}")
    text = " ".join(parts)
    # the launchers nest the brief in "..." inside '...' on a shell line
    bad = sorted(SHELL_SPECIAL.intersection(text))
    if bad:
        raise PairError(f"the brief contains shell-special characters {bad} (from a path "
                        "or --then); the launcher cannot quote them")
    return text


def cmd_brief(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    print(brief(base, load_config(base), args.role, then=args.then, reopen=args.reopen))
    return 0


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


def reviewer_command(base: Path, cfg: dict, reopen: bool) -> str:
    root = pair_dir(base)
    # the brief goes through a file: a typed shell line must stay short
    brief_file = root / "reviewer-brief.txt"
    brief_file.write_text(brief(base, cfg, "reviewer", reopen=reopen), encoding="utf-8")
    brief_arg = f"\"$(cat {shlex.quote(str(brief_file))})\""
    if cfg["roles"]["reviewer"]["kind"] == "claude":
        settings = shlex.quote(str(claude_settings_file(base, "reviewer")))
        return ("claude --dangerously-skip-permissions --disallowedTools Edit Write "
                f"NotebookEdit --settings {settings} --append-system-prompt {brief_arg} "
                "\"Pair mode start: follow the pair brief in your system prompt now.\"")
    # --prompt starts opencode straight in a session, never on its start screen
    return (f"OPENCODE_CONFIG={shlex.quote(str(root / 'opencode-reviewer.json'))} "
            f"opencode --auto --agent {REVIEWER_AGENT} --port {cfg['opencode']['port']} "
            f"--prompt {brief_arg}")


def new_opencode_session(cfg: dict, base: Path) -> str | None:
    """The session opencode runs for this base: newest one active since init.

    `updated`, not `created`: an opencode started with --continue resumes an older
    session, and its --prompt is what touches it now.
    """
    try:
        sessions = http_json("GET", opencode_url(cfg, "/session")) or []
    except PairError:
        return None
    mine = [x for x in sessions
            if Path(x.get("directory", "")).resolve() == base.resolve()
            and x.get("time", {}).get("updated", 0) >= cfg.get("started_ms", 0)]
    if not mine:
        return None
    return max(mine, key=lambda x: x["time"]["updated"])["id"]


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
    ctl("session", "type", *pane, input_text=reviewer_command(base, cfg, args.reopen))
    # Return as its own event: a TUI can read text+Enter in one burst as a paste
    ctl("session", "type", *pane, input_text="\n")
    kind = cfg["roles"]["reviewer"]["kind"]
    wait_until(lambda: runs(find_session(sid).get("splitForeground"), kind),
               f"the {kind} reviewer", args.wait_seconds)

    cfg["opencode"]["session"] = wait_until(lambda: new_opencode_session(cfg, base),
                                            "the opencode session", args.wait_seconds * 2)
    save_config(base, cfg)

    for role in ROLES:
        if cfg["roles"][role]["kind"] == "opencode":
            # messages queued while no opencode session existed (a failed send, a re-open)
            flush_opencode(base, cfg, role)
    print(json.dumps({"session": sid, "opencode_session": cfg["opencode"]["session"]}))
    return 0


THREAD_LABELS = {"writer": "Chat from writer [{id}]:", "reviewer": "Chat from reviewer [{id}]:"}
THREAD_STATUSES = ("open", "agreed", "disputed", "closed")
THREAD_BRIEF = (
    "PAIR THREAD {id}: you are the REVIEWER of this one decision only, in a subagent "
    "session of the pair reviewer. Topic: {topic}. Files: {files}. Read {pair_md} and "
    "follow its reviewer rules: you are READ-ONLY, and you check the proposal against the "
    "code and the project conventions before you answer. Answer ONLY with: python3 {py} "
    "thread send --base {base} --id {id} --key {key} --to writer --stdin, with a body that "
    "starts with "
    "VIEW:, AGREE:, COUNTER:, QUESTION: or ANSWER:. Form your own VIEW before you read the "
    "view of the writer. Keep the key to yourself. Later messages of this thread arrive here and "
    "start with Chat from writer [{id}]:.")


def threads_dir(base: Path) -> Path:
    return pair_dir(base) / "threads"


def thread_dir(base: Path, tid: str) -> Path:
    path = threads_dir(base) / tid
    if not re.fullmatch(r"t\d+", tid) or not (path / "meta.json").is_file():
        raise PairError(f"no thread {tid}")
    return path


def load_meta(path: Path) -> dict:
    return json.loads((path / "meta.json").read_text())


def save_meta(path: Path, meta: dict) -> None:
    tmp = path / "meta.json.tmp"
    tmp.write_text(json.dumps(meta, indent=2) + "\n")
    tmp.replace(path / "meta.json")


def all_threads(base: Path) -> list[tuple[Path, dict]]:
    root = threads_dir(base)
    if not root.is_dir():
        return []
    found = [(p, load_meta(p)) for p in root.glob("t*") if (p / "meta.json").is_file()]
    return sorted(found, key=lambda item: int(item[0].name[1:]))


def norm_files(raw: str) -> list[str]:
    files = []
    for item in raw.split(","):
        item = item.strip()
        while item.startswith("./"):
            item = item[2:]
        if item:
            files.append(item)
    if not files:
        raise PairError("--files names no file")
    return files


def overlaps(a: str, b: str) -> bool:
    """Same file, or one is a directory (ends with /) that holds the other."""
    return a == b or (a.endswith("/") and b.startswith(a)) or (b.endswith("/") and a.startswith(b))


def threads_lock(base: Path):
    """Two forks may open or resume threads at the same moment."""
    root = threads_dir(base)
    root.mkdir(parents=True, exist_ok=True)
    lock = open(root / ".lock", "w")
    fcntl.flock(lock, fcntl.LOCK_EX)
    return lock


def refuse_overlap(base: Path, files: list[str], skip: str | None = None) -> None:
    for path, meta in all_threads(base):
        if meta["status"] == "closed" or path.name == skip:
            continue
        for mine in files:
            for theirs in meta["files"]:
                if overlaps(mine, theirs):
                    raise PairError(f"thread {path.name} ({meta['status']}) already covers "
                                    f"{theirs}; go on after it is closed", code=5)


def new_thread_dir(base: Path, files: list[str]) -> Path:
    """Take the next free t<n>, refusing files that an unclosed thread covers."""
    root = threads_dir(base)
    with threads_lock(base):
        refuse_overlap(base, files)
        n = 1
        while (root / f"t{n}").exists():
            n += 1
        path = root / f"t{n}"
        path.mkdir()
        return path


def key_hash(key: str) -> str:
    return hashlib.sha256(key.encode()).hexdigest()


def flush_thread(base: Path, cfg: dict, path: Path, meta: dict) -> None:
    """Deliver the thread's queued writer messages to its reviewer child, oldest first.

    The first delivery carries the thread brief with a fresh reply key. Neither is
    written to disk: the main reviewer reads .pair/ and must not answer the thread.
    """
    box = path / "inbox" / "reviewer"
    url = opencode_url(cfg, f"/session/{meta['reviewer_session']}/prompt_async")
    agent = cfg["roles"]["reviewer"].get("agent", REVIEWER_AGENT)
    for msg in sorted(box.glob("*.md")):
        text = msg.read_text(encoding="utf-8").rstrip("\n")
        key = None
        if not meta.get("briefed"):
            key = secrets.token_hex(8)
            text = THREAD_BRIEF.format(id=meta["id"], topic=meta["topic"], key=key,
                                       files=", ".join(meta["files"]), base=base,
                                       pair_md=SKILL_DIR / "pair.md", py=PAIR_PY) + "\n\n" + text
        http_json("POST", url, {"parts": [{"type": "text", "text": text}], "agent": agent})
        if key:
            meta.update(briefed=True, key_sha256=key_hash(key))
            save_meta(path, meta)
        (box / "read").mkdir(exist_ok=True)
        msg.replace(box / "read" / msg.name)


def cmd_thread_open(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    cfg = load_config(base)
    if cfg["roles"]["reviewer"]["kind"] != "opencode":
        raise PairError("threads need an OpenCode reviewer; use send and the decision loop",
                        code=2)
    parent = cfg["opencode"].get("session")
    if not parent:
        raise PairError("the opencode session is not bound yet (pair.py launch binds it)")
    files = norm_files(args.files)
    body = read_body(args)
    path = new_thread_dir(base, files)
    tid = path.name
    try:
        child = http_json("POST", opencode_url(cfg, "/session"), {
            "parentID": parent, "agent": cfg["roles"]["reviewer"].get("agent", REVIEWER_AGENT),
            "title": f"pair {tid}: {args.topic}"})
        session = child["id"]
    except (PairError, KeyError, TypeError) as exc:
        shutil.rmtree(path, ignore_errors=True)
        raise PairError(f"could not create the reviewer session of the thread: {exc}") from exc
    meta = {"id": tid, "topic": args.topic, "files": files, "status": "open",
            "reviewer_session": session, "pair_session": parent, "briefed": False,
            "created_ms": int(time.time() * 1000)}
    save_meta(path, meta)
    label = THREAD_LABELS["writer"].format(id=tid)
    put_message(path / "inbox" / "reviewer", "writer", f"{label} {body}")
    try:
        flush_thread(base, cfg, path, meta)
    except PairError as exc:
        raise PairError(f"thread {tid} is open, but its proposal did not reach the reviewer "
                        f"({exc}); the next thread send delivers it") from exc
    print(json.dumps({"id": tid, "reviewer_session": session}))
    return 0


def cmd_thread_send(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    cfg = load_config(base)
    path = thread_dir(base, args.id)
    meta = load_meta(path)
    if meta["status"] != "open" and args.to == "reviewer":
        raise PairError(f"thread {args.id} is {meta['status']}: to discuss it again with its "
                        f"reviewer subagent, run python3 {PAIR_PY} thread resume --base {base} "
                        f"--id {args.id} --stdin")
    if meta["status"] != "open":
        raise PairError(f"thread {args.id} is {meta['status']}: the writer has settled it. "
                        "Do not send it again, here or anywhere else.")
    if args.to == "writer":
        if not args.key:
            raise PairError("--key is required: only the reviewer subagent of the thread, "
                            "which got the key in its first prompt, replies to the writer")
        if key_hash(args.key) != meta.get("key_sha256"):
            raise PairError(f"wrong --key for thread {args.id}")
    body = read_body(args)
    sender = other(args.to)
    label = THREAD_LABELS[sender].format(id=args.id)
    msg = put_message(path / "inbox" / args.to, sender, f"{label} {body}")
    if args.to == "reviewer":
        flush_thread(base, cfg, path, meta)
    print(json.dumps({"sent": msg.name, "thread": args.id}))
    return 0


def cmd_thread_resume(args: argparse.Namespace) -> int:
    """Discuss a topic again with the reviewer subagent that already knows it."""
    base = resolve_base(args.base)
    cfg = load_config(base)
    path = thread_dir(base, args.id)
    body = read_body(args)
    with threads_lock(base):
        meta = load_meta(path)
        if meta.get("pair_session") != cfg["opencode"].get("session"):
            raise PairError(f"the reviewer of thread {args.id} is gone with its pair session "
                            "(the reviewer was relaunched): run thread open", code=6)
        refuse_overlap(base, meta["files"], skip=args.id)
        meta["status"] = "open"
        meta.pop("closed_ms", None)
        save_meta(path, meta)
    label = THREAD_LABELS["writer"].format(id=args.id)
    put_message(path / "inbox" / "reviewer", "writer", f"{label} {body}")
    flush_thread(base, cfg, path, meta)
    print(json.dumps({"id": args.id, "reviewer_session": meta["reviewer_session"],
                      "status": "open"}))
    return 0


def cmd_thread_wait(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    box = thread_dir(base, args.id) / "inbox" / "writer"
    deadline = time.monotonic() + args.timeout if args.timeout else None
    while True:
        messages = sorted(box.glob("*.md"))
        if messages:
            print(f"THREAD {args.id} MESSAGES ({len(messages)}):")
            (box / "read").mkdir(exist_ok=True)
            for msg in messages:
                print(msg.read_text(encoding="utf-8").rstrip("\n"))
                msg.replace(box / "read" / msg.name)
            return 0
        if deadline is not None and time.monotonic() >= deadline:
            return 4
        time.sleep(POLL_SECONDS)


def cmd_thread_close(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    path = thread_dir(base, args.id)
    meta = load_meta(path)
    if args.status in ("agreed", "disputed"):
        if not args.stdin:
            raise PairError(f"--status {args.status} needs the decision on --stdin")
        (path / "decision.md").write_text(read_body(args) + "\n", encoding="utf-8")
    meta["status"] = args.status
    if args.status == "closed":
        meta["closed_ms"] = int(time.time() * 1000)
    save_meta(path, meta)
    print(json.dumps({"id": args.id, "status": args.status}))
    return 0


def age(ms: int) -> str:
    seconds = max(0, int(time.time() - ms / 1000))
    if seconds < 60:
        return f"{seconds}s"
    if seconds < 3600:
        return f"{seconds // 60}m"
    return f"{seconds // 3600}h"


def cmd_threads(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    rows = all_threads(base)
    for _path, meta in rows:
        print(f"{meta['id']:<4} {meta['status']:<8} {age(meta['created_ms']):>4}  "
              f"{meta['topic']}  [{', '.join(meta['files'])}]")
    if not rows:
        print("no threads")
    return 0


THREAD_COMMANDS = {"open": cmd_thread_open, "send": cmd_thread_send, "resume": cmd_thread_resume,
                   "wait": cmd_thread_wait, "close": cmd_thread_close}


def cmd_thread(args: argparse.Namespace) -> int:
    return THREAD_COMMANDS[args.thread_cmd](args)


COMMANDS = {"send": cmd_send, "wait": cmd_wait, "status": cmd_status,
            "init": cmd_init, "brief": cmd_brief, "launch": cmd_launch,
            "flush": cmd_flush, "inspect": cmd_inspect, "respawn": cmd_respawn,
            "user-prompt": cmd_user_prompt, "session-start": cmd_session_start,
            "thread": cmd_thread, "threads": cmd_threads}


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

    flush = sub.add_parser("flush", help="deliver the unread backlog of an opencode role")
    flush.add_argument("--base")
    flush.add_argument("--to", choices=ROLES, required=True)

    inspect = sub.add_parser("inspect", help="one fact about an agterm task session")
    inspect.add_argument("--base")
    inspect.add_argument("--session", required=True)
    inspect.add_argument("--get", choices=("writer", "status", "paired"), required=True)

    respawn = sub.add_parser("respawn", help="replace a session with a new one after it")
    respawn.add_argument("--old", required=True)
    respawn.add_argument("--name", required=True)
    respawn.add_argument("--cwd", required=True)
    respawn.add_argument("--command", required=True)

    user_prompt = sub.add_parser("user-prompt",
                                 help="UserPromptSubmit hook: forward the user message to the peer")
    user_prompt.add_argument("--base")
    user_prompt.add_argument("--role", choices=ROLES, required=True)

    session_start = sub.add_parser("session-start",
                                   help="SessionStart hook after /clear: re-arm the pair")
    session_start.add_argument("--base")
    session_start.add_argument("--role", choices=ROLES, required=True)

    status = sub.add_parser("status", help="roles, reachability and unread counts")
    status.add_argument("--base")

    init = sub.add_parser("init", help="create or refresh .pair/ for a task")
    init.add_argument("--base", required=True)
    init.add_argument("--writer", choices=KINDS, required=True)

    brief_cmd = sub.add_parser("brief", help="print the start brief for a role")
    brief_cmd.add_argument("--base")
    brief_cmd.add_argument("--role", choices=ROLES, required=True)
    brief_cmd.add_argument("--then", help="an instruction appended after the brief")
    brief_cmd.add_argument("--reopen", action="store_true", help="add the catch-up sentence")

    launch = sub.add_parser("launch", help="open the split and start the reviewer")
    launch.add_argument("--base")
    launch.add_argument("--session", required=True, help="agterm session id")
    launch.add_argument("--reopen", action="store_true",
                        help="add the catch-up sentence to the reviewer brief")
    launch.add_argument("--wait-seconds", type=float, default=30)

    thread = sub.add_parser("thread", help="one decision discussed with a reviewer subagent")
    thread_sub = thread.add_subparsers(dest="thread_cmd", required=True)
    t_open = thread_sub.add_parser("open", help="start a thread: reviewer child + first proposal")
    t_open.add_argument("--topic", required=True)
    t_open.add_argument("--files", required=True,
                        help="comma-separated paths from the worktree base; a dir ends with /")
    t_send = thread_sub.add_parser("send", help="send a message inside a thread")
    t_send.add_argument("--to", choices=ROLES, required=True)
    t_send.add_argument("--key", help="--to writer: the key from the thread brief")
    t_resume = thread_sub.add_parser("resume", help="discuss a thread again with its subagent")
    t_wait = thread_sub.add_parser("wait", help="block until the thread's reviewer replies")
    t_wait.add_argument("--timeout", type=float, default=0, help="seconds; 0 waits forever")
    t_close = thread_sub.add_parser("close", help="set the thread status")
    t_close.add_argument("--status", choices=THREAD_STATUSES[1:], required=True)
    for p in (t_open, t_send, t_resume, t_wait, t_close):
        p.add_argument("--base")
    for p in (t_send, t_resume, t_wait, t_close):
        p.add_argument("--id", required=True)
    for p in (t_open, t_send, t_resume):
        p.add_argument("--stdin", action="store_true", required=True)
    t_close.add_argument("--stdin", action="store_true", help="the decision, for agreed|disputed")

    threads = sub.add_parser("threads", help="list the threads and their status")
    threads.add_argument("--base")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        return COMMANDS[args.cmd](args)
    except PairError as exc:
        print(f"pair.py: {exc}", file=sys.stderr)
        return exc.code


if __name__ == "__main__":
    sys.exit(main())
