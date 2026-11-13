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
import uuid
from pathlib import Path

ROLES = ("writer", "reviewer")
KINDS = ("claude", "opencode")
PANES = {"writer": "left", "reviewer": "right"}
LABELS = {"writer": "Chat from writer:", "reviewer": "Chat from reviewer:"}
USER_REQUEST_LABEL = "Chat from reviewer (user request):"
USER_SAID_LABEL = ("The user said to the {role} (context only, never send an ack: a STOP: "
                   "only when its answer or change is wrong):")
USER_SAID_THREAD_LABEL = ("The user said to the {role} (the {role} settles it in a pair "
                          "thread, do not answer it here and never send an ack):")
COMMAND_RULE = ("A message that only runs a command or skill (task finish) runs at once, "
                "with FYI: running <command> to the {peer}.")
PAIR_REMINDER = (
    "PAIR MODE: {forwarded} Clear (simple, settled by plan/skill/convention, done "
    "before, 100% sure): do it now, alone. Hard: do not edit yet; write your view file, "
    "settle it via a liaison (pair.md, Liaison). A hard question: answer the user now "
    "({peer} check pending), then only {peer} agreed. or Changed: <answer>. A hard "
    "request: say the pair discusses it. " + COMMAND_RULE)
PAIR_REMINDER_DIRECT = (
    "PAIR MODE: {forwarded} Clear (simple, settled by plan/skill/convention, done "
    "before, 100% sure): do it now, alone. Hard: do not edit yet; send the {peer} "
    "DECISION: <problem only> (a question: DECISION: user asked: <verbatim>), then VIEW, "
    "MERGE per pair.md. A hard question: answer the user now ({peer} check pending), then "
    "only {peer} agreed. or Changed: <answer>. " + COMMAND_RULE)
# the OpenCode agent names are what its prompt bar shows: Planning, Main, Review
REVIEWER_AGENT = "review"
# the reviewer agents before the rename: a reviewer started with one still runs on it
LEGACY_REVIEWER_AGENTS = ("pair-reviewer", "pair-reviewer-plan")
# the models of each task stage (SKILL.md, Stages). A running session cannot switch its
# model, so `stage` restarts the writer and the reviewer with these.
STAGES = ("plan", "impl")
MODELS = {
    "plan": {"claude": {"model": "claude-opus-5-5", "effort": "xhigh"},
             "opencode": {"model": "openai/gpt-6-astra", "variant": "high"},
             "writer_agent": "planning"},
    "impl": {"claude": {"model": "claude-opus-5-5", "effort": "high"},
             "opencode": {"model": "openai/gpt-6.1-sol", "variant": "high"},
             "writer_agent": "main"},
}
# the stage record of a single-agent session, at the worktree base
STAGE_FILE = ".task-stage"
# the decisions log of a single session (SKILL.md, Model decisions log)
SINGLE_DECISIONS_FILE = ".task-decisions.md"
# the user decisions made after the plan approval, in both modes: PLAN.md is frozen then
# (SKILL.md, User decisions log)
USER_DECISIONS_FILE = ".task-user-decisions.md"
USER_DECISIONS_READ = (f"{USER_DECISIONS_FILE} when it exists (a later user decision wins "
                       "over PLAN.md)")
# the Q&A log after the plan approval, in both modes; on PLAN it is a section of PLAN.md
# (SKILL.md, Q&A log)
QA_FILE = ".task-qa.md"
# the PLAN.md sections that the logs fill on PLAN, by entry kind
PLAN_LOG_SECTIONS = {"qa": "## Q&A", "decision": "## Decisions"}
PLAN_LOG_IDS = {"qa": re.compile(r"^### Q(\d+) ", re.M),
                "decision": re.compile(r"^### D(\d+) ", re.M)}
# PLAN-stage log entries held while PLAN.md is missing or the plan gate shows it: the gate
# reads the file once, so an edit makes the open page stale (references/plannotator.md)
PLAN_LOG_PENDING = ".scratch/plan-log-pending.json"
# the section of a single session's decisions log that reviewd appends (references/reviewd.md)
REVIEWS_HEADING = "## Reviews"
# `fresh` (references/fresh.md): the first prompt of the restarted writer per purpose,
# passed to the launcher as TASK_CONTINUE. {decisions} is the decisions log of the mode,
# {stage} the recorded stage. No shell-special characters: it is nested in "..." inside
# '...' on the start line. Every one starts with `<label>: your context was cleared on
# purpose`: the router hooks skip their routing pass on it (FRESH_PROMPT_RE), so the
# new conversation loads only what its action needs.
FRESH_MARK = "your context was cleared on purpose"
FRESH_PROMPT_RE = re.compile(r"[A-Za-z ]+: " + FRESH_MARK)
TASK_SKILL_ONLY = ("Load only the task skill (the sadensmol-task dispatcher and the project "
                   "task skill)")
FRESH_PROMPTS = {
    "plan": (f"Plan discussion: {FRESH_MARK} and PLAN.md at the worktree base is the whole "
             f"task context now. {TASK_SKILL_ONLY}, read PLAN.md, then open the plan gate "
             "directly (references/plan.md, step 6) and discuss the plan with the user."),
    "impl": (f"Implementation: {FRESH_MARK} after the user approved PLAN.md. "
             f"{TASK_SKILL_ONLY}, read PLAN.md, {{decisions}}, {USER_DECISIONS_READ} and the "
             "git diff of each repo at the worktree base, then implement the approved plan "
             "now (references/plan.md, step 7). Load any other skill only when a step of the "
             "work needs it."),
    "finish": (f"Finish: {FRESH_MARK} for task finish. "
               f"{TASK_SKILL_ONLY}, read PLAN.md, {{decisions}}, {USER_DECISIONS_READ} and the "
               "git diff of each repo at the worktree base, then run the task finish of the "
               "project from its start, without its Start in a clean context step "
               "(references/finish.md). Its review gate still applies. Load any other skill "
               "only when a step of finish needs it."),
    "reviews": (f"Plan review: {FRESH_MARK} for task reviews, and the task is on PLAN again "
                "to rework the approved plan. "
                f"{TASK_SKILL_ONLY}, read PLAN.md, {{decisions}}, {USER_DECISIONS_READ} and the "
                "git diff of each repo at the worktree base, then walk the plan steps with "
                "the user (references/reviews.md, step 3). Write no code until the user "
                "approves the reworked PLAN.md. Load any other skill only when a step of the "
                "review needs it."),
    "continue": (f"Fresh start: {FRESH_MARK} by task fresh, and the task stays in its "
                 "{stage} stage. Load no skill and read nothing about the task now: wait "
                 "for the user."),
}
# the stage of each `fresh --then`; without --then the recorded stage stays
FRESH_STAGES = {"plan": "plan", "impl": "impl", "finish": "impl", "reviews": "plan"}
# `stage` returns before the handoff restarts the session it runs in
STAGE_SETTLE_SECONDS = float(os.environ.get("PAIR_STAGE_SETTLE_SECONDS", "3"))
# typed reviewer start lines retried when the split shell lost keystrokes
LAUNCH_ATTEMPTS = 3
MAX_BODY_BYTES = 4000
POLL_SECONDS = 1.0
HTTP_TIMEOUT = 10
# prompt_async answers 204 before it runs: an accepted prompt is stored as a user message
# within ~200 ms, a dropped one (unknown agent or model) never is
CONFIRM_SECONDS = float(os.environ.get("PAIR_CONFIRM_SECONDS", "5"))
CONFIRM_POLL_SECONDS = 0.2
# the user message is among the newest ones even while the session streams its answer
CONFIRM_MESSAGES = 20
# the decision loop runs in liaison threads when the reviewer is an OpenCode agent
THREAD_ONLY_KINDS = ("DECISION:", "VIEW:", "MERGE:")
# an FYI needs no reply: it waits in .pair/held/<role>/ and goes out with the next other
# message to that role, or when this many pile up; each delivery costs the peer a turn
FYI_BATCH = int(os.environ.get("PAIR_FYI_BATCH", "5"))
# edit tools the writer's PreToolUse hook checks against the open pair threads
EDIT_TOOLS = "Edit|Write|MultiEdit|NotebookEdit"
THREADS_LISTED = 12
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


def stored_prompt(cfg: dict, session: str, text: str) -> bool:
    messages = http_json("GET", opencode_url(
        cfg, f"/session/{session}/message?limit={CONFIRM_MESSAGES}")) or []
    for message in messages if isinstance(messages, list) else []:
        if message.get("info", {}).get("role") != "user":
            continue
        for part in message.get("parts", []):
            if part.get("type") == "text" and str(part.get("text", "")).strip() == text.strip():
                return True
    return False


def prompt_session(cfg: dict, session: str, text: str, agent: str | None) -> None:
    """prompt_async, then wait until the session stored the prompt.

    prompt_async returns 204 before it runs the prompt and logs a failure only on the
    server, so a dropped prompt looked delivered and its sender waited for a reply that
    could never come.
    """
    payload: dict = {"parts": [{"type": "text", "text": text}]}
    if agent:
        payload["agent"] = agent
    http_json("POST", opencode_url(cfg, f"/session/{session}/prompt_async"), payload)
    deadline = time.monotonic() + CONFIRM_SECONDS
    while not stored_prompt(cfg, session, text):
        if time.monotonic() >= deadline:
            raise PairError(f"opencode session {session} did not take the prompt (it answered "
                            f"prompt_async but stored nothing in {CONFIRM_SECONDS:g}s; its log "
                            "says prompt_async failed, e.g. for an unknown agent or model)")
        time.sleep(CONFIRM_POLL_SECONDS)


def opencode_prompt(cfg: dict, role: str, text: str, agent: str | None = None) -> None:
    session = cfg["opencode"].get("session")
    if not session:
        raise PairError("the opencode session is not bound yet (pair.py launch binds it)")
    prompt_session(cfg, session, text, agent or cfg["roles"][role].get("agent"))


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
    if sender == "writer" and uses_threads(cfg, sender) and body.startswith(THREAD_ONLY_KINDS):
        # one main channel serialises every decision behind the main reviewer's turn
        raise PairError("with an OpenCode reviewer a decision runs in a liaison thread, never "
                        "on the main channel (pair.md, Liaison). A liaison that returned "
                        "blocked or unresponsive: follow Peer not reachable.", code=7)
    label = USER_REQUEST_LABEL if args.user_request else LABELS[sender]
    if body.startswith("FYI:") and not args.user_request:
        held = put_message(held_box(base, to), sender, f"{label} {body}")
        if len(held_messages(base, to)) < FYI_BATCH:
            print(json.dumps({"sent": held.name, "delivered": False, "held": True}))
            return 0
        path = release_held(base, to)
    else:
        release_held(base, to)
        path = write_message(base, to, sender, f"{label} {body}")
    return deliver(base, cfg, to, path)


def held_box(base: Path, role: str) -> Path:
    return pair_dir(base) / "held" / role


def held_messages(base: Path, role: str) -> list[Path]:
    box = held_box(base, role)
    return sorted(p for p in box.glob("*.md") if p.is_file()) if box.is_dir() else []


def release_held(base: Path, role: str) -> Path | None:
    """Move the held FYIs for `role` into its inbox as ONE message, so they cost one turn."""
    held = held_messages(base, role)
    if not held:
        return None
    texts = [p.read_text(encoding="utf-8").rstrip("\n") for p in held]
    sender = held[0].stem.split("-", 1)[1]
    path = write_message(base, role, sender, "\n".join(texts))
    for p in held:
        p.unlink()
    return path


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


def uses_threads(cfg: dict, role: str) -> bool:
    """A Claude writer with an OpenCode reviewer settles decisions in liaison threads."""
    return (role == "writer" and cfg["roles"]["writer"]["kind"] == "claude"
            and cfg["roles"]["reviewer"]["kind"] == "opencode")


UUID_RE = r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
SECRET_PATTERNS = (
    re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----", re.S),
    # KEY=value / token: value, where the key names a secret  # pragma: allowlist secret
    re.compile(r"(?i)(?<=\b)((?:api[-_ ]?key|token|secret|password|passwd|authorization|cookie|bearer)"
               r"[\"']?\s*[:=]?\s*[\"']?)[^\s\"',;]{6,}"),
    # long opaque tokens: mixed case and digits, 24+ chars, not part of a path
    re.compile(r"(?<![\w\-/.])(?=[\w\-]*[A-Z])(?=[\w\-]*[a-z])(?=[\w\-]*\d)[\w\-]{24,}(?![\w\-/.])"),
    # UUID keys after a key label; a bare UUID (a cid, a session id) stays readable
    re.compile(rf"(?i)(\bkeys?\b[\"']?\s*[:=]?\s*){UUID_RE}(?:\s+{UUID_RE})*"),
)


def redact_secrets(text: str) -> str:
    """Mask anything that looks like a credential before it leaves this agent."""
    text = SECRET_PATTERNS[0].sub("[REDACTED]", text)
    text = SECRET_PATTERNS[3].sub(r"\1[REDACTED]", text)
    text = SECRET_PATTERNS[1].sub(r"\1[REDACTED]", text)
    return SECRET_PATTERNS[2].sub("[REDACTED]", text)


def user_said(base: Path, cfg: dict, role: str, prompt: str) -> str:
    """The message that tells the peer what the user typed into `role`'s pane."""
    # the peer may run on another provider: a token the user pasted stays with this agent
    prompt = redact_secrets(prompt)
    label = (USER_SAID_THREAD_LABEL if uses_threads(cfg, role) else USER_SAID_LABEL)
    label = label.format(role=role)
    body = f"{label} {prompt}"
    if len(body.encode("utf-8")) <= MAX_BODY_BYTES:
        return body
    long_file = pair_dir(base) / "files" / f"user-to-{role}-{time.time_ns()}.md"
    long_file.parent.mkdir(parents=True, exist_ok=True)
    long_file.write_text(prompt + "\n", encoding="utf-8")
    return f"{label} (long message, read it in {long_file})"


ROUTE_RULE = (
    "ROUTE this message: when it names one of these threads or goes on with its topic, it "
    "belongs to that thread. Open: its liaison still runs, so SendMessage it the follow-up, "
    "as user added: <the message verbatim>. Agreed, disputed or closed: start a liaison with "
    "thread: resume <id>. About none of them: thread: new.")


def thread_routes(base: Path, cfg: dict) -> str:
    """The threads a user message may continue, for the writer's routing; '' when none."""
    session = cfg["opencode"].get("session")
    # a thread of an earlier reviewer process cannot be resumed: its subagent is gone
    rows = [(path, meta) for path, meta in all_threads(base)
            if meta.get("pair_session") == session]
    if not rows:
        return ""
    lines = ["PAIR THREADS, newest first:"]
    for path, meta in reversed(rows[-THREADS_LISTED:]):
        line = (f"{meta['id']} {meta['status']} {age(meta['created_ms'])}: {meta['topic']} "
                f"[{', '.join(meta['files'])}]")
        decision = path / "decision.md"
        if meta["status"] != "open" and decision.is_file():
            first = decision.read_text(encoding="utf-8").strip().splitlines()[:1]
            if first:
                line += f" decided: {first[0][:160]}"
        lines.append(line)
    lines.append(ROUTE_RULE)
    return "\n".join(lines)


STAGE_LINE = "TASK STAGE: {stage} (SKILL.md, Stages)."
PLAN_CLOSED_LINE = "PLAN.md is closed: all feedback is implementation work; task reviews reopens it."
PLAN_CLOSED = (
    "PLAN.md is closed: the user approved it, and PLAN to IMPL is one-way. Only the user's "
    "task reviews reopens it, through pair.py fresh --then reviews (references/reviews.md). "
    "Until then it is frozen, "
    "general direction only. All feedback is implementation work: decision loop, builder or "
    "fixer, pre-PR diff gate with the code delta. No planner, no plan diff, no plan gate, "
    "no stage change, no restart for it. Ask the user only for critical decisions "
    "(SKILL.md, Autonomy). Resolve other choices, log them, and implement. User decisions go to "
    f"{USER_DECISIONS_FILE} through pair.py user-decided (SKILL.md, Stages).")
UNKNOWN_STAGE_NOTE = (
    "TASK STAGE: this pair started before the stage was recorded, so it is PLAN now. Only "
    "when the user explicitly approved PLAN.md, run python3 {py} fresh --base {base} --then impl "
    "(SKILL.md, Stages); never infer the approval from PLAN.md.")


def stage_note(base: Path, cfg: dict, role: str, session_id: object) -> str:
    """The stage line for a hook; records an unknown stage as plan and the Claude session id.

    The session id changes after /clear, and `stage` resumes the recorded one.
    """
    changed = False
    if cfg.get("stage") != "impl" and impl_reached(base):
        cfg["stage"] = "impl"
        changed = True
    note = STAGE_LINE.format(stage=str(cfg.get("stage", "")).upper())
    if cfg.get("stage") == "impl":
        note = f"{note} {PLAN_CLOSED_LINE}"
    if cfg.get("stage") not in STAGES:
        cfg["stage"] = "plan"
        note = UNKNOWN_STAGE_NOTE.format(py=PAIR_PY, base=base)
        changed = True
    sessions = cfg.setdefault("claude_session", {})
    if (isinstance(session_id, str) and UUID_RE.fullmatch(session_id)
            and cfg["roles"][role]["kind"] == "claude" and sessions.get(role) != session_id):
        sessions[role] = session_id
        changed = True
    if changed:
        try:
            save_config(base, cfg)
        except OSError:
            pass  # the next hook run records it
    return note


def cmd_user_prompt(args: argparse.Namespace) -> int:
    """UserPromptSubmit hook of a Claude Code agent in pair mode.

    Forwards what the user typed to the peer and reminds this agent, in its
    context, that a clear message is handled alone and a hard one by the pair. It
    never blocks the prompt: on any failure it only says so in the context.
    """
    peer = other(args.role)
    try:
        payload = json.load(sys.stdin)
        prompt = str(payload.get("prompt", "")).strip()
    except (json.JSONDecodeError, AttributeError):
        payload, prompt = {}, ""
    # claude code also fires the hook for background-task notifications, which no user typed
    if not prompt or prompt.startswith("<task-notification>"):
        return 0
    errors = (PairError, OSError, KeyError, ValueError, subprocess.TimeoutExpired)
    reminder = PAIR_REMINDER_DIRECT
    try:
        base = resolve_base(args.base)
        cfg = load_config(base)
        if uses_threads(cfg, args.role):
            reminder = PAIR_REMINDER
        stage_line = stage_note(base, cfg, args.role, payload.get("session_id"))
        if FRESH_PROMPT_RE.match(prompt):
            # the first prompt of a `fresh` restart comes from the launcher, not the user
            print(stage_line)
            return 0
        release_held(base, peer)
        path = write_message(base, peer, "user", user_said(base, cfg, args.role, prompt))
    except errors as exc:
        print(reminder.format(
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
    print(reminder.format(forwarded=forwarded, peer=peer))
    print(stage_line)
    if uses_threads(cfg, args.role):
        try:
            routes = thread_routes(base, cfg)
        except errors:
            routes = ""
        if routes:
            print(routes)
    try:
        idle_marker(base, args.role).unlink(missing_ok=True)  # a user message is new work
        note = waiter_down_note(base, args.role) if cfg["roles"][args.role]["kind"] == "claude" else ""
    except errors:
        note = ""
    if note:
        print(note)
    return 0


def claude_settings(base: Path, role: str) -> dict:
    args = f"--base {shlex.quote(str(base))} --role {role}"
    prompt = f"python3 {shlex.quote(str(PAIR_PY))} user-prompt {args}"
    start = f"python3 {shlex.quote(str(PAIR_PY))} session-start {args}"
    guard = f"python3 {shlex.quote(str(PAIR_PY))} guard {args}"
    edit_guard = f"python3 {shlex.quote(str(PAIR_PY))} edit-guard {args}"
    hooks = {"hooks": {
        "UserPromptSubmit": [
            {"hooks": [{"type": "command", "command": prompt, "timeout": 20}]}],
        # a Monitor expires after 30 minutes; these catch a waiter nobody re-armed
        "PostToolUse": [{"hooks": [{"type": "command", "command": guard, "timeout": 20}]}],
        "Stop": [{"hooks": [{"type": "command", "command": guard, "timeout": 20}]}],
        # /clear (typed as /clear, /new or /reset) keeps the process and its old waiter
        "SessionStart": [
            {"matcher": "clear", "hooks": [{"type": "command", "command": start, "timeout": 20}]}],
    }}
    if role == "writer":
        # a file of a decision still under discussion in a pair thread is not typed yet
        hooks["hooks"]["PreToolUse"] = [{"matcher": EDIT_TOOLS, "hooks": [
            {"type": "command", "command": edit_guard, "timeout": 20}]}]
    return hooks


WAITER_DOWN = (
    "PAIR WAITER DOWN: your pair.py wait is not running (its Monitor expired or stopped), so "
    "the {peer}'s messages do not reach you. Before any other step, start the Monitor tool "
    "on python3 {py} wait --follow --base {base} --role {role} (timeout_ms 1800000, "
    "description pair messages), then handle the messages above, if any, per pair.md. "
    "If the task is finished and nothing is pending, run python3 {py} idle --base {base} "
    "--role {role} instead and end the turn.")


def idle_marker(base: Path, role: str) -> Path:
    return pair_dir(base) / f"idle-{role}"


IDLE_NOTE = (
    "PAIR IDLE: the {role} has no work. Its waiter is stopped and the pair hooks stay quiet "
    "until new work arrives. Do NOT start the Monitor again; end your turn. A user message "
    "ends idle mode.")


def waiter_down_note(base: Path, role: str) -> str:
    """Hand over the backlog of a dead waiter and say how to re-arm it; '' when listening.

    An idle agent with nothing queued is left alone: there is nothing to wait for.
    """
    if waiter_state(base, role) != "absent":
        return ""
    messages = unread(base, role)
    marker = idle_marker(base, role)
    if marker.exists():
        if not messages:
            return ""
        marker.unlink(missing_ok=True)  # a queued message is new work
    parts = []
    if messages:
        parts.append(f"PAIR MESSAGES for the {role} ({len(messages)}), queued while no waiter ran:")
        for path in messages:
            try:
                parts.append(path.read_text(encoding="utf-8").rstrip("\n"))
                mark_read(base, role, path)
            except FileNotFoundError:
                parts.pop()  # a waiter that just started took it
    parts.append(WAITER_DOWN.format(peer=other(role), py=PAIR_PY, base=base, role=role))
    return "\n".join(parts)


def cmd_guard(args: argparse.Namespace) -> int:
    """PostToolUse and Stop hook of a Claude Code agent in pair mode.

    The Monitor that runs `wait --follow` expires after at most 30 minutes, and an
    agent busy with long work forgets to re-arm it: the peer's messages then pile up
    unseen. This hands the backlog over and tells the agent to re-arm. It never fails
    the tool call or the stop.
    """
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        payload = {}
    if not isinstance(payload, dict) or payload.get("agent_id"):
        return 0  # a subagent must never take the main agent's messages
    event = payload.get("hook_event_name", "")
    if event == "PostToolUse" and payload.get("tool_name") == "Monitor":
        return 0  # the call that starts the waiter, before its pid file exists
    if event == "Stop" and payload.get("stop_hook_active"):
        return 0  # already blocked once: never loop
    errors = (PairError, OSError, KeyError, ValueError)
    try:
        base = resolve_base(args.base)
        cfg = load_config(base)
        if cfg["roles"][args.role]["kind"] != "claude":
            return 0
        note = waiter_down_note(base, args.role)
    except errors:
        return 0
    if not note:
        return 0
    if event == "Stop":
        print(json.dumps({"decision": "block", "reason": note}))
    else:
        print(json.dumps({"hookSpecificOutput": {"hookEventName": event or "PostToolUse",
                                                 "additionalContext": note}}))
    return 0


EDIT_DENIED = ("PAIR: {file} belongs to pair thread {id} ({topic}), which is still open: "
               "its decision is not agreed yet. Do other work, or end the turn; the liaison "
               "returns the decision (pair.md, Liaison). Mechanical work inside an agreed "
               "decision is never blocked: only an open thread blocks its files.")


def cmd_edit_guard(args: argparse.Namespace) -> int:
    """PreToolUse hook of a Claude writer: refuse an edit to a file of an open thread, and
    to PLAN.md at the base on IMPL.

    It covers the writer and its subagents (builders, fixers, a planner). It never fails the
    tool call on its own error: a broken guard must not stop the work.
    """
    try:
        payload = json.load(sys.stdin)
        tool_input = payload.get("tool_input") or {}
        target = tool_input.get("file_path") or tool_input.get("notebook_path")
        if not target:
            return 0
        base = resolve_base(args.base)
        path = Path(target)
        if not path.is_absolute():
            path = Path(payload.get("cwd") or base) / path
        rel = os.path.relpath(path.resolve(), base.resolve())
        if rel.startswith("..") or rel.startswith(".pair" + os.sep):
            return 0
        if rel == "PLAN.md" and impl_reached(base):
            print(json.dumps({"hookSpecificOutput": {
                "hookEventName": "PreToolUse", "permissionDecision": "deny",
                "permissionDecisionReason": f"PAIR: {PLAN_CLOSED}"}}))
            return 0
        for _dir, meta in all_threads(base):
            if meta["status"] != "open":
                continue
            for theirs in meta["files"]:
                if overlaps(rel, theirs):
                    print(json.dumps({"hookSpecificOutput": {
                        "hookEventName": "PreToolUse", "permissionDecision": "deny",
                        "permissionDecisionReason": EDIT_DENIED.format(
                            file=rel, id=meta["id"], topic=meta["topic"])}}))
                    return 0
    except (PairError, OSError, KeyError, ValueError, AttributeError, TypeError):
        return 0
    return 0


SESSION_START_NOTE = (
    "PAIR MODE resumed after /clear: you are still the {role_upper} of the pair, the "
    "{peer} runs in the {pane} pane. pair.py stopped your old waiter{focused}. Re-read "
    "{pair_md}, then PLAN.md, {base}/.pair/decisions.md (every pair decision so far) and "
    "{base}/" + USER_DECISIONS_READ + " to catch up, and the latest files in "
    "{base}/.pair/inbox/*/read/ for what they do not cover.{rearm}")

SESSION_START_REARM = (
    " Start the Monitor tool on python3 {py} wait --follow --base {base} --role {role} "
    "now (timeout_ms 1800000).")
SESSION_START_IDLE = " You are idle (no work): do not start the Monitor until new work arrives."


def cmd_session_start(args: argparse.Namespace) -> int:
    """SessionStart hook (matcher clear) of a Claude Code agent in pair mode."""
    errors = (PairError, OSError, KeyError, ValueError, subprocess.TimeoutExpired)
    try:
        base = resolve_base(args.base)
        cfg = load_config(base)
    except errors:
        return 0
    role, peer = args.role, other(args.role)
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        payload = {}
    # /clear starts a new conversation id: `stage` must resume this one
    stage = stage_note(base, cfg, role, payload.get("session_id") if isinstance(payload, dict)
                       else None)
    retire_waiter(base, role)
    focused = ""
    if cfg["roles"][peer]["kind"] == "opencode":
        try:
            focus_opencode(cfg)
            focused = f" and put the {peer} pane back on the pair session"
        except errors:
            pass
    rearm = (SESSION_START_IDLE if idle_marker(base, role).exists()
             else SESSION_START_REARM.format(py=PAIR_PY, base=base, role=role))
    print(SESSION_START_NOTE.format(role_upper=role.upper(), role=role, peer=peer,
                                    pane=PANES[peer], focused=focused, base=base,
                                    pair_md=SKILL_DIR / "pair.md", rearm=rearm))
    print(stage)
    return 0


def claude_settings_file(base: Path, role: str) -> Path:
    return pair_dir(base) / f"claude-{role}-settings.json"


SUPERSEDED = ("SUPERSEDED: a newer pair.py wait of the {role} took over this one. "
              "Do NOT start another wait for it.")


def recorded_waiter(base: Path, role: str) -> int | None:
    try:
        return int(waiter_pid_file(base, role).read_text().strip())
    except (FileNotFoundError, ValueError):
        return None


def retire_waiter(base: Path, role: str, pid: int | None = None) -> None:
    """Stop the recorded waiter of `role`, e.g. one a /clear left running.

    Two waiters race for one inbox, and the old one hands the batch to a
    conversation that no longer exists.
    """
    pid = recorded_waiter(base, role) if pid is None else pid
    if pid is None:
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


def cmd_idle(args: argparse.Namespace) -> int:
    """Enter idle mode: no work left, so stop the waiter and silence the re-arm nags."""
    base = resolve_base(args.base)
    # the marker goes first: the guard after this call must not see a bare dead waiter
    idle_marker(base, args.role).write_text("idle\n")
    retire_waiter(base, args.role)
    # the FYIs this agent held for the peer must not wait for work that may never come
    # (a Claude peer reads them with its next wait; a send now would mark it blocked)
    peer = other(args.role)
    if release_held(base, peer) is not None:
        try:
            cfg = load_config(base)
            if cfg["roles"][peer]["kind"] == "opencode":
                flush_opencode(base, cfg, peer)
        except (PairError, OSError, KeyError):
            pass  # the message stays in the peer's inbox for its next flush
    print(IDLE_NOTE.format(role=args.role))
    return 0


def cmd_wait(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    role = args.role
    pid_file = waiter_pid_file(base, role)
    old = recorded_waiter(base, role)
    # claim the pid file first: the old waiter then knows a newer one superseded it
    pid_file.write_text(f"{os.getpid()}\n")
    retire_waiter(base, role, old)
    idle_marker(base, role).unlink(missing_ok=True)

    def stopped(_signum, _frame):
        try:
            mine = pid_file.read_text().strip() == str(os.getpid())
        except FileNotFoundError:
            mine = False
        if mine:
            # killed by its owner (a Monitor expiry, a /clear): nothing to say
            pid_file.unlink(missing_ok=True)
        else:
            print(SUPERSEDED.format(role=role))
            sys.stdout.flush()
        # skip the finally below: the pid file is gone or belongs to the newer waiter
        os._exit(0)

    signal.signal(signal.SIGTERM, stopped)
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
                if args.follow:
                    # under the Monitor tool each batch is one event; keep listening
                    sys.stdout.flush()
                    continue
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


def respawn(old: str, name: str, cwd: str, command: str) -> str:
    """Replace a session with a new one right after it, then close the old one."""
    try:
        tint = find_session(old).get("tint")
    except PairError:
        tint = None
    out = ctl("session", "new", "--cwd", cwd, "--name", name,
              "--after", old, "--no-select", "--command", command)
    match = UUID_RE.search(out)
    if not match:
        raise PairError(f"agtermctl session new printed no session id: {out.strip()}")
    if tint:
        try:
            ctl("session", "tint", tint, "--target", match.group(0))
        except (PairError, OSError, subprocess.TimeoutExpired):
            pass
    if os.environ.get("AGTERM_SESSION_ID", "").lower() == old.lower():
        # this process runs inside the old session: close it once we have returned
        ctl_bin = os.environ.get("AGTERMCTL", "agtermctl")
        subprocess.Popen(["/bin/sh", "-c", f"sleep 2; {shlex.quote(ctl_bin)} session close "
                          f"--target {shlex.quote(old)}"],
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                         start_new_session=True)
    else:
        ctl("session", "close", "--target", old)
    return match.group(0)


def cmd_respawn(args: argparse.Namespace) -> int:
    refuse_live_companions(args.old)
    print(respawn(args.old, args.name, args.cwd, args.command))
    return 0


def cmd_flush(args: argparse.Namespace) -> int:
    base = resolve_base(args.base)
    cfg = load_config(base)
    if cfg["roles"][args.to]["kind"] != "opencode":
        raise PairError(f"the {args.to} is a Claude Code agent; its next wait delivers the backlog")
    release_held(base, args.to)
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
    unknown = cfg.get("stage") not in STAGES
    note = stage_note(base, cfg, "writer", None)
    out: dict = {"base": str(base), "stage": cfg["stage"], "roles": {}}
    if unknown:
        out["stage_note"] = note
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


def reviewer_opencode_config(stage: str) -> dict:
    return {
        "$schema": "https://opencode.ai/config.json",
        # one agent on the stage model: `stage` restarts the reviewer, which rewrites this
        "agent": {REVIEWER_AGENT: {
            "description": f"Read-only reviewer in task pair mode, {stage} stage",
            # "all": a thread's child session runs it as a subagent too
            "mode": "all",
            **MODELS[stage]["opencode"],
            "permission": {"edit": "deny", "bash": "allow"},
        }},
        # keeps the pane on the pair session after a /new there
        "plugin": [(PAIR_PY.parent / "pair-opencode-plugin.js").as_uri()],
    }


def exclude_from_git(base: Path, entry: str = "/.pair/") -> None:
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
    if entry not in lines:
        path.write_text("\n".join([*lines, entry]) + "\n")


def single_record(base: Path) -> dict:
    try:
        rec = json.loads((base / STAGE_FILE).read_text())
    except FileNotFoundError:
        return {}
    except json.JSONDecodeError as exc:
        raise PairError(f"{base / STAGE_FILE} is not JSON: {exc}") from exc
    return rec if isinstance(rec, dict) else {}


def is_pair(base: Path, single: bool) -> bool:
    """Pair mode keeps the stage in .pair/config.json, a single session in .task-stage.

    --single: a single session opened beside a pair (task switch --single).
    """
    return not single and (pair_dir(base) / "config.json").is_file()


def impl_reached(base: Path) -> bool:
    """One impl record is enough: a stale record of the other mode never reopens the plan."""
    pair_cfg = pair_dir(base) / "config.json"
    return ((pair_cfg.is_file() and load_config(base).get("stage") == "impl")
            or single_record(base).get("stage") == "impl")


def refuse_plan_reopen(base: Path) -> None:
    if impl_reached(base):
        raise PairError(PLAN_CLOSED)


def reopen_plan(base: Path) -> None:
    """task reviews: clear the impl stage in both records, so no stale one keeps it."""
    pair_cfg = pair_dir(base) / "config.json"
    if pair_cfg.is_file():
        cfg = load_config(base)
        if cfg.get("stage") == "impl":
            cfg["stage"] = "plan"
            save_config(base, cfg)
    single = single_record(base)
    if single.get("stage") == "impl":
        single["stage"] = "plan"
        save_record(base, True, single)


def load_record(base: Path, single: bool) -> dict:
    rec = load_config(base) if is_pair(base, single) else single_record(base)
    if impl_reached(base):
        rec["stage"] = "impl"
    return rec


def save_record(base: Path, single: bool, rec: dict) -> None:
    if is_pair(base, single):
        save_config(base, rec)
        return
    path = base / STAGE_FILE
    tmp = path.with_name(STAGE_FILE + ".tmp")
    tmp.write_text(json.dumps(rec, indent=2) + "\n")
    tmp.replace(path)
    exclude_from_git(base, f"/{STAGE_FILE}*")


def record_stage(rec: dict) -> str:
    """The recorded stage; an unknown one (a session older than stages) is plan."""
    if rec.get("stage") not in STAGES:
        rec["stage"] = "plan"
    return rec["stage"]


def claude_transcript_exists(session_id: str) -> bool:
    root = Path(os.environ.get("CLAUDE_CONFIG_DIR") or Path.home() / ".claude") / "projects"
    return any(root.glob(f"*/{session_id}.jsonl"))


def stage_flags(base: Path, rec: dict, role: str, pair: bool) -> str:
    """The claude flags of the stage: resume the recorded conversation, or start a new one
    under a recorded id. Updates `rec`; the caller saves it."""
    stage = record_stage(rec)
    if pair:
        sessions = rec.setdefault("claude_session", {})
        # a single session turned into the writer of a pair goes on with its conversation
        sid = sessions.get(role) or (single_record(base).get("claude_session")
                                     if role == "writer" and not rec.get("fresh") else None)
    else:
        sid = rec.get("claude_session")
    if sid and claude_transcript_exists(sid):
        flag = f"--resume {sid}"
    else:
        # a recorded id whose session never started is still free
        sid = sid or str(uuid.uuid4())
        flag = f"--session-id {sid}"
    if pair:
        rec["claude_session"][role] = sid
    else:
        rec["claude_session"] = sid
    model = MODELS[stage]["claude"]
    flags = f"{flag} --model {model['model']} --effort {model['effort']}"
    if rec.get("fresh"):
        flags = " ".join(filter(None, (flags, fresh_claude_flags(base, role, pair))))
    return flags


# plugins whose SessionStart hook loads a skill into every new conversation. A `fresh`
# conversation runs them without their hooks: their skills load on request only.
AUTO_CONTEXT_PLUGINS = ("superpowers@claude-plugins-official",)
FRESH_DIR = ".task-fresh"


def enabled_plugin_root(plugin_id: str) -> Path | None:
    """The install directory of a plugin the user enabled; None when it is not."""
    root = Path(os.environ.get("CLAUDE_CONFIG_DIR") or Path.home() / ".claude")
    try:
        enabled = json.loads((root / "settings.json").read_text()).get("enabledPlugins") or {}
        installed = json.loads((root / "plugins" / "installed_plugins.json").read_text())
    except (OSError, json.JSONDecodeError):
        return None
    if enabled.get(plugin_id) is not True:
        return None
    for entry in installed.get("plugins", {}).get(plugin_id) or []:
        path = Path(entry.get("installPath", ""))
        if path.is_dir():
            return path
    return None


def fresh_claude_flags(base: Path, role: str, pair: bool) -> str:
    """The --settings and --plugin-dir flags of a `fresh` Claude conversation; '' when no
    AUTO_CONTEXT_PLUGINS plugin is enabled.

    Each such plugin is switched off and loaded again from a copy without its hooks, under
    the same name, so `superpowers:<skill>` still resolves. Claude Code applies only the last
    --settings, so in pair mode the file carries the pair hooks too.
    """
    folder = base / FRESH_DIR
    disabled, dirs = {}, []
    for plugin_id in AUTO_CONTEXT_PLUGINS:
        source = enabled_plugin_root(plugin_id)
        if not source:
            continue
        name = plugin_id.split("@")[0]
        copy = folder / "plugins" / name
        (copy / ".claude-plugin").mkdir(parents=True, exist_ok=True)
        (copy / ".claude-plugin" / "plugin.json").write_text(json.dumps(
            {"name": name, "description": f"the {name} skills without its hooks",
             "version": "0.0.0"}, indent=2) + "\n")
        for part in ("skills", "agents", "commands"):
            link = copy / part
            if link.is_symlink():
                link.unlink()
            if (source / part).is_dir():
                link.symlink_to(source / part)
        disabled[plugin_id] = False
        dirs.append(copy)
    if not disabled:
        return ""
    settings = claude_settings(base, role) if pair else {}
    settings["enabledPlugins"] = disabled
    path = folder / f"claude-{role}-settings.json"
    path.write_text(json.dumps(settings, indent=2) + "\n")
    exclude_from_git(base, f"/{FRESH_DIR}/")
    # double quotes: the launchers nest these flags in '...' on a shell line
    return " ".join([f"--settings \"{path}\"", *(f"--plugin-dir \"{d}\"" for d in dirs)])


def latest_opencode_session(base: Path) -> str | None:
    """The newest OpenCode session of this directory, for a writer that recorded none."""
    try:
        out = subprocess.run(["opencode", "session", "list", "--format", "json", "-n", "50"],
                             capture_output=True, text=True, timeout=30, check=False).stdout
        sessions = json.loads(out or "[]")
    except (OSError, subprocess.TimeoutExpired, json.JSONDecodeError):
        return None
    mine = [x for x in sessions if isinstance(x, dict)
            and Path(x.get("directory", "")).resolve() == base.resolve()]
    return max(mine, key=lambda x: x.get("updated", 0))["id"] if mine else None


def record_launcher(rec: dict, args: argparse.Namespace) -> None:
    if args.launcher:
        if not args.branch:
            raise PairError("--launcher needs --branch")
        rec["launcher"] = {"script": str(Path(args.launcher).resolve()), "branch": args.branch}


def cmd_claude_args(args: argparse.Namespace) -> int:
    """Print the claude flags of the task stage for a role, for a launcher."""
    base = Path(args.base).resolve()
    pair = is_pair(base, args.single)
    if args.role == "reviewer" and not pair:
        raise PairError("--role reviewer needs a pair (.pair/config.json)")
    rec = load_record(base, args.single)
    record_launcher(rec, args)
    if args.resume_id:
        if not UUID_RE.fullmatch(args.resume_id):
            raise PairError(f"--resume-id {args.resume_id} is not a claude session id")
        if pair:
            rec.setdefault("claude_session", {})[args.role] = args.resume_id
        else:
            rec["claude_session"] = args.resume_id
    flags = stage_flags(base, rec, args.role, pair)
    save_record(base, args.single, rec)
    print(flags)
    return 0


def cmd_opencode_args(args: argparse.Namespace) -> int:
    """Print the opencode writer flags of the task stage: its agent, and its session."""
    base = Path(args.base).resolve()
    rec = load_record(base, args.single)
    record_launcher(rec, args)
    stage = record_stage(rec)
    session = rec.get("opencode_session")
    if not session and args.resume and not rec.get("fresh"):
        session = latest_opencode_session(base)
    save_record(base, args.single, rec)
    flags = f"--agent {MODELS[stage]['writer_agent']}"
    print(f"{flags} --session {session}" if session else flags)
    return 0


SESSION_ID_RE = re.compile(r"[A-Za-z0-9_-]+")


def restart_record(args: argparse.Namespace) -> dict:
    """The record of a restart; refused with exit 5 while a pair thread is open."""
    base = Path(args.base).resolve()
    if is_pair(base, args.single):
        busy = [meta["id"] for _path, meta in all_threads(base) if meta["status"] == "open"]
        if busy:
            raise PairError(f"thread {', '.join(busy)} is open: settle and close it before the "
                            "sessions restart (its liaison and reviewer subagent die with the "
                            "restart)", code=5)
    rec = load_record(base, args.single)
    record_launcher(rec, args)
    return rec


def refuse_live_companions(old: str) -> None:
    """Exit 5 while `old` has companions: closing a parent closes them too."""
    tree = json.loads(ctl("tree", "--json"))
    names = [node.get("name") or node["id"]
             for workspace in tree["result"]["tree"]["workspaces"]
             for node in workspace.get("sessions", [])
             if str(node.get("parent", "")).lower() == old.lower()]
    if names:
        raise PairError(f"companion session {', '.join(names)} is open: wait for it or close "
                        "it, then run this again (closing the task session closes its "
                        "companions)", code=5)


def restart(args: argparse.Namespace, rec: dict, stage: str, purpose: str | None) -> int:
    """Restart the sessions of the task on the models of `stage` (SKILL.md, Stages).

    A running session cannot switch its model or clear its context, so the writer terminal
    is respawned with the command the project launcher prints, and in pair mode `launch
    --reopen` starts the reviewer in the new split. The old terminal closes with both
    panes, so the reviewer is never restarted on its own first. The restart runs detached:
    this command usually runs inside the session it replaces.

    Without `purpose` every agent resumes its recorded conversation. With one (`fresh`)
    every agent starts a new conversation, and the writer gets the first prompt of that
    purpose (FRESH_PROMPTS).
    """
    base = Path(args.base).resolve()
    pair = is_pair(base, args.single)
    old = rec.get("agterm_session") if pair else os.environ.get("AGTERM_SESSION_ID")
    if not old:
        raise PairError("no agterm session to restart: " + (
            "pair.py launch records it" if pair else "run it inside the task session"))
    if not rec.get("launcher"):
        raise PairError("no launcher recorded for this task (it was opened before stages): "
                        "pass --launcher <the project new-task.sh> --branch <branch>")
    refuse_live_companions(old)
    if purpose == "reviews":
        # the one way back from IMPL: the record of the other mode must not keep impl
        reopen_plan(base)
    if purpose:
        # no conversation to resume: never an old id, never the newest OpenCode session
        rec.pop("claude_session", None)
        rec.pop("opencode_session", None)
        rec["fresh"] = purpose
    elif (pair and rec["roles"]["writer"]["kind"] == "claude"
            and not rec.get("claude_session", {}).get("writer")):
        raise PairError("the conversation of the Claude writer is not recorded: run stage "
                        "from the writer, or pass --session-id writer=<id>")
    rec["stage"] = stage
    if stage == "impl":
        release_plan_log(base)
    if pair:
        if rec["roles"]["reviewer"]["kind"] == "opencode":
            # every reader follows it: prompts, thread children, the pane plugin
            rec["roles"]["reviewer"]["agent"] = REVIEWER_AGENT
        elif rec["opencode"].get("session") and not purpose:
            # the OpenCode writer resumes the session that launch bound
            rec["opencode_session"] = rec["opencode"]["session"]
    save_record(base, args.single, rec)
    handoff = [sys.executable, str(PAIR_PY), "stage-handoff", "--base", str(base),
               "--old", old, "--wait-seconds", str(args.wait_seconds)]
    if args.single:
        handoff.append("--single")
    if args.foreground:
        return main(handoff[2:])
    log = (pair_dir(base) / "stage.log") if pair else base / f"{STAGE_FILE}.log"
    env = dict(os.environ)
    # detached from the old session: respawn closes it at once, not after we return
    env.pop("AGTERM_SESSION_ID", None)
    with open(log, "w") as out:
        subprocess.Popen(handoff, stdin=subprocess.DEVNULL, stdout=out,
                         stderr=subprocess.STDOUT, env=env, start_new_session=True)
    print(json.dumps({"stage": stage, "changed": True, "restart": "started",
                      "log": str(log)}))
    return 0


def cmd_stage(args: argparse.Namespace) -> int:
    """Switch the task stage and restart the sessions on its models, each agent in its
    recorded conversation (SKILL.md, Stages)."""
    base = Path(args.base).resolve()
    if args.stage == "plan":
        refuse_plan_reopen(base)
    rec = restart_record(args)
    pair = is_pair(base, args.single)
    # a Claude writer runs this command: its own conversation is the one to resume
    own = os.environ.get("CLAUDE_CODE_SESSION_ID", "")
    if UUID_RE.fullmatch(own):
        if not pair:
            rec["claude_session"] = own
        elif rec["roles"]["writer"]["kind"] == "claude":
            rec.setdefault("claude_session", {})["writer"] = own
    for item in args.session_id or []:
        role, _, sid = item.partition("=")
        if role not in ROLES or not SESSION_ID_RE.fullmatch(sid):
            raise PairError(f"--session-id {item}: use ROLE=ID, ROLE writer or reviewer")
        if sid.startswith("ses_"):
            if role != "writer":
                raise PairError("only the OpenCode writer resumes a recorded session")
            rec["opencode_session"] = sid
        elif pair:
            rec.setdefault("claude_session", {})[role] = sid
        else:
            rec["claude_session"] = sid
    if rec.get("stage") == args.stage:
        save_record(base, args.single, rec)
        print(json.dumps({"stage": args.stage, "changed": False}))
        return 0
    return restart(args, rec, args.stage, None)


def cmd_fresh(args: argparse.Namespace) -> int:
    """Restart the writer and the reviewer in new conversations on the same task, worktree
    and agterm session (references/fresh.md), even on the same stage.

    --then picks the stage and the first prompt: plan or reviews (PLAN), impl or finish
    (IMPL). reviews is the only one that reopens the plan on IMPL (references/reviews.md).
    Without it the recorded stage stays and the writer reports where the task stands.
    """
    if args.then == "plan":
        refuse_plan_reopen(Path(args.base).resolve())
    rec = restart_record(args)
    stage = FRESH_STAGES[args.then] if args.then else record_stage(rec)
    return restart(args, rec, stage, args.then or "continue")


def cmd_plan_open(args: argparse.Namespace) -> int:
    """Exit 1 on IMPL: the plan gate scripts run this before they touch PLAN.md."""
    refuse_plan_reopen(Path(args.base).resolve())
    return 0


def launcher_command(rec: dict, single: bool, kind: str, pair: bool) -> str:
    """The writer start command the project launcher prints for the recorded stage."""
    launcher = rec["launcher"]
    argv = [launcher["script"], launcher["branch"], "--print-command"]
    if single:
        argv.append("--single")
    env = dict(os.environ)
    env.pop("OPENCODE", None)
    env.pop("TASK_CONTINUE", None)
    if kind == "opencode":
        env["OPENCODE"] = "1"
    if rec.get("fresh"):
        # the first prompt of the new conversation, instead of the stage-change one
        decisions = ".pair/decisions.md" if pair else SINGLE_DECISIONS_FILE
        env["TASK_CONTINUE"] = FRESH_PROMPTS[rec["fresh"]].format(
            decisions=decisions, stage=record_stage(rec).upper())
    result = subprocess.run(argv, capture_output=True, text=True, env=env, timeout=120,
                            check=False)
    command = result.stdout.strip()
    if result.returncode != 0 or not command:
        raise PairError(f"{' '.join(argv)} failed: {(result.stderr or result.stdout).strip()}")
    return command.splitlines()[-1]


def cmd_stage_handoff(args: argparse.Namespace) -> int:
    """The detached half of `stage` and `fresh`: respawn the writer, then start the reviewer."""
    base = Path(args.base).resolve()
    time.sleep(STAGE_SETTLE_SECONDS)  # let the agent that ran the restart take its result
    pair = is_pair(base, args.single)
    rec = load_record(base, args.single)
    node = find_session(args.old)
    kind = rec["roles"]["writer"]["kind"] if pair else agent_kind(node.get("foreground"))
    if kind == "none":
        raise PairError(f"no agent runs in agterm session {args.old}")
    if pair:
        # fresh port and reviewer configs (both stage agents), as for a re-open
        init_pair(base, kind)
        if rec.get("fresh"):
            # the new conversations never get the old backlog: held FYIs and unread
            # messages go to read/ before any agent starts
            for role in ROLES:
                release_held(base, role)
                for path in unread(base, role):
                    mark_read(base, role, path)
    command = launcher_command(rec, args.single, kind, pair)
    sid = respawn(args.old, node.get("name") or base.name, str(base), command)
    if pair:
        launch(base, sid, True, args.wait_seconds)
    rec = load_record(base, args.single)
    if rec.pop("fresh", None):
        # the new conversations are recorded: later restarts resume them again
        save_record(base, args.single, rec)
    print(json.dumps({"stage": rec["stage"], "session": sid}))
    return 0


def init_pair(base: Path, writer: str) -> int:
    """Create or refresh .pair/ for a task; returns the opencode port."""
    root = pair_dir(base)
    for role in ROLES:
        (inbox(base, role) / "read").mkdir(parents=True, exist_ok=True)
    (root / "files").mkdir(parents=True, exist_ok=True)
    if (root / "config.json").is_file():
        # an existing config keeps its stage and session ids: a re-open reads them
        cfg = load_config(base)
        started = cfg["roles"]["writer"]["kind"]
        if started != writer:
            raise PairError(f"this task was started with a {started} writer; "
                            "re-open it with the same agent")
        if impl_reached(base):
            cfg["stage"] = "impl"
    else:
        reviewer = "opencode" if writer == "claude" else "claude"
        # a single session turned into a pair keeps its stage
        stage = single_record(base).get("stage")
        stage = stage if stage in STAGES else "plan"
        cfg = {"version": 1, "stage": stage,
               "roles": {"writer": {"kind": writer}, "reviewer": {"kind": reviewer}}}
        if reviewer == "opencode":
            cfg["roles"]["reviewer"]["agent"] = REVIEWER_AGENT
    cfg["pair_py"] = str(PAIR_PY)
    # launch looks for the opencode session created after this moment
    cfg["started_ms"] = int(time.time() * 1000)
    # a fresh port and no session: the previous opencode server is gone with its pane
    cfg["opencode"] = {"port": int(os.environ.get("PAIR_FREE_PORT") or free_port()),
                       "session": None}
    (root / "opencode-reviewer.json").write_text(
        json.dumps(reviewer_opencode_config(record_stage(cfg)), indent=2) + "\n")
    for role in ROLES:
        if cfg["roles"][role]["kind"] == "claude":
            # the agent starts with --settings <this file>: its hook forwards user messages
            claude_settings_file(base, role).write_text(
                json.dumps(claude_settings(base, role), indent=2) + "\n")
    save_config(base, cfg)
    for role in ROLES:
        waiter_pid_file(base, role).unlink(missing_ok=True)
    exclude_from_git(base)
    return cfg["opencode"]["port"]


def cmd_init(args: argparse.Namespace) -> int:
    print(init_pair(Path(args.base).resolve(), args.writer))
    return 0


SHELL_SPECIAL = set("'\"$`\\")
CATCH_UP = ("This task was re-opened: read PLAN.md, .pair/decisions.md (every pair "
            f"decision so far) and {USER_DECISIONS_READ} to catch up first; read "
            ".pair/inbox/*/read/ only for a topic they do not cover.")
# a reviewer restarted by `fresh`, per purpose: never the old messages
FRESH_ONLY_NEEDED = "Load any skill only when a step of your work needs it."
FRESH_CATCH_UPS = {
    "plan": ("The context of both agents was cleared on purpose for the plan discussion: "
             "read PLAN.md, the whole task context now, and never the old messages in "
             f".pair/inbox/*/read/. {FRESH_ONLY_NEEDED}"),
    "impl": ("The context of both agents was cleared on purpose after the user approved "
             "PLAN.md, and the writer implements it now: read PLAN.md, .pair/decisions.md, "
             f"{USER_DECISIONS_READ} and the git diff, and never the old messages in "
             f".pair/inbox/*/read/. {FRESH_ONLY_NEEDED}"),
    "finish": ("The context of both agents was cleared on purpose for task finish, and the "
               "writer runs it now: read PLAN.md, .pair/decisions.md, "
               f"{USER_DECISIONS_READ} and the git diff, and never the old messages in "
               f".pair/inbox/*/read/. {FRESH_ONLY_NEEDED}"),
    "reviews": ("The context of both agents was cleared on purpose for task reviews, and the "
                "task is on PLAN again: the writer walks the plan steps with the user. Read "
                f"PLAN.md, .pair/decisions.md, {USER_DECISIONS_READ} and the git diff, and "
                f"never the old messages in .pair/inbox/*/read/. {FRESH_ONLY_NEEDED}"),
    "continue": ("The context of both agents was cleared on purpose by task fresh: load no "
                 "skill and read nothing about the task now, never the old messages in "
                 ".pair/inbox/*/read/, and wait for the messages of the writer."),
}


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
        parts.append(f"Receive messages now with the Monitor tool (load it with ToolSearch "
                     f"when it is deferred) on python3 {py} wait --follow --base {base} "
                     f"--role {role}, timeout_ms 1800000, description pair messages. Each "
                     "batch arrives as one event. Start it again only when the monitor "
                     "expires or stops.")
    else:
        parts.append(f"Messages from the {peer} arrive by themselves as prompts that "
                     "start with Chat from.")
    parts.append("This is pair programming with a fast default: the writer decides a "
                 "clear case alone (simple, settled by the plan, a skill or a convention, "
                 "or done before, and 100% sure). A hard case (not clear what or how, or "
                 "not 100% sure) is decided by both agents BEFORE the code is typed: each "
                 "agent forms its own view before it sees the view of the other, then the "
                 "two views are merged. When in doubt, it is hard. The reviewer is ALWAYS "
                 "in code review: every landed change, the final check and every "
                 "code-review run. A user message that only runs a command, subcommand or "
                 "skill, such as task finish, task plan or run the tests, is not a "
                 "decision: run it at once and send the peer FYI: running the command. "
                 "See Decision weight in pair.md.")
    if uses_threads(cfg, role):
        parts.append("A clear user question or request: answer or do it at once, alone. "
                     "A hard opinion question gets your own answer at once, labelled "
                     "reviewer check pending, and is then checked by both agents; afterwards "
                     "tell the user only Reviewer agreed. or Changed after review: the new "
                     "answer. You are the DRIVER: for each hard decision and each hard user "
                     "question or request, write your own view to a file, then run a "
                     "background liaison subagent that settles it with the reviewer in a "
                     "pair thread (see Liaison in pair.md). Edit or answer it only with the "
                     "result it returns.")
    else:
        parts.append(f"A clear user question or request: answer or do it at once, alone. "
                     f"A hard opinion question gets your own answer at once, labelled "
                     f"{peer} check pending: send the {peer} DECISION: user asked: the "
                     f"question (the {peer} answers VIEW: user asked: with its own answer), "
                     "and give the user your answer right away. After the merge, "
                     f"tell the user only The {peer} agreed. or Changed after review: the "
                     "new answer; give both answers when you still disagree. A user "
                     f"message forwarded from the {peer} is context only: no VIEW, no ack.")
    if role == "writer":
        if not uses_threads(cfg, role):
            parts.append("You are the DRIVER: decide each clear case alone and type it. "
                         "For each hard decision, write your own view to a file, send the "
                         "reviewer a DECISION with the problem only, never your view, and "
                         "wait for its VIEW. Then send your VIEW and a MERGE that takes the "
                         "best of both, and edit only after the reviewer agrees.")
    else:
        parts.append("You are the NAVIGATOR and you are READ-ONLY: never edit, create or "
                     "delete files in the worktree. You review every change as it lands "
                     "(git diff) and send STOP: for a wrong one or for a hard decision taken "
                     "alone; a correct one gets no reply. You co-own every hard decision: "
                     "answer each DECISION first with your own VIEW, formed from the code "
                     "and the project conventions before you read the view of the writer. "
                     "Answer a MERGE with AGREE only after you name the strongest objection "
                     "you checked in the code and why it does not hold, or with COUNTER. "
                     "Start reading the ticket and the code now.")
        if kind == "opencode":
            parts.append("Threads in .pair/threads/ belong to their own reviewer subagents: "
                         "never read their files and never run thread send.")
    if reopen:
        parts.append(FRESH_CATCH_UPS[cfg["fresh"]] if cfg.get("fresh") else CATCH_UP)
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
    """Write the reviewer start script and return the short line typed to run it.

    The env and the agent flags live in the script, never in the typed line: a keystroke
    lost while the split shell starts must break the line, not silently drop
    OPENCODE_CONFIG and start the reviewer on the default agent and model.
    """
    root = pair_dir(base)
    brief_file = root / "reviewer-brief.txt"
    brief_file.write_text(brief(base, cfg, "reviewer", reopen=reopen), encoding="utf-8")
    brief_arg = f"\"$(cat {shlex.quote(str(brief_file))})\""
    if cfg["roles"]["reviewer"]["kind"] == "claude":
        settings = shlex.quote(str(claude_settings_file(base, "reviewer")))
        # the stage model, and --resume of the recorded conversation after a restart
        flags = stage_flags(base, cfg, "reviewer", pair=True)
        save_config(base, cfg)
        # after `fresh` the first prompt carries FRESH_MARK, so the router hooks skip it too
        first = (f"Pair mode start: {FRESH_MARK}. Follow the pair brief in your system "
                 "prompt now." if reopen and cfg.get("fresh") else
                 "Pair mode start: follow the pair brief in your system prompt now.")
        start = ("exec claude --dangerously-skip-permissions --disallowedTools Edit Write "
                 f"NotebookEdit --settings {settings} {flags} --append-system-prompt {brief_arg} "
                 f"\"{first}\"")
    else:
        # the stage model: a reviewer started before the rename comes back on `review`
        (root / "opencode-reviewer.json").write_text(
            json.dumps(reviewer_opencode_config(record_stage(cfg)), indent=2) + "\n")
        cfg["roles"]["reviewer"]["agent"] = agent = REVIEWER_AGENT
        save_config(base, cfg)
        # --prompt starts opencode straight in a session, never on its start screen
        start = (f"export OPENCODE_CONFIG={shlex.quote(str(root / 'opencode-reviewer.json'))}\n"
                 f"exec opencode --auto --agent {agent} --port {cfg['opencode']['port']} "
                 f"--prompt {brief_arg}")
    script = root / "reviewer-start.sh"
    script.write_text(f"#!/bin/sh\n{start}\n", encoding="utf-8")
    return f"sh {shlex.quote(str(script))}"


def new_opencode_session(cfg: dict, base: Path) -> str | None:
    """The session opencode runs for this base: newest one active since init.

    `updated`, not `created`: an opencode started with --session resumes an older
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


def launch(base: Path, sid: str, reopen: bool, wait_seconds: float) -> dict:
    cfg = load_config(base)
    cfg["agterm_session"] = sid
    save_config(base, cfg)
    start_reviewer(base, cfg, sid, reopen, wait_seconds)
    return cfg


def cmd_launch(args: argparse.Namespace) -> int:
    cfg = launch(resolve_base(args.base), args.session, args.reopen, args.wait_seconds)
    print(json.dumps({"session": args.session, "opencode_session": cfg["opencode"]["session"]}))
    return 0


def start_reviewer(base: Path, cfg: dict, sid: str, reopen: bool, wait_seconds: float) -> None:
    """Start the reviewer in the right pane, bind its opencode session, flush backlogs."""
    ctl("session", "split", "on", "--target", sid)
    wait_until(lambda: find_session(sid).get("splitForegroundShell"), "the split shell",
               wait_seconds)
    pane =("--stdin", "--target", sid, "--pane", "right")
    line = reviewer_command(base, cfg, reopen)
    kind = cfg["roles"]["reviewer"]["kind"]
    for attempt in range(1, LAUNCH_ATTEMPTS + 1):
        ctl("session", "type", *pane, input_text=line)
        # Return as its own event: a TUI can read text+Enter in one burst as a paste
        ctl("session", "type", *pane, input_text="\n")
        try:
            wait_until(lambda: runs(find_session(sid).get("splitForeground"), kind),
                       f"the {kind} reviewer", wait_seconds)
            break
        except PairError:
            node = find_session(sid)
            # retype only into an idle shell: a garbled line failed and left the prompt
            if (attempt == LAUNCH_ATTEMPTS or node.get("splitForeground")
                    or not node.get("splitForegroundShell")):
                raise

    cfg["opencode"]["session"] = wait_until(lambda: new_opencode_session(cfg, base),
                                            "the opencode session", wait_seconds * 2)
    if cfg["roles"]["writer"]["kind"] == "opencode":
        # `stage` and a re-open resume the writer in this session
        cfg["opencode_session"] = cfg["opencode"]["session"]
    save_config(base, cfg)
    if kind == "opencode":
        agents = [a.get("name") for a in http_json("GET", opencode_url(cfg, "/agent")) or []]
        if REVIEWER_AGENT not in agents:
            raise PairError(f"the opencode reviewer started without the {REVIEWER_AGENT} agent "
                            "(OPENCODE_CONFIG not applied); it runs the default agent and model")

    for role in ROLES:
        if cfg["roles"][role]["kind"] == "opencode":
            # messages queued while no opencode session existed (a failed send, a re-open)
            flush_opencode(base, cfg, role)


# a reviewer gets this long to exit on SIGTERM before it gets SIGKILL
KILL_GRACE_SECONDS = 5


def process_command(pid: int) -> str:
    return subprocess.run(["ps", "-o", "command=", "-p", str(pid)], capture_output=True,
                          text=True, check=False).stdout.strip()


def is_reviewer_process(base: Path, cfg: dict, command: str) -> bool:
    """Only the reviewer runs this command line: a recycled pid or a neighbour does not."""
    if cfg["roles"]["reviewer"]["kind"] == "opencode":
        # either stage agent: `stage` rewrites roles.reviewer.agent before the restart
        return ("opencode" in command and f"--port {cfg['opencode']['port']}" in command
                and any(f"--agent {agent}" in command
                        for agent in (REVIEWER_AGENT, *LEGACY_REVIEWER_AGENTS)))
    return "claude" in command and f"--settings {claude_settings_file(base, 'reviewer')}" in command


def reviewer_pids(base: Path, cfg: dict) -> list[int]:
    """The live reviewer processes; refuses a port held by anything but the reviewer."""
    if cfg["roles"]["reviewer"]["kind"] == "opencode":
        out = subprocess.run(["lsof", "-ti", f"tcp:{cfg['opencode']['port']}", "-sTCP:LISTEN"],
                             capture_output=True, text=True, check=False).stdout
        pids = sorted({int(x) for x in out.split() if x.isdigit()})
        for pid in pids:
            command = process_command(pid)
            if command and not is_reviewer_process(base, cfg, command):
                raise PairError(f"port {cfg['opencode']['port']} is held by pid {pid}, which is "
                                f"not the reviewer ({command[:120]}); not killing it")
        return pids
    out = subprocess.run(["pgrep", "-f", str(claude_settings_file(base, "reviewer"))],
                         capture_output=True, text=True, check=False).stdout
    # pgrep -f matches any command line with the path: keep only the reviewer itself
    return [pid for pid in sorted({int(x) for x in out.split() if x.isdigit()})
            if pid != os.getpid() and is_reviewer_process(base, cfg, process_command(pid))]


def reviewer_alive(base: Path, cfg: dict) -> bool:
    if cfg["roles"]["reviewer"]["kind"] == "opencode":
        return opencode_up(cfg)
    return bool(reviewer_pids(base, cfg))


def kill_reviewer(base: Path, cfg: dict) -> list[int]:
    pids = reviewer_pids(base, cfg)
    for pid in pids:
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    deadline = time.monotonic() + KILL_GRACE_SECONDS
    while any(pid_alive(p) for p in pids) and time.monotonic() < deadline:
        time.sleep(0.2)
    for pid in pids:
        # check again: the pid may have been reused after the reviewer exited
        if pid_alive(pid) and is_reviewer_process(base, cfg, process_command(pid)):
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
    return pids


def close_threads_of(base: Path, session: str | None) -> list[str]:
    """Close the threads of a dead reviewer: free their files, refuse late replies."""
    now = int(time.time() * 1000)
    closed = []
    with threads_lock(base):
        for path, meta in all_threads(base):
            if meta["status"] != "closed" and meta.get("pair_session") == session:
                meta.update(status="closed", closed_ms=now, restarted_ms=now)
                save_meta(path, meta)
                closed.append(meta["id"])
    return closed


def cmd_restart_reviewer(args: argparse.Namespace) -> int:
    """Kill a dead or stuck reviewer and start it again in its pane with the catch-up brief."""
    base = resolve_base(args.base)
    cfg = load_config(base)
    if args.check:
        print(json.dumps({"alive": reviewer_alive(base, cfg)}))
        return 0
    seen = cfg.get("reviewer_generation", 0)
    with open(pair_dir(base) / "restart.lock", "w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        cfg = load_config(base)
        if cfg.get("reviewer_generation", 0) != seen:
            # another caller restarted it while this one waited for the lock
            print(json.dumps({"restarted": False, "opencode_session": cfg["opencode"]["session"],
                              "reason": "already restarted by another caller"}))
            return 0
        sid = cfg.get("agterm_session")
        if not sid:
            raise PairError("no agterm session in .pair/config.json (pair.py launch sets it)")
        kind = cfg["roles"]["reviewer"]["kind"]
        killed = kill_reviewer(base, cfg)
        closed = []
        if kind == "opencode":
            closed = close_threads_of(base, cfg["opencode"].get("session"))
            # a fresh server: new port, and launch binds the session it creates from now on
            cfg["opencode"] = {"port": int(os.environ.get("PAIR_FREE_PORT") or free_port()),
                               "session": None}
            cfg["started_ms"] = int(time.time() * 1000)
        else:
            # the OpenCode writer keeps its server and session; only the old waiter goes
            retire_waiter(base, "reviewer")
        save_config(base, cfg)
        try:
            wait_until(lambda: not runs(find_session(sid).get("splitForeground"), kind),
                       "the reviewer pane to return to its shell", args.wait_seconds)
        except PairError:
            pass  # start_reviewer retypes the start line into a pane that lost it
        start_reviewer(base, cfg, sid, True, args.wait_seconds)
        cfg["reviewer_generation"] = seen + 1
        save_config(base, cfg)
    print(json.dumps({"restarted": True, "killed": killed, "closed_threads": closed,
                      "opencode_session": cfg["opencode"]["session"]}))
    return 0


THREAD_LABELS = {"writer": "Chat from writer [{id}]:", "reviewer": "Chat from reviewer [{id}]:"}
THREAD_STATUSES = ("open", "agreed", "disputed", "closed")
THREAD_BRIEF = (
    "PAIR THREAD {id}: you are the REVIEWER of this one decision only, in a subagent "
    "session of the pair reviewer. Topic: {topic}. Files: {files}. Read {thread_md} and "
    "follow it: you are READ-ONLY, and you check the proposal against the "
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
    agent = cfg["roles"]["reviewer"].get("agent", REVIEWER_AGENT)
    for msg in sorted(box.glob("*.md")):
        text = msg.read_text(encoding="utf-8").rstrip("\n")
        key = None
        if not meta.get("briefed"):
            key = secrets.token_hex(8)
            text = THREAD_BRIEF.format(id=meta["id"], topic=meta["topic"], key=key,
                                       files=", ".join(meta["files"]), base=base,
                                       thread_md=SKILL_DIR / "pair-thread.md",
                                       py=PAIR_PY) + "\n\n" + text
        prompt_session(cfg, meta["reviewer_session"], text, agent)
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
        # a thread nobody answers only holds its files and its liaison: leave none
        try:
            http_json("DELETE", opencode_url(cfg, f"/session/{session}"))
        except PairError:
            pass
        shutil.rmtree(path, ignore_errors=True)
        raise PairError(f"thread not opened: its proposal did not reach the reviewer "
                        f"subagent ({exc})") from exc
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
    # with threads the main reviewer sends the writer nothing but FYIs, so no main-channel
    # message would ever release them; the writer's wait delivers them as one batch
    release_held(base, "writer")
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
        if meta["status"] == "open":
            # a second liaison would race the running one for the same replies
            raise PairError(f"thread {args.id} is open: its liaison still runs. Send it the "
                            "follow-up with SendMessage instead", code=5)
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
    unread = sorted((path / "inbox" / "writer").glob("*.md"))
    if unread:
        raise PairError(f"thread {args.id} has {len(unread)} unread reviewer repl"
                        f"{'y' if len(unread) == 1 else 'ies'}: read them first with "
                        f"python3 {PAIR_PY} thread wait --base {base} --id {args.id}, "
                        "answer them, then close", code=8)
    if args.status in ("agreed", "disputed"):
        if not args.stdin:
            raise PairError(f"--status {args.status} needs the decision on --stdin")
        decision = read_body(args)
        (path / "decision.md").write_text(decision + "\n", encoding="utf-8")
        append_decision(base, f"{args.id} {args.status}: {meta['topic']} "
                              f"[{', '.join(meta['files'])}]", decision)
    meta["status"] = args.status
    if args.status == "closed":
        meta["closed_ms"] = int(time.time() * 1000)
    save_meta(path, meta)
    print(json.dumps({"id": args.id, "status": args.status}))
    return 0


def decisions_file(base: Path) -> Path:
    return pair_dir(base) / "decisions.md"


def append_decision(base: Path, title: str, text: str) -> None:
    """One log of every pair decision, read on catch-up instead of the whole message log.

    On PLAN the decision is also shown in PLAN.md under the same heading; reviewd shows it
    once (references/reviewd.md).
    """
    with open(decisions_file(base), "a", encoding="utf-8") as log:
        log.write(f"## {title}\n\n{text}\n\n")
    if not impl_reached(base):
        log_on_plan(base, "decision", f"### {title}\n\n{text}", mirror=True)


def section_bounds(lines: list[str], heading: str) -> tuple[int, int] | None:
    """The line range of a `## ` section (heading included), outside code fences."""
    start, fenced = None, False
    for i, line in enumerate(lines):
        if line.startswith("```"):
            fenced = not fenced
        if fenced:
            continue
        if start is None and line.rstrip("\n") == heading:
            start = i
        elif start is not None and line.startswith("## "):
            return start, i
    return (start, len(lines)) if start is not None else None


def insert_in_section(text: str, heading: str, entry: str) -> str:
    """Append the entry at the end of the section; a missing section is added at the end."""
    block = entry.strip("\n") + "\n\n"
    lines = text.splitlines(keepends=True)
    bounds = section_bounds(lines, heading)
    if bounds is None:
        head = text.rstrip("\n")
        return f"{head}\n\n{heading}\n\n{block}" if head else f"{heading}\n\n{block}"
    _, end = bounds
    head = "".join(lines[:end]).rstrip("\n")
    return f"{head}\n\n{block}{''.join(lines[end:])}"


def insert_before_heading(text: str, heading: str, entry: str) -> str:
    """Put the entry above `heading`, or at the end of the file without it."""
    block = entry.strip("\n") + "\n\n"
    lines = text.splitlines(keepends=True)
    bounds = section_bounds(lines, heading)
    if bounds is None:
        head = text.rstrip("\n")
        return f"{head}\n\n{block}" if head else block
    start, _ = bounds
    head = "".join(lines[:start]).rstrip("\n")
    return f"{head}\n\n{block}{''.join(lines[start:])}" if head else block + "".join(lines[start:])


def load_pending(base: Path) -> list[dict]:
    path = base / PLAN_LOG_PENDING
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return []
    except json.JSONDecodeError as exc:
        raise PairError(f"{path} is not JSON: {exc}") from exc


def save_pending(base: Path, pending: list[dict]) -> None:
    path = base / PLAN_LOG_PENDING
    if not pending:
        path.unlink(missing_ok=True)
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(json.dumps(pending, indent=1, ensure_ascii=False) + "\n", encoding="utf-8")
    tmp.replace(path)


def plan_gate_open(base: Path) -> bool:
    """The plannotator slot of the base shows PLAN.md (the gate or a plain annotate)."""
    result = subprocess.run(["bash", str(PAIR_PY.parent / "plannotator-session.sh"), "--status"],
                            cwd=base, capture_output=True, text=True, check=False)
    status = result.stdout.strip()
    return status.startswith("RUNNING:") and "PLAN.md" in status


def flush_plan_log(base: Path) -> tuple[int, int]:
    """Write the held entries into PLAN.md when it can take them: (written, still held)."""
    pending = load_pending(base)
    plan = base / "PLAN.md"
    if not pending or not plan.is_file() or plan_gate_open(base):
        return 0, len(pending)
    text = plan.read_text(encoding="utf-8")
    for item in pending:
        text = insert_in_section(text, PLAN_LOG_SECTIONS[item["kind"]], item["entry"])
    plan.write_text(text, encoding="utf-8")
    save_pending(base, [])
    return len(pending), 0


def log_on_plan(base: Path, kind: str, entry: str, mirror: bool = False) -> str:
    """Show a PLAN-stage entry in PLAN.md, or hold it until PLAN.md can take it.

    `mirror`: the entry also lives in .pair/decisions.md, so it is dropped when the plan
    closes with it still held.
    """
    pending = load_pending(base)
    pending.append({"kind": kind, "entry": entry, "mirror": mirror})
    save_pending(base, pending)
    written, _ = flush_plan_log(base)
    return "PLAN.md" if written else PLAN_LOG_PENDING


def impl_log_file(base: Path, kind: str) -> Path:
    return base / (QA_FILE if kind == "qa" else SINGLE_DECISIONS_FILE)


def append_impl_log(base: Path, kind: str, entry: str) -> Path:
    path = impl_log_file(base, kind)
    text = path.read_text(encoding="utf-8") if path.exists() else ""
    text = insert_before_heading(text, REVIEWS_HEADING, entry)
    path.write_text(text, encoding="utf-8")
    exclude_from_git(base, f"/{path.name}")
    return path


def release_plan_log(base: Path) -> int:
    """The plan closes with entries still held: they go to the IMPL logs, never into the
    approved PLAN.md. A held pair decision is dropped: .pair/decisions.md already has it."""
    pending = load_pending(base)
    for item in pending:
        if not item.get("mirror"):
            append_impl_log(base, item["kind"], item["entry"])
    save_pending(base, [])
    return len(pending)


def next_log_id(base: Path, kind: str) -> int:
    """The next Q<n> / D<n> over PLAN.md, the held entries and the IMPL log."""
    texts = [item["entry"] for item in load_pending(base) if item["kind"] == kind]
    for path in (base / "PLAN.md", impl_log_file(base, kind)):
        if path.is_file():
            texts.append(path.read_text(encoding="utf-8"))
    ids = [int(m) for t in texts for m in PLAN_LOG_IDS[kind].findall(t)]
    return max(ids, default=0) + 1


def log_entry(base: Path, kind: str, heading: str, body: str) -> str:
    """Log one Q&A entry or one model decision of a single session, by the task stage."""
    stage = "IMPL" if impl_reached(base) else "PLAN"
    prefix = "Q" if kind == "qa" else "D"
    entry = (f"### {prefix}{next_log_id(base, kind)} — {heading.format(stage=stage)}"
             f"\n\n{body}")
    if stage == "IMPL":
        return str(append_impl_log(base, kind, entry))
    return str(base / log_on_plan(base, kind, entry))


def cmd_qa(args: argparse.Namespace) -> int:
    """Log one question and its answers (SKILL.md, Q&A log)."""
    base = Path(args.base).resolve()
    body = sys.stdin.read().strip()
    if not body:
        raise PairError("empty entry")
    date = time.strftime("%Y-%m-%d")
    logged = log_entry(base, "qa", f"{args.topic} ({date}, {{stage}})", body)
    print(json.dumps({"logged": logged}))
    return 0


def cmd_model_decided(args: argparse.Namespace) -> int:
    """Log one model decision of a single session (SKILL.md, Model decisions log)."""
    base = Path(args.base).resolve()
    body = sys.stdin.read().strip()
    if not body:
        raise PairError("empty entry")
    date = time.strftime("%Y-%m-%d")
    logged = log_entry(base, "decision", f"{args.topic} ({date}, {{stage}}, {args.by})", body)
    print(json.dumps({"logged": logged}))
    return 0


def cmd_plan_log_flush(args: argparse.Namespace) -> int:
    """Write the held PLAN-stage entries into PLAN.md before the plan gate opens again."""
    written, held = flush_plan_log(Path(args.base).resolve())
    print(json.dumps({"written": written, "held": held}))
    return 0


def cmd_decided(args: argparse.Namespace) -> int:
    """Log a decision settled on the main channel (no thread records it there)."""
    base = resolve_base(args.base)
    append_decision(base, f"main: {args.topic}", read_body(args))
    print(json.dumps({"logged": str(decisions_file(base))}))
    return 0


def cmd_user_decided(args: argparse.Namespace) -> int:
    """Log a user decision made on IMPL, in either mode: a single session has no .pair/."""
    base = Path(args.base).resolve()
    words = sys.stdin.read().strip()
    if not words:
        raise PairError("empty decision")
    path = base / USER_DECISIONS_FILE
    with open(path, "a", encoding="utf-8") as log:
        log.write(f"### {time.strftime('%Y-%m-%d')}\n\n{words}\n\n")
    exclude_from_git(base, f"/{USER_DECISIONS_FILE}")
    print(json.dumps({"logged": str(path)}))
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
            "guard": cmd_guard, "edit-guard": cmd_edit_guard, "idle": cmd_idle,
            "restart-reviewer": cmd_restart_reviewer, "decided": cmd_decided,
            "user-decided": cmd_user_decided, "plan-open": cmd_plan_open,
            "qa": cmd_qa, "model-decided": cmd_model_decided,
            "plan-log-flush": cmd_plan_log_flush,
            "thread": cmd_thread, "threads": cmd_threads, "stage": cmd_stage, "fresh": cmd_fresh,
            "stage-handoff": cmd_stage_handoff, "claude-args": cmd_claude_args,
            "opencode-args": cmd_opencode_args}


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

    decided = sub.add_parser("decided", help="log a decision settled on the main channel")
    decided.add_argument("--base")
    decided.add_argument("--topic", required=True)
    decided_source = decided.add_mutually_exclusive_group(required=True)
    decided_source.add_argument("--stdin", action="store_true")
    decided_source.add_argument("--file")

    user_decided = sub.add_parser("user-decided",
                                  help="log a user decision made after the plan approval")
    user_decided.add_argument("--base", required=True)
    user_decided.add_argument("--stdin", action="store_true", required=True)

    plan_open = sub.add_parser("plan-open", help="exit 1 once the plan is approved and closed")
    plan_open.add_argument("--base", required=True)

    qa = sub.add_parser("qa", help="log one question and its answers: PLAN.md on PLAN, "
                                   f"{QA_FILE} on IMPL")
    qa.add_argument("--base", required=True)
    qa.add_argument("--topic", required=True)
    qa.add_argument("--stdin", action="store_true", required=True)

    model_decided = sub.add_parser("model-decided",
                                   help="log a model decision of a single session: PLAN.md "
                                        f"on PLAN, {SINGLE_DECISIONS_FILE} on IMPL")
    model_decided.add_argument("--base", required=True)
    model_decided.add_argument("--topic", required=True)
    model_decided.add_argument("--by", choices=("main", "builder", "fixer"), required=True)
    model_decided.add_argument("--stdin", action="store_true", required=True)

    plan_log_flush = sub.add_parser("plan-log-flush",
                                    help="write the held PLAN-stage entries into PLAN.md")
    plan_log_flush.add_argument("--base", required=True)

    wait = sub.add_parser("wait", help="block until a message arrives (Claude side)")
    wait.add_argument("--base")
    wait.add_argument("--role", choices=ROLES, required=True)
    wait.add_argument("--timeout", type=float, default=0, help="seconds; 0 waits forever")
    wait.add_argument("--follow", action="store_true",
                      help="keep listening after a batch (run it under the Monitor tool)")

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

    guard = sub.add_parser("guard", help="PostToolUse/Stop hook: catch a waiter nobody re-armed")
    guard.add_argument("--base")
    guard.add_argument("--role", choices=ROLES, required=True)

    edit_guard = sub.add_parser("edit-guard",
                                help="PreToolUse hook: refuse an edit to a file of an open "
                                     "thread, and to PLAN.md on IMPL")
    edit_guard.add_argument("--base")
    edit_guard.add_argument("--role", choices=ROLES, required=True)

    idle = sub.add_parser("idle", help="no work left: stop the waiter until new work arrives")
    idle.add_argument("--base")
    idle.add_argument("--role", choices=ROLES, required=True)

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

    restart = sub.add_parser("restart-reviewer",
                             help="kill a dead or stuck reviewer and start it again")
    restart.add_argument("--base")
    restart.add_argument("--check", action="store_true",
                         help="only print whether the reviewer is alive")
    restart.add_argument("--wait-seconds", type=float, default=30)

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

    stage = sub.add_parser("stage", help="switch the task stage and restart the sessions on "
                                         "its models; PLAN to IMPL is one-way (only fresh "
                                         "--then reviews reopens the plan)")
    stage.add_argument("stage", choices=STAGES)
    stage.add_argument("--session-id", action="append", metavar="ROLE=ID",
                       help="the conversation to resume when none is recorded")

    fresh = sub.add_parser("fresh", help="restart the writer and the reviewer in new "
                                         "conversations on the same task")
    fresh.add_argument("--then", choices=tuple(FRESH_STAGES),
                       help="the first prompt and its stage: plan or reviews (PLAN), impl or "
                            "finish (IMPL); reviews also reopens an approved plan; omitted: "
                            "the recorded stage and a status report")
    for p in (stage, fresh):
        p.add_argument("--foreground", action="store_true",
                       help="restart from this process (only from outside the task session)")
        p.add_argument("--wait-seconds", type=float, default=30)

    handoff = sub.add_parser("stage-handoff",
                             help="internal: the detached restart of stage and fresh")
    handoff.add_argument("--old", required=True, help="the agterm session to replace")
    handoff.add_argument("--wait-seconds", type=float, default=30)

    claude_args = sub.add_parser("claude-args", help="the claude flags of the task stage")
    claude_args.add_argument("--role", choices=ROLES, default="writer")
    claude_args.add_argument("--resume-id", help="the conversation of the role, given by the "
                                                 "session that runs the launcher")

    opencode_args = sub.add_parser("opencode-args",
                                   help="the opencode writer flags of the task stage")
    opencode_args.add_argument("--resume", action="store_true",
                               help="no session recorded: take the newest of the base")

    for p in (stage, fresh, handoff, claude_args, opencode_args):
        p.add_argument("--base", required=True)
        p.add_argument("--single", action="store_true",
                       help="the record of a single session (.task-stage), even beside a pair")
    for p in (stage, fresh, claude_args, opencode_args):
        p.add_argument("--launcher", help="the project new-task.sh, recorded for stage")
        p.add_argument("--branch", help="the task branch, with --launcher")
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
