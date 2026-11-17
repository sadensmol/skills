"""Tests for pair.py. Run: python3 -m unittest -v skills/task/scripts/test_pair.py"""

import http.server
import json
import os
import re
import shutil
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import time
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
elif sys.argv[1:3] == ["session", "new"]:
    print("11111111-2222-3333-4444-555555555555")
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


def make_base(root, writer="claude", port=1, session="ses_test", agterm="SID", stage="impl"):
    base = Path(root)
    reviewer = "opencode" if writer == "claude" else "claude"
    roles = {"writer": {"kind": writer}, "reviewer": {"kind": reviewer}}
    if reviewer == "opencode":
        roles["reviewer"]["agent"] = "review"
    cfg = {"version": 1, "pair_py": str(PAIR), "roles": roles,
           "opencode": {"port": port, "session": session}, "agterm_session": agterm}
    if stage:
        cfg["stage"] = stage
    for role in ("writer", "reviewer"):
        (base / ".pair" / "inbox" / role / "read").mkdir(parents=True, exist_ok=True)
    (base / ".pair" / "config.json").write_text(json.dumps(cfg))
    return base


class StubOpencode:
    """A local stand-in for the OpenCode server that records every request."""

    def __init__(self):
        self.requests = []
        self.sessions = []
        self.agents = [{"name": "main"}, {"name": "review"}]
        self.created = 0
        self.fail_prompts = False
        # answer 204 but store nothing, like a server without the prompt's agent
        self.drop_prompts = False
        self.messages = {}
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
                match = re.fullmatch(r"/session/([^/]+)/message(\?limit=(\d+))?", self.path)
                if match:
                    stored = stub.messages.get(match.group(1), [])
                    limit = int(match.group(3) or 0)
                    self._reply(200, stored[-limit:] if limit else stored)
                    return
                replies = {"/session": stub.sessions, "/agent": stub.agents}
                self._reply(200, replies.get(self.path, {}))

            def do_DELETE(self):
                stub.requests.append(("DELETE", self.path, None))
                self._reply(200, True)

            def do_POST(self):
                size = int(self.headers.get("content-length") or 0)
                body = json.loads(self.rfile.read(size)) if size else None
                stub.requests.append(("POST", self.path, body))
                if self.path == "/session":
                    stub.created += 1
                    self._reply(200, {"id": "ses_new" if stub.created == 1
                                      else f"ses_new{stub.created}"})
                elif self.path.endswith("/prompt_async"):
                    if not (stub.fail_prompts or stub.drop_prompts):
                        session = self.path.split("/")[2]
                        stub.messages.setdefault(session, []).append(
                            {"info": {"role": "user"}, "parts": body["parts"]})
                    self._reply(500 if stub.fail_prompts else 204, None)
                else:
                    self._reply(200, True)

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self.server.server_address[1]
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def prompts(self):
        return [body for method, path, body in self.requests if path.endswith("/prompt_async")]

    def calls(self):
        """The requests without the reads that confirm a prompt was stored."""
        return [r for r in self.requests if not (r[0] == "GET" and "/message" in r[1])]

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
                    "FAKE_AGTERM_TREE": str(self.tree),
                    # claude transcripts (--resume or --session-id) live here, not in ~/.claude
                    "CLAUDE_CONFIG_DIR": str(self.root / "claude"),
                    "PAIR_STAGE_SETTLE_SECONDS": "0"}
        self.base_dir = self.root / "wt"
        self.base_dir.mkdir()

    def tearDown(self):
        self.tmp.cleanup()

    def run_pair(self, *args, stdin=None, cwd=None):
        env = dict(os.environ)
        # the tests may run inside a Claude Code session or an OPENCODE=1 shell
        env.pop("CLAUDE_CODE_SESSION_ID", None)
        env.pop("OPENCODE", None)
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
        self.assertEqual(stub.calls(), [
            ("POST", "/tui/select-session", {"sessionID": "ses_test"}),
            ("POST", "/session/ses_test/prompt_async", {
                "parts": [{"type": "text", "text": "Chat from writer: step 1 ready"}],
                "agent": "review"})])
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


    def test_a_prompt_opencode_did_not_store_fails_the_send_and_stays_queued(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        self.env["PAIR_CONFIRM_SECONDS"] = "0.5"
        stub.drop_prompts = True
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin",
                          stdin="ANSWER: step 1 ready")
        self.assertEqual(r.returncode, 1)
        self.assertIn("did not take the prompt", r.stderr)
        self.assertEqual(len(list((base / ".pair/inbox/reviewer").glob("*.md"))), 1)
        stub.drop_prompts = False
        r = self.run_pair("flush", "--base", str(base), "--to", "reviewer")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual([m["parts"][0]["text"] for m in stub.messages["ses_test"]],
                         ["Chat from writer: ANSWER: step 1 ready"])
        self.assertEqual(list((base / ".pair/inbox/reviewer").glob("*.md")), [])

    def test_a_writer_decision_on_the_main_channel_is_refused_with_an_opencode_reviewer(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        for body in ("DECISION: retry policy", "VIEW: .pair/files/v.md", "MERGE: take both"):
            with self.subTest(body=body):
                r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin",
                                  stdin=body)
                self.assertEqual(r.returncode, 7)
                self.assertIn("liaison thread", r.stderr)
        self.assertEqual(list((base / ".pair/inbox/reviewer").glob("*.md")), [])
        self.assertEqual(stub.prompts(), [])
        for body in ("FYI: running task finish", ".pair/files/cr-quality-1.md"):
            r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin",
                              stdin=body)
            self.assertEqual(r.returncode, 0, r.stderr)

    def test_decisions_stay_on_the_main_channel_without_threads(self):
        base = make_base(self.base_dir, writer="opencode")
        (base / ".pair" / "waiter-reviewer.pid").write_text(f"{os.getpid()}\n")
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin",
                          stdin="DECISION: retry policy")
        self.assertEqual(r.returncode, 0, r.stderr)


class ClearRecoveryTests(PairTestCase):
    """A /clear keeps the Claude process, its old waiter and a drifted OpenCode pane."""

    def start_waiter(self, base):
        env = dict(os.environ)
        env.update(self.env)
        proc = subprocess.Popen([sys.executable, str(PAIR), "wait", "--base", str(base),
                                 "--role", "writer", "--timeout", "30"],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=env)
        self.addCleanup(lambda: proc.poll() is None and proc.kill())
        pid_file = base / ".pair" / "waiter-writer.pid"
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if pid_file.exists() and pid_file.read_text().strip() == str(proc.pid):
                return proc
            time.sleep(0.05)
        self.fail("the first waiter never wrote its pid file")

    def test_new_wait_retires_the_old_waiter_which_says_not_to_restart(self):
        base = make_base(self.base_dir, writer="claude")
        old = self.start_waiter(base)
        (base / ".pair/inbox/writer/01-reviewer.md").write_text("Chat from reviewer: hi\n")
        r = self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "5")
        self.assertIn("Chat from reviewer: hi", r.stdout)
        out, _ = old.communicate(timeout=10)
        self.assertEqual(old.returncode, 0)
        self.assertIn("SUPERSEDED", out)
        self.assertNotIn("Chat from reviewer", out)

    def test_wait_leaves_a_recycled_pid_alone(self):
        base = make_base(self.base_dir, writer="claude")
        other = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(30)"])
        self.addCleanup(other.kill)
        (base / ".pair" / "waiter-writer.pid").write_text(f"{other.pid}\n")
        self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "0.2")
        self.assertIsNone(other.poll())

    def test_session_start_retires_the_waiter_refocuses_the_pane_and_rearms(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        old = self.start_waiter(base)
        r = self.run_pair("session-start", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"hook_event_name": "SessionStart", "source": "clear"}))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("PAIR MODE resumed after /clear", r.stdout)
        self.assertIn(f"wait --follow --base {base.resolve()} --role writer", r.stdout)
        self.assertIn("Monitor", r.stdout)
        self.assertIn("put the reviewer pane back on the pair session", r.stdout)
        old.communicate(timeout=10)
        self.assertEqual(old.returncode, 0)
        self.assertIn(("POST", "/tui/select-session", {"sessionID": "ses_test"}), stub.requests)

    def test_session_start_outside_pair_mode_is_silent(self):
        r = self.run_pair("session-start", "--role", "writer", cwd=str(self.base_dir))
        self.assertEqual((r.returncode, r.stdout), (0, ""))


class FollowWaitTests(PairTestCase):
    """wait --follow runs under the Monitor tool: one event per batch, no re-arm."""

    def start_follow(self, base):
        env = dict(os.environ)
        env.update(self.env)
        proc = subprocess.Popen([sys.executable, str(PAIR), "wait", "--follow", "--base",
                                 str(base), "--role", "writer", "--timeout", "30"],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=env)
        self.addCleanup(lambda: proc.poll() is None and proc.kill())
        lines = []
        threading.Thread(target=lambda: lines.extend(iter(proc.stdout.readline, "")),
                         daemon=True).start()
        pid_file = base / ".pair" / "waiter-writer.pid"
        deadline = time.monotonic() + 10
        while not (pid_file.exists() and pid_file.read_text().strip() == str(proc.pid)):
            self.assertLess(time.monotonic(), deadline, "the follow waiter never wrote its pid")
            time.sleep(0.05)
        return proc, lines

    def wait_for_line(self, lines, text):
        deadline = time.monotonic() + 10
        while not any(text in line for line in lines):
            self.assertLess(time.monotonic(), deadline, f"no line with {text!r} in {lines}")
            time.sleep(0.05)

    def send(self, base, body):
        return self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin=body)

    def test_follow_wait_delivers_every_batch_and_keeps_listening(self):
        base = make_base(self.base_dir, writer="claude")
        proc, lines = self.start_follow(base)
        self.assertTrue(json.loads(self.send(base, "one").stdout)["delivered"])
        self.wait_for_line(lines, "Chat from reviewer: one")
        self.assertIsNone(proc.poll())
        r = self.send(base, "two")
        self.assertEqual(json.loads(r.stdout), {"sent": json.loads(r.stdout)["sent"],
                                                "delivered": True})
        self.wait_for_line(lines, "Chat from reviewer: two")
        self.assertIsNone(proc.poll())
        self.assertFalse(any("Start the wait again" in line for line in lines))
        self.assertEqual(list((base / ".pair/inbox/writer").glob("*.md")), [])

    def test_killed_follow_waiter_exits_silently_and_frees_its_pid_file(self):
        base = make_base(self.base_dir, writer="claude")
        proc, lines = self.start_follow(base)
        proc.terminate()
        proc.wait(timeout=10)
        self.assertEqual(proc.returncode, 0)
        time.sleep(0.2)
        self.assertNotIn("SUPERSEDED", "".join(lines))
        self.assertFalse((base / ".pair/waiter-writer.pid").exists())

    def test_a_new_follow_waiter_supersedes_the_old_one(self):
        base = make_base(self.base_dir, writer="claude")
        old, old_lines = self.start_follow(base)
        new, new_lines = self.start_follow(base)
        old.wait(timeout=10)
        time.sleep(0.2)
        self.assertIn("SUPERSEDED", "".join(old_lines))
        self.assertIsNone(new.poll())
        self.send(base, "after")
        self.wait_for_line(new_lines, "Chat from reviewer: after")


class UserPromptHookTests(PairTestCase):
    def hook(self, base, role, prompt):
        return self.run_pair("user-prompt", "--base", str(base), "--role", role,
                             stdin=json.dumps({"prompt": prompt, "hook_event_name": "UserPromptSubmit"}))

    def test_writer_prompt_is_not_forwarded_to_the_reviewer_and_reminds_the_writer(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        r = self.hook(base, "writer", "why not subscribe in the constructor? you think otherwise?")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(stub.prompts(), [])
        self.assertIn("PAIR MODE: pair.py does not forward user messages to the reviewer", r.stdout)
        self.assertIn("liaison", r.stdout)
        self.assertNotIn("{", r.stdout)
        self.assertLess(len(r.stdout), 700)

    def test_reminder_without_threads_keeps_the_view_rule_and_is_short(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="opencode", port=stub.port)
        (base / ".pair" / "waiter-reviewer.pid").write_text(f"{os.getpid()}\n")
        r = self.hook(base, "reviewer", "is this right?")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("DECISION: user asked:", r.stdout)
        self.assertIn("FYI: running", r.stdout)
        self.assertLess(len(r.stdout), 650)

    def test_reviewer_prompt_is_queued_for_the_claude_writer(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        r = self.hook(base, "reviewer", "is this right?")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "5")
        self.assertIn("The user said to the reviewer (context only, never send an ack: a "
                      "STOP: only when its answer or change is wrong): is this right?", r.stdout)

    def test_long_prompt_goes_through_a_file(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        prompt = "x" * 5000
        self.assertEqual(self.hook(base, "reviewer", prompt).returncode, 0)
        [msg] = list((base / ".pair/inbox/writer").glob("*.md"))
        text = msg.read_text()
        self.assertIn("long message, read it in", text)
        path = Path(text.split("read it in ", 1)[1].rstrip(")\n"))
        self.assertEqual(path.read_text().strip(), prompt)

    def test_secrets_in_the_prompt_are_redacted_before_forwarding(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        token = "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7"  # pragma: allowlist secret
        prompt = f"this is a token for production: {token}\nAPI_KEY=hunter2secret path a/b/{token}x"
        self.assertEqual(self.hook(base, "reviewer", prompt).returncode, 0)
        [msg] = list((base / ".pair/inbox/writer").glob("*.md"))
        text = msg.read_text()
        self.assertNotIn(f": {token}", text)
        self.assertNotIn("hunter2secret", text)
        self.assertIn("this is a token for production: [REDACTED]", text)
        self.assertIn(f"a/b/{token}x", text)  # paths stay readable

    def test_uuid_keys_pasted_under_a_key_label_are_redacted(self):
        key = "11111111-2222-3333-4444-555555555555"  # pragma: allowlist secret
        for prompt in (f"this is prod api key:\n{key}", f"prod key: {key}",
                       f"these are the keys:\n{key}\n{key}", "api key abcdef123456"):
            with self.subTest(prompt=prompt):
                base = make_base(self.base_dir / str(abs(hash(prompt))), writer="claude")
                (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
                self.assertEqual(self.hook(base, "reviewer", prompt).returncode, 0)
                [msg] = list((base / ".pair/inbox/writer").glob("*.md"))
                text = msg.read_text()
                self.assertNotIn(key, text)
                self.assertNotIn("abcdef123456", text)

    def test_uuid_without_a_key_label_stays_readable(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        cid = "11111111-2222-3333-4444-555555555555"
        self.assertEqual(self.hook(base, "reviewer", f"check cid: {cid}").returncode, 0)
        [msg] = list((base / ".pair/inbox/writer").glob("*.md"))
        self.assertIn(cid, msg.read_text())

    def test_failure_never_blocks_the_prompt_and_says_so(self):
        r = self.hook(self.base_dir, "writer", "hello")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("could NOT forward", r.stdout)

    def test_writer_prompt_queues_nothing_for_an_unreachable_reviewer(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        r = self.hook(base, "writer", "hello")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("does not forward user messages to the reviewer", r.stdout)
        self.assertEqual(len(list((base / ".pair/inbox/reviewer").glob("*.md"))), 0)

    def test_empty_prompt_does_nothing(self):
        base = make_base(self.base_dir, writer="claude")
        r = self.hook(base, "reviewer", "   ")
        self.assertEqual((r.returncode, r.stdout), (0, ""))
        self.assertEqual(list((base / ".pair/inbox/writer").glob("*.md")), [])

    def test_task_notification_is_not_forwarded(self):
        base = make_base(self.base_dir, writer="claude")
        r = self.hook(base, "writer", "<task-notification>\n<task-id>b1</task-id>\n"
                                      "<status>completed</status>\n</task-notification>")
        self.assertEqual((r.returncode, r.stdout), (0, ""))
        self.assertEqual(list((base / ".pair/inbox/reviewer").glob("*.md")), [])


    def test_writer_prompt_lists_the_threads_it_may_continue_newest_first(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        for topic, files in (("retry policy", "svc/a.go"), ("plan outline", "PLAN.md")):
            r = self.run_pair("thread", "open", "--base", str(base), "--topic", topic,
                              "--files", files, "--stdin", stdin="DECISION: " + topic)
            self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("thread", "close", "--base", str(base), "--id", "t1", "--status",
                          "agreed", "--stdin", stdin="retry 3 times with jitter in svc/a.go\nmore")
        self.assertEqual(r.returncode, 0, r.stderr)
        # a thread of an earlier reviewer process: its subagent is gone
        stale = base / ".pair/threads/t3"
        (stale / "inbox").mkdir(parents=True)
        (stale / "meta.json").write_text(json.dumps({
            "id": "t3", "topic": "old topic", "files": ["x.go"], "status": "closed",
            "reviewer_session": "ses_gone", "pair_session": "ses_old", "created_ms": 0}))
        r = self.hook(base, "writer", "and the retry: what about the timeout?")
        self.assertEqual(r.returncode, 0, r.stderr)
        out = r.stdout
        self.assertIn("PAIR THREADS, newest first:", out)
        self.assertRegex(out, r"t2 open \d+s: plan outline \[PLAN.md\]")
        self.assertRegex(out, r"t1 agreed \d+s: retry policy \[svc/a.go\] decided: retry 3 "
                              r"times with jitter in svc/a.go\n")
        self.assertLess(out.index("t2 open"), out.index("t1 agreed"))
        self.assertNotIn("old topic", out)
        self.assertIn("SendMessage", out)
        self.assertIn("thread: resume <id>", out)
        self.assertIn("thread: new", out)

    def test_reviewer_prompt_has_no_thread_routing(self):
        base = make_base(self.base_dir, writer="opencode")
        (base / ".pair/threads/t1/inbox").mkdir(parents=True)
        (base / ".pair/threads/t1/meta.json").write_text(json.dumps({
            "id": "t1", "topic": "x", "files": ["a"], "status": "open",
            "pair_session": "ses_test", "created_ms": 0}))
        r = self.hook(base, "reviewer", "why?")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertNotIn("PAIR THREADS", r.stdout)


class WaiterGuardTests(PairTestCase):
    """The writer's waiter dies when its Monitor expires; the hooks catch that."""

    def guard(self, base, event, **extra):
        payload = {"hook_event_name": event, **extra}
        return self.run_pair("guard", "--base", str(base), "--role", "writer",
                             stdin=json.dumps(payload))

    def queue(self, base, *bodies):
        for body in bodies:
            self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin=body)

    def test_live_waiter_is_silent(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        r = self.guard(base, "PostToolUse", tool_name="Bash")
        self.assertEqual((r.returncode, r.stdout), (0, ""))

    def transcript(self, *entries):
        path = Path(self.base_dir) / "transcript.jsonl"
        path.write_text("".join(json.dumps(e) + "\n" for e in entries))
        return str(path)

    def test_stop_that_asks_in_chat_is_blocked_once(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        path = self.transcript(
            {"type": "user", "message": {"content": "fix the bug"}},
            {"type": "assistant", "message": {"content": [
                {"type": "text", "text": "Fixed it. Shall I also rename the helper?"}]}})
        r = self.guard(base, "Stop", transcript_path=path)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout)["decision"], "block")
        self.assertIn("PAIR STOP CHECK", r.stdout)
        r = self.guard(base, "Stop", transcript_path=path, stop_hook_active=True)
        self.assertEqual((r.returncode, r.stdout), (0, ""))

    def test_stop_after_an_ask_user_call_or_a_plain_report_passes(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        asked = self.transcript(
            {"type": "user", "message": {"content": "deploy?"}},
            {"type": "assistant", "message": {"content": [
                {"type": "tool_use", "name": "AskUserQuestion", "input": {}}]}},
            {"type": "assistant", "message": {"content": [
                {"type": "text", "text": "I am waiting for your answer."}]}})
        r = self.guard(base, "Stop", transcript_path=asked)
        self.assertEqual((r.returncode, r.stdout), (0, ""))
        plain = self.transcript(
            {"type": "user", "message": {"content": "fix the bug"}},
            {"type": "assistant", "message": {"content": [
                {"type": "text", "text": "Fixed in svc/a.go; tests pass."}]}})
        r = self.guard(base, "Stop", transcript_path=plain)
        self.assertEqual((r.returncode, r.stdout), (0, ""))

    def test_dead_waiter_hands_over_the_backlog_in_order_and_says_to_rearm(self):
        base = make_base(self.base_dir, writer="claude")
        self.queue(base, "first", "second")
        r = self.guard(base, "PostToolUse", tool_name="Bash")
        self.assertEqual(r.returncode, 0, r.stderr)
        out = json.loads(r.stdout)["hookSpecificOutput"]
        self.assertEqual(out["hookEventName"], "PostToolUse")
        text = out["additionalContext"]
        self.assertLess(text.index("Chat from reviewer: first"), text.index("Chat from reviewer: second"))
        self.assertIn(f"wait --follow --base {base.resolve()} --role writer", text)
        self.assertEqual(list((base / ".pair/inbox/writer").glob("*.md")), [])
        self.assertEqual(len(list((base / ".pair/inbox/writer/read").glob("*.md"))), 2)

    def test_dead_waiter_without_backlog_still_says_to_rearm(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{dead_pid()}\n")
        r = self.guard(base, "PostToolUse", tool_name="Read")
        text = json.loads(r.stdout)["hookSpecificOutput"]["additionalContext"]
        self.assertIn("PAIR WAITER DOWN", text)
        self.assertNotIn("PAIR MESSAGES", text)

    def test_subagent_tool_calls_never_take_the_writers_messages(self):
        base = make_base(self.base_dir, writer="claude")
        self.queue(base, "first")
        r = self.guard(base, "PostToolUse", tool_name="Bash", agent_id="a1b2")
        self.assertEqual((r.returncode, r.stdout), (0, ""))
        self.assertEqual(len(list((base / ".pair/inbox/writer").glob("*.md"))), 1)

    def test_the_monitor_call_that_starts_the_waiter_is_not_nagged(self):
        base = make_base(self.base_dir, writer="claude")
        self.queue(base, "first")
        r = self.guard(base, "PostToolUse", tool_name="Monitor")
        self.assertEqual((r.returncode, r.stdout), (0, ""))
        self.assertEqual(len(list((base / ".pair/inbox/writer").glob("*.md"))), 1)

    def test_stop_without_a_waiter_is_blocked_once(self):
        base = make_base(self.base_dir, writer="claude")
        self.queue(base, "first")
        r = self.guard(base, "Stop", stop_hook_active=False)
        out = json.loads(r.stdout)
        self.assertEqual(out["decision"], "block")
        self.assertIn("Chat from reviewer: first", out["reason"])
        self.assertIn("wait --follow", out["reason"])
        r = self.guard(base, "Stop", stop_hook_active=True)
        self.assertEqual((r.returncode, r.stdout), (0, ""))

    def test_outside_pair_mode_is_silent(self):
        r = self.guard(self.base_dir, "PostToolUse", tool_name="Bash")
        self.assertEqual((r.returncode, r.stdout), (0, ""))

    def idle(self, base):
        r = self.run_pair("idle", "--base", str(base), "--role", "writer")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("PAIR IDLE", r.stdout)

    def test_idle_stops_the_waiter_and_quiets_the_hooks(self):
        base = make_base(self.base_dir, writer="claude")
        waiter = subprocess.Popen([sys.executable, str(PAIR), "wait", "--follow", "--base", str(base),
                                   "--role", "writer"], stdout=subprocess.PIPE, text=True)
        self.addCleanup(waiter.kill)
        deadline = time.monotonic() + 10
        while not (base / ".pair" / "waiter-writer.pid").exists() and time.monotonic() < deadline:
            time.sleep(0.05)
        self.idle(base)
        waiter.communicate(timeout=10)
        r = self.guard(base, "PostToolUse", tool_name="Read")
        self.assertEqual((r.returncode, r.stdout), (0, ""))
        r = self.guard(base, "Stop", stop_hook_active=False)
        self.assertEqual((r.returncode, r.stdout), (0, ""), "an idle agent may stop without a waiter")

    def test_idle_still_hands_over_a_queued_message_and_ends_idle(self):
        base = make_base(self.base_dir, writer="claude")
        self.idle(base)
        self.queue(base, "first")
        r = self.guard(base, "Stop", stop_hook_active=False)
        out = json.loads(r.stdout)
        self.assertEqual(out["decision"], "block")
        self.assertIn("Chat from reviewer: first", out["reason"])
        self.assertIn("PAIR WAITER DOWN", out["reason"])
        self.assertFalse((base / ".pair" / "idle-writer").exists())

    def test_a_user_message_ends_idle_and_says_to_rearm(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        self.idle(base)
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": "next task"}))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("PAIR WAITER DOWN", r.stdout)
        self.assertFalse((base / ".pair" / "idle-writer").exists())

    def test_a_task_notification_does_not_end_idle(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        self.idle(base)
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": "<task-notification>done</task-notification>"}))
        self.assertEqual((r.returncode, r.stdout), (0, ""))
        self.assertTrue((base / ".pair" / "idle-writer").exists())

    def test_the_first_prompt_of_a_fresh_restart_stays_with_the_agent(self):
        sys.path.insert(0, str(HERE))
        import pair
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        prompt = pair.FRESH_PROMPTS["impl"].format(decisions=".pair/decisions.md", stage="IMPL")
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": prompt, "session_id": WRITER_ID}))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(stub.prompts(), [])
        self.assertEqual(list((base / ".pair" / "inbox" / "reviewer").glob("*.md")), [])
        self.assertNotIn("PAIR MODE", r.stdout)
        self.assertIn("TASK STAGE:", r.stdout)
        # the new conversation is still recorded: a later `stage` resumes it
        cfg = json.loads((base / ".pair" / "config.json").read_text())
        self.assertEqual(cfg["claude_session"]["writer"], WRITER_ID)

    def test_starting_a_wait_ends_idle(self):
        base = make_base(self.base_dir, writer="claude")
        self.idle(base)
        self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "0.2")
        self.assertFalse((base / ".pair" / "idle-writer").exists())

    def test_session_start_while_idle_does_not_rearm(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        self.idle(base)
        r = self.run_pair("session-start", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"hook_event_name": "SessionStart", "source": "clear"}))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("PAIR MODE resumed after /clear", r.stdout)
        self.assertIn("idle", r.stdout)
        self.assertNotIn("wait --follow", r.stdout)
        self.assertTrue((base / ".pair" / "idle-writer").exists())

    def test_user_prompt_with_a_dead_waiter_hands_over_the_backlog(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        self.queue(base, "first")
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": "status?"}))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("Chat from reviewer: first", r.stdout)
        self.assertIn("PAIR WAITER DOWN", r.stdout)


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
                                        "reviewer": {"kind": "opencode",
                                                     "agent": "review"}})
        self.assertEqual(cfg["stage"], "plan")
        self.assertIsNone(cfg["opencode"]["session"])
        for d in ("inbox/writer/read", "inbox/reviewer/read", "files"):
            self.assertTrue((self.base_dir / ".pair" / d).is_dir(), d)
        hook = json.loads((self.base_dir / ".pair/claude-writer-settings.json").read_text())
        [entry] = hook["hooks"]["UserPromptSubmit"]
        self.assertIn(" user-prompt --base ", entry["hooks"][0]["command"])
        self.assertTrue(entry["hooks"][0]["command"].endswith("--role writer"))
        self.assertFalse((self.base_dir / ".pair/claude-reviewer-settings.json").exists())
        [post] = hook["hooks"]["PostToolUse"]
        self.assertIn(" guard --base ", post["hooks"][0]["command"])
        [stop] = hook["hooks"]["Stop"]
        self.assertIn(" guard --base ", stop["hooks"][0]["command"])
        [start] = hook["hooks"]["SessionStart"]
        self.assertEqual(start["matcher"], "clear")
        self.assertIn(" session-start --base ", start["hooks"][0]["command"])
        [edit] = hook["hooks"]["PreToolUse"]
        self.assertEqual(edit["matcher"], "Edit|Write|MultiEdit|NotebookEdit")
        self.assertIn(" edit-guard --base ", edit["hooks"][0]["command"])
        oc = json.loads((self.base_dir / ".pair/opencode-reviewer.json").read_text())
        self.assertEqual(oc["agent"]["review"]["permission"], {"edit": "deny", "bash": "allow"})
        [plugin] = oc["plugin"]
        self.assertTrue(plugin.startswith("file://") and plugin.endswith("/pair-opencode-plugin.js"))
        self.assertTrue(Path(plugin[len("file://"):]).is_file())

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
        self.assertIn(f"wait --follow --base {base} --role writer", writer)
        self.assertIn("Monitor", writer)
        self.assertIn("READ-ONLY", reviewer)
        self.assertIn("A hard opinion question", writer)
        self.assertIn("liaison", writer)
        self.assertIn("send the writer DECISION: user asked", reviewer)
        self.assertNotIn(" wait --base", reviewer)

    def test_brief_then_and_reopen_make_a_first_prompt(self):
        self.init("opencode")
        r = self.run_pair("brief", "--base", str(self.base_dir), "--role", "writer",
                          "--reopen", "--then", "task plan")
        self.assertEqual(r.returncode, 0, r.stderr)
        text = r.stdout.strip()
        self.assertIn("This task was re-opened", text)
        self.assertTrue(text.endswith("Then do this: task plan"))

    def test_brief_refuses_shell_special_characters(self):
        self.init("claude")
        for bad in ('say "hi"', "cost $5", "run `x`", "a\\b"):
            r = self.run_pair("brief", "--base", str(self.base_dir), "--role", "writer", "--then", bad)
            self.assertEqual(r.returncode, 1, bad)

    def test_brief_refuses_a_path_with_an_apostrophe(self):
        odd = self.root / "it's"
        odd.mkdir()
        self.assertEqual(self.run_pair("init", "--base", str(odd), "--writer", "claude").returncode, 0)
        r = self.run_pair("brief", "--base", str(odd), "--role", "writer")
        self.assertEqual(r.returncode, 1)
        self.assertIn("shell-special", r.stderr)

class LaunchTests(PairTestCase):
    def setup_pair(self, writer):
        self.stub = StubOpencode()
        self.addCleanup(self.stub.close)
        self.run_pair("init", "--base", str(self.base_dir), "--writer", writer)
        path = self.base_dir / ".pair/config.json"
        cfg = json.loads(path.read_text())
        cfg["opencode"]["port"] = self.stub.port
        path.write_text(json.dumps(cfg))
        # the session opencode itself creates from --prompt, plus an older one to ignore
        self.stub.sessions = [
            {"id": "ses_old", "directory": str(self.base_dir.resolve()),
             "time": {"created": cfg["started_ms"] - 60000, "updated": cfg["started_ms"] - 50000}},
            {"id": "ses_other_dir", "directory": "/elsewhere",
             "time": {"created": cfg["started_ms"] + 5, "updated": cfg["started_ms"] + 5}},
            {"id": "ses_mine", "directory": str(self.base_dir.resolve()),
             "time": {"created": cfg["started_ms"] + 10, "updated": cfg["started_ms"] + 10}},
        ]
        reviewer = "opencode" if writer == "claude" else "claude"
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "hasSplit": True, "splitForegroundShell": "zsh",
            "splitForeground": [reviewer, "--flag"]}]}]}}}))

    def launch(self, *extra):
        return self.run_pair("launch", "--base", str(self.base_dir), "--session", "SID-1", *extra)

    def typed(self):
        return [c["stdin"] for c in self.agterm_calls() if c["argv"][:2] == ["session", "type"]]

    def test_opencode_reviewer_starts_with_its_brief_as_first_prompt(self):
        self.setup_pair("claude")
        r = self.launch()
        self.assertEqual(r.returncode, 0, r.stderr)
        calls = self.agterm_calls()
        self.assertEqual(calls[0]["argv"], ["session", "split", "on", "--target", "SID-1"])
        line, enter = self.typed()
        script = self.base_dir.resolve() / ".pair/reviewer-start.sh"
        self.assertEqual(line, f"sh {script}")
        self.assertEqual(enter, "\n")
        start = script.read_text()
        self.assertIn("export OPENCODE_CONFIG=", start)
        self.assertIn(f"exec opencode --auto --agent review --port {self.stub.port} "
                      "--prompt ", start)
        self.assertIn("reviewer-brief.txt", start)
        for c in calls:
            if c["argv"][:2] == ["session", "type"]:
                self.assertEqual(c["argv"][2:], ["--stdin", "--target", "SID-1", "--pane", "right"])
        self.assertIn("you are the REVIEWER", (self.base_dir / ".pair/reviewer-brief.txt").read_text())
        self.assertFalse(any(m == "POST" for m, _, _ in self.stub.requests))
        cfg = json.loads((self.base_dir / ".pair/config.json").read_text())
        self.assertEqual((cfg["agterm_session"], cfg["opencode"]["session"]), ("SID-1", "ses_mine"))

    def test_claude_reviewer_and_the_opencode_writer_session_is_found(self):
        self.setup_pair("opencode")
        r = self.launch()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.typed()
        start = (self.base_dir / ".pair/reviewer-start.sh").read_text()
        self.assertIn("\nexec claude --dangerously-skip-permissions --disallowedTools Edit Write "
                      "NotebookEdit ", start)
        self.assertIn("--settings ", start)
        self.assertIn("claude-reviewer-settings.json", start)
        self.assertIn("reviewer-brief.txt", start)
        self.assertIn("READ-ONLY", (self.base_dir / ".pair/reviewer-brief.txt").read_text())
        self.assertEqual(self.stub.prompts(), [])
        cfg = json.loads((self.base_dir / ".pair/config.json").read_text())
        self.assertEqual(cfg["opencode"]["session"], "ses_mine")

    def test_reopen_puts_catch_up_into_the_reviewer_brief(self):
        self.setup_pair("claude")
        self.assertEqual(self.launch("--reopen").returncode, 0)
        self.assertIn("This task was re-opened", (self.base_dir / ".pair/reviewer-brief.txt").read_text())

    def test_missing_reviewer_in_split_fails_with_timeout(self):
        self.setup_pair("claude")
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "hasSplit": True, "splitForegroundShell": "zsh"}]}]}}}))
        r = self.launch("--wait-seconds", "1")
        self.assertEqual(r.returncode, 1)
        self.assertIn("timed out waiting for the opencode reviewer", r.stderr)

    def test_garbled_line_is_retyped_into_the_idle_shell(self):
        self.setup_pair("claude")
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "hasSplit": True, "splitForegroundShell": "zsh"}]}]}}}))
        r = self.launch("--wait-seconds", "1")
        self.assertEqual(r.returncode, 1)
        lines = [t for t in self.typed() if t != "\n"]
        self.assertEqual(len(lines), 3)
        self.assertEqual(len(set(lines)), 1)

    def test_opencode_reviewer_without_its_agent_fails(self):
        self.setup_pair("claude")
        self.stub.agents = [{"name": "main"}]
        r = self.launch()
        self.assertEqual(r.returncode, 1)
        self.assertIn("without the review agent", r.stderr)

    def test_no_new_opencode_session_fails_with_timeout(self):
        self.setup_pair("claude")
        self.stub.sessions = self.stub.sessions[:2]
        r = self.launch("--wait-seconds", "1")
        self.assertEqual(r.returncode, 1)
        self.assertIn("timed out waiting for the opencode session", r.stderr)


class ReviewFixTests(PairTestCase):
    def test_send_while_claude_handles_a_batch_queues_without_blocked(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin="a")
        self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "5")
        r = self.run_pair("send", "--base", str(base), "--to", "writer", "--stdin", stdin="b")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout), {"sent": json.loads(r.stdout)["sent"],
                                                "delivered": False, "queued": True})
        self.assertFalse(any(c["argv"][:2] == ["session", "status"] for c in self.agterm_calls()))

    def test_flush_delivers_a_stranded_opencode_backlog(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        self.assertEqual(self.run_pair("send", "--base", str(base), "--to", "reviewer",
                                       "--stdin", stdin="stranded").returncode, 1)
        stub = StubOpencode()
        self.addCleanup(stub.close)
        cfg = json.loads((base / ".pair/config.json").read_text())
        cfg["opencode"]["port"] = stub.port
        (base / ".pair/config.json").write_text(json.dumps(cfg))
        r = self.run_pair("flush", "--base", str(base), "--to", "reviewer")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual([p["parts"][0]["text"] for p in stub.prompts()], ["Chat from writer: stranded"])

    def test_launch_delivers_the_opencode_backlog(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        self.run_pair("init", "--base", str(self.base_dir), "--writer", "claude")
        path = self.base_dir / ".pair/config.json"
        cfg = json.loads(path.read_text())
        cfg["opencode"]["port"] = stub.port
        path.write_text(json.dumps(cfg))
        self.run_pair("send", "--base", str(self.base_dir), "--to", "reviewer", "--stdin", stdin="queued before launch")
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "hasSplit": True, "splitForegroundShell": "zsh",
            "splitForeground": ["opencode"]}]}]}}}))
        stub.sessions = [{"id": "ses_mine", "directory": str(self.base_dir.resolve()),
                          "time": {"created": cfg["started_ms"] + 10, "updated": cfg["started_ms"] + 10}}]
        r = self.run_pair("launch", "--base", str(self.base_dir), "--session", "SID-1", "--reopen")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual([p["parts"][0]["text"] for p in stub.prompts()],
                         ["Chat from writer: queued before launch"])

class ExistingSessionTests(PairTestCase):
    def tree_with(self, **node):
        node.setdefault("id", "OLD-1")
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [node]}]}}}))

    def inspect(self, field, base=None):
        args = ["inspect", "--session", "OLD-1", "--get", field]
        if base:
            args += ["--base", str(base)]
        return self.run_pair(*args)

    def test_inspect_reports_writer_kind_from_the_left_pane(self):
        self.tree_with(foreground=["claude", "--dangerously-skip-permissions"])
        self.assertEqual(self.inspect("writer").stdout.strip(), "claude")
        self.tree_with(foreground=["opencode", "--auto"])
        self.assertEqual(self.inspect("writer").stdout.strip(), "opencode")
        self.tree_with(foregroundShell="zsh")
        self.assertEqual(self.inspect("writer").stdout.strip(), "none")

    def test_inspect_reports_agent_status(self):
        self.tree_with(foreground=["claude"], status="active")
        self.assertEqual(self.inspect("status").stdout.strip(), "active")
        self.tree_with(foreground=["claude"])
        self.assertEqual(self.inspect("status").stdout.strip(), "idle")

    def test_inspect_paired_only_when_the_split_runs_the_reviewer(self):
        make_base(self.base_dir, writer="claude")
        self.tree_with(foreground=["claude"], hasSplit=True, splitForeground=["opencode", "--auto"])
        self.assertEqual(self.inspect("paired", self.base_dir).stdout.strip(), "1")
        self.tree_with(foreground=["claude"], hasSplit=True, splitForegroundShell="zsh")
        self.assertEqual(self.inspect("paired", self.base_dir).stdout.strip(), "0")
        self.tree_with(foreground=["claude"])
        self.assertEqual(self.inspect("paired", self.root / "no-pair-here").stdout.strip(), "0")

    def respawn(self, env_session):
        env = dict(self.env)
        env["AGTERM_SESSION_ID"] = env_session
        self.env = env
        return self.run_pair("respawn", "--old", "OLD-1", "--name", "T-1 thing",
                             "--cwd", str(self.base_dir), "--command", "zsh -lc 'claude --continue'")

    def test_respawn_creates_after_the_old_session_then_closes_it(self):
        self.tree_with()
        r = self.respawn("SOMEONE-ELSE")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.strip(), "11111111-2222-3333-4444-555555555555")
        argvs = [c["argv"] for c in self.agterm_calls()]
        self.assertIn(["session", "new", "--cwd", str(self.base_dir), "--name", "T-1 thing",
                       "--after", "OLD-1", "--no-select", "--command",
                       "zsh -lc 'claude --continue'"], argvs)
        self.assertIn(["session", "close", "--target", "OLD-1"], argvs)
        self.assertFalse([a for a in argvs if a[:2] == ["session", "tint"]])

    def test_respawn_is_refused_while_a_companion_is_open(self):
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [
            {"id": "OLD-1"}, {"id": "C-1", "name": "test core T-1", "parent": "OLD-1"}]}]}}}))
        r = self.respawn("SOMEONE-ELSE")
        self.assertEqual(r.returncode, 5, r.stderr)
        self.assertIn("test core T-1", r.stderr)
        argvs = [c["argv"] for c in self.agterm_calls()]
        self.assertFalse([a for a in argvs if a[:2] in (["session", "new"], ["session", "close"])])

    def test_respawn_keeps_the_old_session_tint(self):
        self.tree_with(tint="purple", tintOn=["icon"])
        r = self.respawn("SOMEONE-ELSE")
        self.assertEqual(r.returncode, 0, r.stderr)
        argvs = [c["argv"] for c in self.agterm_calls()]
        self.assertIn(["session", "tint", "purple", "--target", "11111111-2222-3333-4444-555555555555"], argvs)

    def test_respawn_from_inside_the_old_session_closes_it_later(self):
        self.tree_with()
        r = self.respawn("OLD-1")
        self.assertEqual(r.returncode, 0, r.stderr)
        argvs = [c["argv"] for c in self.agterm_calls()]
        self.assertNotIn(["session", "close", "--target", "OLD-1"], argvs)
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if ["session", "close", "--target", "OLD-1"] in [c["argv"] for c in self.agterm_calls()]:
                break
            time.sleep(0.2)
        else:
            self.fail("the delayed close never ran")

    def test_launch_accepts_a_continued_opencode_session(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        self.run_pair("init", "--base", str(self.base_dir), "--writer", "opencode")
        path = self.base_dir / ".pair/config.json"
        cfg = json.loads(path.read_text())
        cfg["opencode"]["port"] = stub.port
        path.write_text(json.dumps(cfg))
        stub.sessions = [{"id": "ses_continued", "directory": str(self.base_dir.resolve()),
                          "time": {"created": cfg["started_ms"] - 3600000,
                                   "updated": cfg["started_ms"] + 20}}]
        self.tree_with(id="SID-1", hasSplit=True, splitForegroundShell="zsh", splitForeground=["claude"])
        r = self.run_pair("launch", "--base", str(self.base_dir), "--session", "SID-1")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(path.read_text())["opencode"]["session"], "ses_continued")

class ThreadTests(PairTestCase):
    """One decision per thread: a reviewer child session and a writer fork."""

    def setUp(self):
        super().setUp()
        self.stub = StubOpencode()
        self.addCleanup(self.stub.close)
        self.base = make_base(self.base_dir, writer="claude", port=self.stub.port)

    def open_thread(self, files="svc/a.go", body="PROPOSAL: add a retry", topic="retry"):
        return self.run_pair("thread", "open", "--base", str(self.base), "--topic", topic,
                             "--files", files, "--stdin", stdin=body)

    def reply(self, tid, body, key=None):
        """Send as the thread's reviewer child, with the key from its first prompt."""
        if key is None:
            first = next(p["parts"][0]["text"] for p in self.stub.prompts()
                         if p["parts"][0]["text"].startswith(f"PAIR THREAD {tid}:"))
            key = re.search(r"--key (\S+)", first).group(1)
        return self.run_pair("thread", "send", "--base", str(self.base), "--id", tid,
                             "--key", key, "--to", "writer", "--stdin", stdin=body)

    def meta(self, tid):
        return json.loads((self.base / ".pair/threads" / tid / "meta.json").read_text())

    def test_open_creates_a_reviewer_child_session_and_sends_brief_then_proposal(self):
        r = self.open_thread()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout), {"id": "t1", "reviewer_session": "ses_new"})
        self.assertIn(("POST", "/session", {"parentID": "ses_test", "agent": "review",
                                            "title": "pair t1: retry"}), self.stub.requests)
        [prompt] = self.stub.prompts()
        self.assertEqual(prompt["agent"], "review")
        text = prompt["parts"][0]["text"]
        self.assertTrue(text.startswith("PAIR THREAD t1: you are the REVIEWER"))
        self.assertIn(str(PAIR.parent.parent / "pair-thread.md"), text)
        self.assertNotIn("pair.md", text.replace("pair-thread.md", ""))
        self.assertIn("thread send --base", text)
        self.assertRegex(text, r"--id t1 --key \S+ --to writer --stdin")
        self.assertTrue(text.endswith("Chat from writer [t1]: PROPOSAL: add a retry"))
        self.assertIn(("POST", "/session/ses_new/prompt_async", prompt), self.stub.requests)
        meta = self.meta("t1")
        self.assertEqual((meta["status"], meta["topic"], meta["files"], meta["reviewer_session"]),
                         ("open", "retry", ["svc/a.go"], "ses_new"))

    def test_two_opens_get_distinct_ids_and_child_sessions(self):
        self.open_thread(files="a.go")
        r = self.open_thread(files="b.go")
        self.assertEqual(json.loads(r.stdout), {"id": "t2", "reviewer_session": "ses_new2"})

    def test_overlapping_files_of_an_unclosed_thread_are_refused_with_exit_5(self):
        self.assertEqual(self.open_thread(files="svc/,docs/x.md").returncode, 0)
        for files in ("svc/a.go", "docs/x.md", "./svc/"):
            r = self.open_thread(files=files)
            self.assertEqual(r.returncode, 5, files)
            self.assertIn("t1", r.stderr)
        self.assertFalse((self.base / ".pair/threads/t2").exists())
        self.assertEqual(self.open_thread(files="svcx/a.go").returncode, 0)

    def test_a_closed_thread_frees_its_files(self):
        self.open_thread()
        self.run_pair("thread", "close", "--base", str(self.base), "--id", "t1",
                      "--status", "closed")
        self.assertEqual(self.open_thread().returncode, 0)

    def test_claude_reviewer_is_refused_with_exit_2(self):
        (self.root / "oc").mkdir()
        base = make_base(self.root / "oc", writer="opencode")
        r = self.run_pair("thread", "open", "--base", str(base), "--topic", "t", "--files", "a",
                          "--stdin", stdin="PROPOSAL: x")
        self.assertEqual(r.returncode, 2)
        self.assertIn("OpenCode", r.stderr)

    def test_reviewer_reply_reaches_only_that_threads_wait(self):
        self.open_thread(files="a.go")
        self.open_thread(files="b.go")
        r = self.reply("t2", "AGREE: fine")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("thread", "wait", "--base", str(self.base), "--id", "t1", "--timeout", "1")
        self.assertEqual(r.returncode, 4)
        r = self.run_pair("thread", "wait", "--base", str(self.base), "--id", "t2", "--timeout", "5")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("Chat from reviewer [t2]: AGREE: fine", r.stdout)
        r = self.run_pair("thread", "wait", "--base", str(self.base), "--id", "t2", "--timeout", "1")
        self.assertEqual(r.returncode, 4)

    def test_writer_follow_up_goes_to_the_same_child_session(self):
        self.open_thread()
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                          "reviewer", "--stdin", stdin="ANSWER: the caller retries already")
        self.assertEqual(r.returncode, 0, r.stderr)
        last = self.stub.prompts()[-1]
        self.assertEqual(last["parts"][0]["text"],
                         "Chat from writer [t1]: ANSWER: the caller retries already")
        self.assertEqual(self.stub.calls()[-1][1], "/session/ses_new/prompt_async")

    def test_send_is_refused_for_an_unknown_thread_an_oversized_body_or_a_settled_thread(self):
        self.open_thread()
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t9", "--to",
                          "writer", "--stdin", stdin="x")
        self.assertEqual(r.returncode, 1)
        r = self.reply("t1", "x" * 4001)
        self.assertEqual(r.returncode, 1)
        self.run_pair("thread", "close", "--base", str(self.base), "--id", "t1", "--status",
                      "agreed", "--stdin", stdin="retry in svc/a.go")
        r = self.reply("t1", "late")
        self.assertEqual(r.returncode, 1)
        self.assertIn("agreed", r.stderr)
        self.assertIn("Do not send it again", r.stderr)
        self.assertNotIn("pair.py send", r.stderr)

    def test_opencode_down_keeps_the_writer_message_and_the_next_send_flushes_it(self):
        self.open_thread()
        cfg = json.loads((self.base / ".pair/config.json").read_text())
        cfg["opencode"]["port"] = free_port()
        (self.base / ".pair/config.json").write_text(json.dumps(cfg))
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                          "reviewer", "--stdin", stdin="first")
        self.assertEqual(r.returncode, 1)
        cfg["opencode"]["port"] = self.stub.port
        (self.base / ".pair/config.json").write_text(json.dumps(cfg))
        self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                      "reviewer", "--stdin", stdin="second")
        self.assertEqual([p["parts"][0]["text"] for p in self.stub.prompts()[1:]],
                         ["Chat from writer [t1]: first", "Chat from writer [t1]: second"])

    def test_opencode_down_at_open_leaves_no_thread(self):
        cfg = json.loads((self.base / ".pair/config.json").read_text())
        cfg["opencode"]["port"] = free_port()
        (self.base / ".pair/config.json").write_text(json.dumps(cfg))
        r = self.open_thread()
        self.assertEqual(r.returncode, 1)
        self.assertEqual(list((self.base / ".pair").glob("threads/t*")), [])

    def test_close_writes_the_decision_and_threads_lists_every_thread(self):
        self.open_thread(files="a.go", topic="retry")
        self.open_thread(files="b.go", topic="cache key")
        r = self.run_pair("thread", "close", "--base", str(self.base), "--id", "t1", "--status",
                          "agreed", "--stdin", stdin="Retry 3 times in a.go Fetch.")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual((self.base / ".pair/threads/t1/decision.md").read_text(),
                         "Retry 3 times in a.go Fetch.\n")
        self.assertEqual(self.meta("t1")["status"], "agreed")
        r = self.run_pair("thread", "close", "--base", str(self.base), "--id", "t2", "--status",
                          "disputed")
        self.assertEqual(r.returncode, 1)
        self.assertIn("--stdin", r.stderr)
        out = self.run_pair("threads", "--base", str(self.base)).stdout.splitlines()
        self.assertEqual(len(out), 2)
        self.assertTrue(out[0].startswith("t1") and "agreed" in out[0] and "retry" in out[0])
        self.assertTrue(out[1].startswith("t2") and "open" in out[1] and "b.go" in out[1])

    def test_close_is_refused_with_exit_8_while_a_reviewer_reply_is_unread(self):
        self.open_thread()
        self.assertEqual(self.reply("t1", "ANSWER: one more point").returncode, 0)
        r = self.run_pair("thread", "close", "--base", str(self.base), "--id", "t1", "--status",
                          "agreed", "--stdin", stdin="Retry 3 times.")
        self.assertEqual(r.returncode, 8)
        self.assertIn("thread wait", r.stderr)
        self.assertEqual(self.meta("t1")["status"], "open")
        self.assertFalse((self.base / ".pair/threads/t1/decision.md").exists())
        r = self.run_pair("thread", "wait", "--base", str(self.base), "--id", "t1", "--timeout", "5")
        self.assertIn("ANSWER: one more point", r.stdout)
        r = self.run_pair("thread", "close", "--base", str(self.base), "--id", "t1", "--status",
                          "agreed", "--stdin", stdin="Retry 3 times.")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.meta("t1")["status"], "agreed")

    def test_a_thread_message_releases_the_main_reviewers_held_fyis_to_the_writer(self):
        r = self.run_pair("send", "--base", str(self.base), "--to", "writer", "--stdin",
                          stdin="FYI: ready for the plan discussion")
        self.assertTrue(json.loads(r.stdout)["held"])
        self.open_thread()
        self.assertEqual(self.reply("t1", "VIEW: retry in the caller").returncode, 0)
        self.assertEqual(list((self.base / ".pair/held/writer").glob("*.md")), [])
        r = self.run_pair("wait", "--base", str(self.base), "--role", "writer", "--timeout", "5")
        self.assertIn("Chat from reviewer: FYI: ready for the plan discussion", r.stdout)

    def test_reply_to_the_writer_needs_the_thread_key(self):
        self.open_thread()
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                          "writer", "--stdin", stdin="AGREE: hijack")
        self.assertEqual(r.returncode, 1)
        self.assertIn("--key", r.stderr)
        self.assertEqual(self.reply("t1", "AGREE: hijack", key="wrong").returncode, 1)
        self.assertEqual(list((self.base / ".pair/threads/t1/inbox/writer").glob("*.md")), [])
        self.assertEqual(self.reply("t1", "AGREE: real").returncode, 0)

    def test_neither_the_thread_brief_nor_its_key_is_on_disk(self):
        self.open_thread()
        key = re.search(r"--key (\S+)", self.stub.prompts()[0]["parts"][0]["text"]).group(1)
        for path in (self.base / ".pair").rglob("*"):
            if path.is_file():
                text = path.read_text(errors="ignore")
                self.assertNotIn("PAIR THREAD", text, path)
                self.assertNotIn(key, text, path)

    def test_a_proposal_the_child_did_not_take_leaves_no_thread(self):
        self.env["PAIR_CONFIRM_SECONDS"] = "0.5"
        for failure in ("fail_prompts", "drop_prompts"):
            with self.subTest(failure=failure):
                setattr(self.stub, failure, True)
                self.stub.requests.clear()
                r = self.open_thread()
                self.assertEqual(r.returncode, 1)
                self.assertIn("thread not opened", r.stderr)
                self.assertFalse((self.base / ".pair/threads/t1").exists())
                deleted = [path for method, path, _ in self.stub.requests if method == "DELETE"]
                self.assertEqual(len(deleted), 1)
                self.assertRegex(deleted[0], r"^/session/ses_new\d*$")
                setattr(self.stub, failure, False)
        # the files are free again and the next open briefs its own child
        r = self.open_thread()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout)["id"], "t1")
        self.assertTrue(self.stub.prompts()[-1]["parts"][0]["text"].startswith("PAIR THREAD t1:"))

    def test_a_follow_up_the_child_did_not_take_stays_queued_for_the_next_send(self):
        self.open_thread()
        self.env["PAIR_CONFIRM_SECONDS"] = "0.5"
        self.stub.drop_prompts = True
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                          "reviewer", "--stdin", stdin="first")
        self.assertEqual(r.returncode, 1)
        self.assertIn("did not take the prompt", r.stderr)
        self.stub.drop_prompts = False
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                          "reviewer", "--stdin", stdin="second")
        self.assertEqual(r.returncode, 0, r.stderr)
        stored = [m["parts"][0]["text"] for m in self.stub.messages["ses_new"]]
        self.assertEqual(stored[-2:], ["Chat from writer [t1]: first",
                                       "Chat from writer [t1]: second"])

    def close(self, tid, status="closed", body=None):
        extra = ("--stdin",) if body else ()
        return self.run_pair("thread", "close", "--base", str(self.base), "--id", tid,
                             "--status", status, *extra, stdin=body)

    def resume(self, tid, body):
        return self.run_pair("thread", "resume", "--base", str(self.base), "--id", tid,
                             "--stdin", stdin=body)

    def test_resume_brings_a_closed_topic_back_to_its_own_reviewer_subagent(self):
        self.open_thread(files="a.go")
        self.open_thread(files="b.go")
        self.close("t1")
        r = self.resume("t1", "PROPOSAL: the retry also needs a jitter")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout),
                         {"id": "t1", "reviewer_session": "ses_new", "status": "open"})
        self.assertEqual(self.meta("t1")["status"], "open")
        method, path, body = self.stub.calls()[-1]
        self.assertEqual(path, "/session/ses_new/prompt_async")
        self.assertEqual(body["parts"][0]["text"],
                         "Chat from writer [t1]: PROPOSAL: the retry also needs a jitter")
        self.assertEqual(self.reply("t1", "AGREE: jitter is fine").returncode, 0)

    def test_resume_of_an_open_thread_is_refused_so_two_liaisons_never_share_it(self):
        self.open_thread()
        before = len(self.stub.prompts())
        r = self.resume("t1", "DECISION: user asked: cap it at 3?")
        self.assertEqual(r.returncode, 5)
        self.assertIn("SendMessage", r.stderr)
        self.assertEqual(len(self.stub.prompts()), before)
        self.assertEqual(list((self.base / ".pair/threads/t1/inbox/reviewer").glob("*.md")), [])

    def test_resume_is_refused_after_the_main_reviewer_session_changed(self):
        self.open_thread()
        self.close("t1")
        cfg = json.loads((self.base / ".pair/config.json").read_text())
        cfg["opencode"]["session"] = "ses_relaunched"
        (self.base / ".pair/config.json").write_text(json.dumps(cfg))
        r = self.resume("t1", "PROPOSAL: again")
        self.assertEqual(r.returncode, 6)
        self.assertIn("thread open", r.stderr)
        self.assertEqual(self.meta("t1")["status"], "closed")

    def test_resume_is_refused_when_another_unclosed_thread_covers_its_files(self):
        self.open_thread(files="a.go")
        self.close("t1")
        self.open_thread(files="a.go")
        r = self.resume("t1", "PROPOSAL: again")
        self.assertEqual(r.returncode, 5)
        self.assertIn("t2", r.stderr)
        self.assertEqual(self.meta("t1")["status"], "closed")

    def test_writer_send_to_a_closed_thread_points_to_resume(self):
        self.open_thread()
        self.close("t1")
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                          "reviewer", "--stdin", stdin="one more thing")
        self.assertEqual(r.returncode, 1)
        self.assertIn("thread resume --base", r.stderr)

    def test_main_opencode_reviewer_brief_keeps_out_of_threads(self):
        r = self.run_pair("brief", "--base", str(self.base), "--role", "reviewer")
        self.assertIn(".pair/threads", r.stdout)
        self.assertIn("never run thread send", r.stdout)

    def test_init_makes_the_reviewer_agent_usable_as_a_subagent(self):
        (self.root / "wt2").mkdir()
        self.run_pair("init", "--base", str(self.root / "wt2"), "--writer", "claude")
        oc =json.loads((self.root / "wt2/.pair/opencode-reviewer.json").read_text())
        self.assertEqual(oc["agent"]["review"]["mode"], "all")

    def test_init_runs_the_reviewer_on_the_plan_stage_model(self):
        (self.root / "wt2").mkdir()
        self.run_pair("init", "--base", str(self.root / "wt2"), "--writer", "claude")
        oc = json.loads((self.root / "wt2/.pair/opencode-reviewer.json").read_text())
        self.assertEqual(oc["agent"]["review"]["model"], "openai/gpt-6-astra")
        self.assertEqual(oc["agent"]["review"]["variant"], "high")

    def test_writer_brief_names_the_liaison_only_with_an_opencode_reviewer(self):
        r = self.run_pair("brief", "--base", str(self.base), "--role", "writer")
        self.assertIn("liaison", r.stdout)
        self.assertNotIn("thread open", r.stdout)
        oc = self.root / "oc"
        oc.mkdir()
        make_base(oc, writer="opencode")
        r = self.run_pair("brief", "--base", str(oc), "--role", "writer")
        self.assertNotIn("liaison", r.stdout)
        self.assertIn("VIEW: user asked", r.stdout)


if __name__ == "__main__":
    unittest.main()


FAKE_REVIEWER_SERVER = """
import http.server, sys
class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        self.send_response(200); self.send_header("content-length", "2"); self.end_headers()
        self.wfile.write(b"{}")
http.server.HTTPServer(("127.0.0.1", int(sys.argv[-1])), H).serve_forever()
"""


class RestartReviewerTests(PairTestCase):
    def fake_process(self, *argv, port=None):
        """A stand-in reviewer whose command line carries the reviewer's marks."""
        code = FAKE_REVIEWER_SERVER if port else "import time; time.sleep(60)"
        proc = subprocess.Popen([sys.executable, "-c", code, *argv])
        self.addCleanup(lambda: (proc.poll() is None and proc.kill(), proc.wait()))
        if port:
            deadline = time.monotonic() + 10
            while time.monotonic() < deadline:
                with socket.socket() as s:
                    if s.connect_ex(("127.0.0.1", port)) == 0:
                        break
                time.sleep(0.05)
        return proc

    def setup_opencode_reviewer(self):
        """Claude writer; the dead reviewer listened on old_port, the new one on the stub."""
        self.stub = StubOpencode()
        self.addCleanup(self.stub.close)
        self.old_port = free_port()
        base = make_base(self.base_dir, writer="claude", port=self.old_port, session="ses_old",
                         agterm="SID-1")
        self.env["PAIR_FREE_PORT"] = str(self.stub.port)
        future = int(time.time() * 1000) + 10 ** 9
        self.stub.sessions = [{"id": "ses_restarted", "directory": str(base.resolve()),
                               "time": {"created": future, "updated": future}}]
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "splitForegroundShell": "zsh",
            "splitForeground": ["opencode", "--flag"]}]}]}}}))
        return base

    def restart(self, base, *extra):
        return self.run_pair("restart-reviewer", "--base", str(base), "--wait-seconds", "0.5",
                             *extra)

    def cfg(self, base):
        return json.loads((base / ".pair/config.json").read_text())

    def opencode_argv(self, port):
        return ["opencode", "--auto", "--agent", "review", "--port", str(port)]

    def test_check_reports_a_dead_opencode_reviewer(self):
        base = self.setup_opencode_reviewer()
        r = self.restart(base, "--check")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout), {"alive": False})

    def test_stuck_opencode_reviewer_is_killed_and_started_with_the_catch_up_brief(self):
        base = self.setup_opencode_reviewer()
        # reachable but stuck: it answers HTTP, so only a restart helps
        proc = self.fake_process(*self.opencode_argv(self.old_port), port=self.old_port)
        self.assertEqual(json.loads(self.restart(base, "--check").stdout), {"alive": True})
        (base / ".pair/inbox/reviewer/0001-writer.md").write_text("Chat from writer: queued\n")
        r = self.restart(base)
        self.assertEqual(r.returncode, 0, r.stderr)
        out = json.loads(r.stdout)
        self.assertTrue(out["restarted"])
        self.assertEqual(out["killed"], [proc.pid])
        self.assertIsNotNone(proc.wait(timeout=10))
        cfg = self.cfg(base)
        self.assertEqual(cfg["opencode"], {"port": self.stub.port, "session": "ses_restarted"})
        self.assertEqual(cfg["reviewer_generation"], 1)
        typed = [c["stdin"] for c in self.agterm_calls() if c["argv"][:2] == ["session", "type"]]
        self.assertEqual(typed[0], f"sh {base.resolve() / '.pair/reviewer-start.sh'}")
        self.assertIn(f"--port {self.stub.port}", (base / ".pair/reviewer-start.sh").read_text())
        self.assertIn("catch up", (base / ".pair/reviewer-brief.txt").read_text().lower())
        # the backlog reached the new session
        self.assertEqual([p["parts"][0]["text"] for p in self.stub.prompts()],
                         ["Chat from writer: queued"])

    def test_threads_of_the_dead_reviewer_are_closed_and_refuse_late_replies(self):
        base = self.setup_opencode_reviewer()
        tdir = base / ".pair/threads/t1"
        (tdir / "inbox/writer").mkdir(parents=True)
        key = "k" * 16
        import hashlib
        (tdir / "meta.json").write_text(json.dumps({
            "id": "t1", "topic": "retry", "files": ["a.go"], "status": "open",
            "reviewer_session": "ses_child", "pair_session": "ses_old", "briefed": True,
            "created_ms": 1, "key_sha256": hashlib.sha256(key.encode()).hexdigest()}))
        r = self.restart(base)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout)["closed_threads"], ["t1"])
        meta = json.loads((tdir / "meta.json").read_text())
        self.assertEqual(meta["status"], "closed")
        self.assertIn("restarted_ms", meta)
        r = self.run_pair("thread", "send", "--base", str(base), "--id", "t1", "--to", "writer",
                          "--key", key, "--stdin", stdin="AGREE: late")
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(list((tdir / "inbox/writer").glob("*.md")), [])

    def test_failed_launch_keeps_the_generation(self):
        base = self.setup_opencode_reviewer()
        self.stub.agents = [{"name": "main"}]
        r = self.restart(base)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("review", r.stderr)
        self.assertNotIn("reviewer_generation", self.cfg(base))

    def test_a_port_held_by_another_process_is_not_killed(self):
        base = self.setup_opencode_reviewer()
        proc = self.fake_process("some-other-server", "--port", str(self.old_port),
                                 port=self.old_port)
        r = self.restart(base)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("not the reviewer", r.stderr)
        self.assertIsNone(proc.poll())
        self.assertEqual(self.cfg(base)["opencode"]["port"], self.old_port)

    def test_two_restarts_at_once_restart_once(self):
        base = self.setup_opencode_reviewer()
        import fcntl
        env = dict(os.environ)
        env.update(self.env)
        cmd = [sys.executable, str(PAIR), "restart-reviewer", "--base", str(base),
               "--wait-seconds", "0.5"]
        with open(base / ".pair/restart.lock", "w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            procs = [subprocess.Popen(cmd, env=env, stdout=subprocess.PIPE, text=True)
                     for _ in range(2)]
            time.sleep(1)  # both have read the generation and wait for the lock
        outs = [json.loads(p.communicate(timeout=60)[0]) for p in procs]
        self.assertEqual(sorted(o["restarted"] for o in outs), [False, True])
        self.assertEqual(self.cfg(base)["reviewer_generation"], 1)

    def test_claude_reviewer_restart_keeps_the_opencode_writer(self):
        self.stub = StubOpencode()
        self.addCleanup(self.stub.close)
        base = make_base(self.base_dir, writer="opencode", port=self.stub.port,
                         session="ses_writer", agterm="SID-1")
        self.stub.sessions = [{"id": "ses_writer", "directory": str(base.resolve()),
                               "time": {"created": 5, "updated": 5}}]
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [{"sessions": [{
            "id": "SID-1", "splitForegroundShell": "zsh",
            "splitForeground": ["claude", "--flag"]}]}]}}}))
        settings = base.resolve() / ".pair/claude-reviewer-settings.json"
        proc = self.fake_process("claude", "--settings", str(settings))
        # another process that only mentions the path is not the reviewer
        bystander = self.fake_process("tail", str(settings))
        r = self.restart(base)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout)["killed"], [proc.pid])
        self.assertIsNotNone(proc.wait(timeout=10))
        self.assertIsNone(bystander.poll())
        cfg = self.cfg(base)
        self.assertEqual(cfg["opencode"], {"port": self.stub.port, "session": "ses_writer"})
        self.assertIn("--append-system-prompt", (base / ".pair/reviewer-start.sh").read_text())


@unittest.skipUnless(os.environ.get("PAIR_REAL_OPENCODE") == "1" and shutil.which("opencode"),
                     "set PAIR_REAL_OPENCODE=1 to run against a real opencode serve")
class RealOpencodeTests(PairTestCase):
    """Delivery against a real OpenCode server. The accepted prompts start real model
    turns; each one is aborted right after pair.py confirms it."""

    def serve(self, config: Path | None) -> int:
        port = free_port()
        env = dict(os.environ)
        env.pop("OPENCODE_CONFIG", None)
        if config:
            env["OPENCODE_CONFIG"] = str(config)
        proc = subprocess.Popen(["opencode", "serve", "--port", str(port)], cwd=self.base_dir,
                                env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.addCleanup(proc.wait, 10)
        self.addCleanup(proc.terminate)
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            try:
                self.api(port, "GET", "/session/status")
                return port
            except OSError:
                time.sleep(0.3)
        self.fail("opencode serve did not come up")

    def api(self, port, method, path, payload=None):
        import urllib.request
        data = None if payload is None else json.dumps(payload).encode()
        req = urllib.request.Request(f"http://127.0.0.1:{port}{path}", data=data, method=method,
                                     headers={"content-type": "application/json"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = resp.read()
        return json.loads(body) if body else None

    def paired_base(self, port):
        parent = self.api(port, "POST", "/session", {"title": "pair test main"})["id"]
        self.addCleanup(self.api, port, "DELETE", f"/session/{parent}")
        base = make_base(self.base_dir, writer="claude", port=port, session=parent)
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        return base, parent

    def stop_turns(self, port, base, parent):
        """Abort the model turns the accepted prompts started, and drop their sessions."""
        metas = [json.loads(m.read_text()) for m in base.glob(".pair/threads/*/meta.json")]
        for session in [m["reviewer_session"] for m in metas]:
            self.api(port, "POST", f"/session/{session}/abort")
            self.api(port, "DELETE", f"/session/{session}")
        self.api(port, "POST", f"/session/{parent}/abort")

    def test_a_server_without_the_reviewer_agent_fails_loudly_and_opens_no_thread(self):
        port = self.serve(None)
        base, parent = self.paired_base(port)
        r = self.run_pair("thread", "open", "--base", str(base), "--topic", "retry", "--files",
                          "a.go", "--stdin", stdin="DECISION: retry policy")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertIn("thread not opened", r.stderr)
        self.assertIn("did not take the prompt", r.stderr)
        self.assertFalse((base / ".pair/threads/t1").exists())
        children = self.api(port, "GET", f"/session/{parent}/children")
        self.assertEqual(children, [])
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin",
                          stdin="ANSWER: ping")
        self.assertEqual(r.returncode, 1)
        self.assertEqual(len(list((base / ".pair/inbox/reviewer").glob("*.md"))), 1)

    def test_a_server_with_the_reviewer_agent_takes_thread_and_main_prompts(self):
        config = self.root / "opencode-reviewer.json"
        sys.path.insert(0, str(HERE))
        import pair
        config.write_text(json.dumps(pair.reviewer_opencode_config("impl")))
        port = self.serve(config)
        base, parent = self.paired_base(port)
        try:
            self.check_thread_and_main_prompts(port, base, parent)
        finally:
            self.stop_turns(port, base, parent)

    def check_thread_and_main_prompts(self, port, base, parent):
        for topic, files in (("retry", "a.go"), ("timeouts", "b.go")):
            r = self.run_pair("thread", "open", "--base", str(base), "--topic", topic,
                              "--files", files, "--stdin", stdin=f"DECISION: {topic}")
            self.assertEqual(r.returncode, 0, r.stderr)
        metas = [json.loads((base / f".pair/threads/t{n}/meta.json").read_text()) for n in (1, 2)]
        children = {c["id"] for c in self.api(port, "GET", f"/session/{parent}/children")}
        self.assertEqual(children, {m["reviewer_session"] for m in metas})
        for meta in metas:
            stored = self.api(port, "GET", f"/session/{meta['reviewer_session']}/message")
            user = [m for m in stored if m["info"]["role"] == "user"]
            self.assertEqual(user[0]["info"]["agent"], "review")
            self.assertTrue(user[0]["parts"][0]["text"].startswith(f"PAIR THREAD {meta['id']}:"))
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin",
                          stdin="FYI: running the tests")
        self.assertEqual(r.returncode, 0, r.stderr)


FAKE_LAUNCHER = """#!/bin/sh
# a workspace new-task.sh in --print-command mode: <branch> --print-command [--single]
echo "$@" >> "$FAKE_LAUNCHER_LOG"
single=""
[ "${3:-}" = "--single" ] && single="--single"
prompt=""
[ -n "${TASK_CONTINUE:-}" ] && prompt=" \\"$TASK_CONTINUE\\""
if [ "${OPENCODE:-}" = 1 ]; then
  flags="$("$PAIR_PYTHON" "$PAIR_PY" opencode-args --base "$FAKE_BASE" $single --resume)"
  echo "zsh -lc 'opencode --auto $flags$prompt'"
else
  flags="$("$PAIR_PYTHON" "$PAIR_PY" claude-args --base "$FAKE_BASE" $single)"
  echo "zsh -lc 'claude --dangerously-skip-permissions $flags$prompt'"
fi
"""

WRITER_ID = "aaaaaaaa-0000-4000-8000-000000000001"
REVIEWER_ID = "bbbbbbbb-0000-4000-8000-000000000002"
OTHER_ID = "cccccccc-0000-4000-8000-000000000003"
NEW_SID = "11111111-2222-3333-4444-555555555555"


class StageTests(PairTestCase):
    """Plan and impl stages: the record, the flags, and the restart on the stage models."""

    def setUp(self):
        super().setUp()
        self.launcher = self.root / "new-task.sh"
        self.launcher.write_text(FAKE_LAUNCHER)
        self.launcher.chmod(self.launcher.stat().st_mode | stat.S_IEXEC)
        self.launcher_log = self.root / "launcher.log"
        self.env.update({"FAKE_LAUNCHER_LOG": str(self.launcher_log), "PAIR_PY": str(PAIR),
                         "PAIR_PYTHON": sys.executable,
                         "FAKE_BASE": str(self.base_dir.resolve())})

    def transcript(self, sid):
        path = self.root / "claude" / "projects" / "-wt" / f"{sid}.jsonl"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("{}\n")

    def cfg(self):
        return json.loads((self.base_dir / ".pair/config.json").read_text())

    def write_cfg(self, cfg):
        (self.base_dir / ".pair/config.json").write_text(json.dumps(cfg))

    def single(self):
        return json.loads((self.base_dir / ".task-stage").read_text())

    def sessions(self, *nodes):
        self.tree.write_text(json.dumps({"result": {"tree": {"workspaces": [
            {"sessions": list(nodes)}]}}}))

    def new_commands(self):
        return [c["argv"][c["argv"].index("--command") + 1] for c in self.agterm_calls()
                if c["argv"][:2] == ["session", "new"]]

    def pair_base(self, stage="plan", writer="claude"):
        self.stub = StubOpencode()
        self.addCleanup(self.stub.close)
        base = make_base(self.base_dir, writer=writer, port=self.stub.port, agterm="OLD-1",
                         stage=stage)
        cfg = self.cfg()
        cfg["roles"]["reviewer"].setdefault("agent", None)
        if writer == "claude":
            cfg["roles"]["reviewer"]["agent"] = "review"
            cfg["claude_session"] = {"writer": WRITER_ID}
            self.transcript(WRITER_ID)
        else:
            del cfg["roles"]["reviewer"]["agent"]
            cfg["opencode"]["session"] = "ses_writer"
            cfg["claude_session"] = {"reviewer": REVIEWER_ID}
            self.transcript(REVIEWER_ID)
        cfg["launcher"] = {"script": str(self.launcher), "branch": "abc-1-thing"}
        self.write_cfg(cfg)
        self.env["PAIR_FREE_PORT"] = str(self.stub.port)
        future = int(time.time() * 1000) + 10 ** 9
        self.stub.sessions = [{"id": "ses_writer" if writer == "opencode" else "ses_reviewer",
                               "directory": str(base.resolve()),
                               "time": {"created": future, "updated": future}}]
        reviewer = "opencode" if writer == "claude" else "claude"
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": [writer]},
                  {"id": NEW_SID, "name": "ABC-1 thing", "foreground": [writer],
                   "hasSplit": True, "splitForegroundShell": "zsh",
                   "splitForeground": [reviewer]})
        return base

    def stage(self, base, stage, *extra):
        return self.run_pair("stage", "--base", str(base), stage, "--foreground",
                             "--wait-seconds", "2", *extra)

    def test_models_of_each_stage(self):
        sys.path.insert(0, str(HERE))
        import pair
        self.assertEqual(pair.MODELS["plan"]["claude"], {"model": "claude-opus-5-5", "effort": "xhigh"})
        self.assertEqual(pair.MODELS["impl"]["claude"],
                         {"model": "claude-opus-5-5", "effort": "high"})
        self.assertEqual(pair.MODELS["plan"]["opencode"],
                         {"model": "openai/gpt-6-astra", "variant": "high"})
        self.assertEqual(pair.MODELS["impl"]["opencode"],
                         {"model": "openai/gpt-6.1-sol", "variant": "high"})
        self.assertEqual((pair.MODELS["plan"]["writer_agent"], pair.MODELS["impl"]["writer_agent"]),
                         ("planning", "main"))
        plan = pair.reviewer_opencode_config("plan")["agent"]
        impl = pair.reviewer_opencode_config("impl")["agent"]
        self.assertEqual(list(plan), ["review"])
        self.assertEqual((plan["review"]["model"], plan["review"]["variant"]),
                         ("openai/gpt-6-astra", "high"))
        self.assertEqual((impl["review"]["model"], impl["review"]["variant"]),
                         ("openai/gpt-6.1-sol", "high"))
        for agent in (plan["review"], impl["review"]):
            self.assertEqual((agent["mode"], agent["permission"]),
                             ("all", {"edit": "deny", "bash": "allow"}))

    def test_init_writes_plan_and_a_reopen_keeps_a_recorded_impl(self):
        self.assertEqual(self.run_pair("init", "--base", str(self.base_dir), "--writer",
                                       "claude").returncode, 0)
        self.assertEqual(self.cfg()["stage"], "plan")
        cfg = self.cfg()
        cfg["stage"] = "impl"
        cfg["claude_session"] = {"writer": WRITER_ID}
        self.write_cfg(cfg)
        self.assertEqual(self.run_pair("init", "--base", str(self.base_dir), "--writer",
                                       "claude").returncode, 0)
        self.assertEqual((self.cfg()["stage"], self.cfg()["claude_session"]),
                         ("impl", {"writer": WRITER_ID}))

    def test_a_single_session_turned_into_a_pair_keeps_its_stage(self):
        (self.base_dir / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        self.run_pair("init", "--base", str(self.base_dir), "--writer", "claude")
        self.assertEqual(self.cfg()["stage"], "impl")
        self.assertEqual(self.cfg()["roles"]["reviewer"]["agent"], "review")

    def test_unknown_stage_becomes_plan_with_the_approval_notice(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port, stage=None)
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        hook = json.dumps({"prompt": "go on", "session_id": WRITER_ID})
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer", stdin=hook)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("so it is PLAN now", r.stdout)
        self.assertIn(f"fresh --base {base.resolve()} --then impl", r.stdout)
        self.assertIn("explicitly approved PLAN.md", r.stdout)
        cfg = self.cfg()
        self.assertEqual((cfg["stage"], cfg["claude_session"]), ("plan", {"writer": WRITER_ID}))
        # the agent stays: the running reviewer server may not know the plan agent
        self.assertEqual(cfg["roles"]["reviewer"]["agent"], "review")
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer", stdin=hook)
        self.assertIn("TASK STAGE: PLAN", r.stdout)
        self.assertNotIn("so it is PLAN now", r.stdout)

    def test_status_prints_the_stage(self):
        base = make_base(self.base_dir, writer="claude", stage=None)
        out = json.loads(self.run_pair("status", "--base", str(base)).stdout)
        self.assertEqual(out["stage"], "plan")
        self.assertIn("explicitly approved PLAN.md", out["stage_note"])
        out = json.loads(self.run_pair("status", "--base", str(base)).stdout)
        self.assertEqual(out["stage"], "plan")
        self.assertNotIn("stage_note", out)

    def test_session_start_after_clear_records_the_new_conversation(self):
        base = make_base(self.base_dir, writer="opencode", port=free_port(), stage="impl")
        r = self.run_pair("session-start", "--base", str(base), "--role", "reviewer",
                          stdin=json.dumps({"source": "clear", "session_id": REVIEWER_ID}))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("TASK STAGE: IMPL", r.stdout)
        self.assertEqual(self.cfg()["claude_session"], {"reviewer": REVIEWER_ID})

    def test_claude_args_start_then_resume_on_the_model_of_the_stage(self):
        base = str(self.base_dir)
        r = self.run_pair("claude-args", "--base", base, "--launcher", str(self.launcher),
                          "--branch", "abc-1-thing")
        self.assertEqual(r.returncode, 0, r.stderr)
        rec = self.single()
        sid = rec["claude_session"]
        self.assertEqual(r.stdout.strip(), f"--session-id {sid} --model claude-opus-5-5 --effort xhigh")
        self.assertEqual(rec["stage"], "plan")
        self.assertEqual(rec["launcher"], {"script": str(self.launcher.resolve()),
                                           "branch": "abc-1-thing"})
        # recorded but never started: the id is still free
        self.assertEqual(self.run_pair("claude-args", "--base", base).stdout.strip(),
                         f"--session-id {sid} --model claude-opus-5-5 --effort xhigh")
        self.transcript(sid)
        rec = self.single()
        rec["stage"] = "impl"
        (self.base_dir / ".task-stage").write_text(json.dumps(rec))
        self.assertEqual(self.run_pair("claude-args", "--base", base).stdout.strip(),
                         f"--resume {sid} --model claude-opus-5-5 --effort high")

    def test_claude_args_of_a_pair_role_and_of_a_single_session_beside_it(self):
        make_base(self.base_dir, writer="claude", stage="impl")
        r = self.run_pair("claude-args", "--base", str(self.base_dir), "--role", "writer")
        sid = self.cfg()["claude_session"]["writer"]
        self.assertEqual(r.stdout.strip(), f"--session-id {sid} --model claude-opus-5-5 --effort high")
        r = self.run_pair("claude-args", "--base", str(self.base_dir), "--single")
        self.assertIn("--model claude-opus-5-5 --effort high", r.stdout)
        self.assertNotEqual(self.single()["claude_session"], sid)

    def test_opencode_args_name_the_stage_agent_and_the_recorded_session(self):
        base = str(self.base_dir)
        self.assertEqual(self.run_pair("opencode-args", "--base", base).stdout.strip(),
                         "--agent planning")
        (self.base_dir / ".task-stage").write_text(json.dumps(
            {"stage": "impl", "opencode_session": "ses_x"}))
        self.assertEqual(self.run_pair("opencode-args", "--base", base).stdout.strip(),
                         "--agent main --session ses_x")

    def test_stage_to_the_same_stage_changes_nothing(self):
        base = self.pair_base("plan")
        r = self.stage(base, "plan")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout), {"stage": "plan", "changed": False})
        self.assertEqual(self.agterm_calls(), [])

    def test_stage_is_refused_while_a_thread_is_open(self):
        base = self.pair_base("plan")
        thread = base / ".pair/threads/t1"
        thread.mkdir(parents=True)
        (thread / "meta.json").write_text(json.dumps({
            "id": "t1", "topic": "retry", "files": ["a.go"], "status": "open",
            "created_ms": 1}))
        r = self.stage(base, "impl")
        self.assertEqual(r.returncode, 5, r.stderr)
        self.assertIn("t1 is open", r.stderr)
        self.assertEqual(self.cfg()["stage"], "plan")
        self.assertEqual(self.agterm_calls(), [])

    def assert_refused_for_a_companion(self, base, result):
        self.assertEqual(result.returncode, 5, result.stderr)
        self.assertIn("test core ABC-1", result.stderr)
        self.assertEqual(self.cfg()["stage"], "plan")
        self.assertNotIn("fresh", self.cfg())
        argvs = [c["argv"] for c in self.agterm_calls()]
        self.assertFalse([a for a in argvs if a[:2] in (["session", "new"], ["session", "close"])])

    def test_stage_is_refused_while_a_companion_is_open(self):
        base = self.pair_base("plan")
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]},
                      {"id": "C-1", "name": "test core ABC-1", "parent": "OLD-1"})
        self.assert_refused_for_a_companion(base, self.stage(base, "impl"))

    def test_fresh_is_refused_while_a_companion_is_open(self):
        base = self.pair_base("plan")
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]},
                      {"id": "C-1", "name": "test core ABC-1", "parent": "OLD-1"})
        self.assert_refused_for_a_companion(base, self.fresh(base))

    def test_stage_without_a_launcher_is_refused(self):
        base = self.pair_base("plan")
        cfg = self.cfg()
        del cfg["launcher"]
        self.write_cfg(cfg)
        r = self.stage(base, "impl")
        self.assertEqual(r.returncode, 1)
        self.assertIn("--launcher", r.stderr)
        self.assertEqual(self.cfg()["stage"], "plan")

    def test_pair_stage_respawns_the_writer_then_launch_starts_the_reviewer(self):
        base = self.pair_base("plan")
        r = self.stage(base, "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        cfg = self.cfg()
        self.assertEqual((cfg["stage"], cfg["roles"]["reviewer"]["agent"]),
                         ("impl", "review"))
        self.assertEqual(self.launcher_log.read_text().split(), ["abc-1-thing", "--print-command"])
        self.assertEqual(self.new_commands(), [
            "zsh -lc 'claude --dangerously-skip-permissions "
            f"--resume {WRITER_ID} --model claude-opus-5-5 --effort high'"])
        argvs = [c["argv"] for c in self.agterm_calls()]
        new = next(a for a in argvs if a[:2] == ["session", "new"])
        self.assertEqual(new[:10], ["session", "new", "--cwd", str(base.resolve()), "--name",
                                    "ABC-1 thing", "--after", "OLD-1", "--no-select", "--command"])
        # the old terminal closes before the reviewer starts in the new one
        close = argvs.index(["session", "close", "--target", "OLD-1"])
        split = argvs.index(["session", "split", "on", "--target", NEW_SID])
        self.assertLess(close, split)
        start = (base / ".pair/reviewer-start.sh").read_text()
        self.assertIn(f"--agent review --port {self.stub.port} ", start)
        self.assertIn("This task was re-opened", (base / ".pair/reviewer-brief.txt").read_text())
        self.assertEqual((cfg["agterm_session"], cfg["opencode"]["session"]),
                         (NEW_SID, "ses_reviewer"))

    def test_the_stage_agent_reaches_main_prompts_thread_children_and_flushes(self):
        base = self.pair_base("plan")
        self.assertEqual(self.stage(base, "impl").returncode, 0)
        self.assertEqual(self.cfg()["roles"]["reviewer"]["agent"], "review")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        r = self.run_pair("send", "--base", str(base), "--to", "reviewer", "--stdin",
                          stdin="QUESTION: running the tests?")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("thread", "open", "--base", str(base), "--topic", "retry",
                          "--files", "a.go", "--stdin", stdin="DECISION: retry policy")
        self.assertEqual(r.returncode, 0, r.stderr)
        created = [body for method, path, body in self.stub.requests
                   if (method, path) == ("POST", "/session")]
        self.assertEqual(created[-1]["agent"], "review")
        self.assertEqual({p.get("agent") for p in self.stub.prompts()}, {"review"})
        self.assertEqual(len(self.stub.prompts()), 2)

    def test_pair_stage_with_an_opencode_writer_resumes_both_on_the_stage(self):
        base = self.pair_base("plan", writer="opencode")
        r = self.stage(base, "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.new_commands(), [
            "zsh -lc 'opencode --auto --agent main --session ses_writer'"])
        start = (base / ".pair/reviewer-start.sh").read_text()
        self.assertIn(f"--resume {REVIEWER_ID} --model claude-opus-5-5 --effort high ", start)
        self.assertNotIn("agent", self.cfg()["roles"]["reviewer"])

    def test_stage_needs_the_conversation_of_a_claude_writer(self):
        base = self.pair_base("plan")
        cfg = self.cfg()
        del cfg["claude_session"]
        self.write_cfg(cfg)
        r = self.stage(base, "impl")
        self.assertEqual(r.returncode, 1)
        self.assertIn("--session-id writer=", r.stderr)
        self.env["CLAUDE_CODE_SESSION_ID"] = WRITER_ID
        self.assertEqual(self.stage(base, "impl").returncode, 0)
        self.assertEqual(self.cfg()["claude_session"]["writer"], WRITER_ID)

    def test_single_stage_respawns_only_the_writer(self):
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "plan", "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.env["CLAUDE_CODE_SESSION_ID"] = WRITER_ID
        self.transcript(WRITER_ID)
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]})
        r = self.stage(self.base_dir, "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.single()["stage"], "impl")
        self.assertEqual(self.new_commands()[-1],
                         "zsh -lc 'claude --dangerously-skip-permissions "
                         f"--resume {WRITER_ID} --model claude-opus-5-5 --effort high'")
        self.assertFalse(any(c["argv"][:2] == ["session", "split"] for c in self.agterm_calls()))
        self.assertFalse((self.base_dir / ".pair").exists())

    def test_single_stage_restarts_detached_and_logs_it(self):
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "plan", "claude_session": WRITER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]})
        r = self.run_pair("stage", "--base", str(self.base_dir), "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        out = json.loads(r.stdout)
        self.assertEqual((out["stage"], out["restart"]), ("impl", "started"))
        log = Path(out["log"])
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline and NEW_SID not in (log.read_text() if log.exists() else ""):
            time.sleep(0.2)
        self.assertIn(NEW_SID, log.read_text())
        # detached: the old session closes at once, not from inside itself
        self.assertIn(["session", "close", "--target", "OLD-1"],
                      [c["argv"] for c in self.agterm_calls()])

    def fresh(self, base, *extra):
        return self.run_pair("fresh", "--base", str(base), "--foreground",
                             "--wait-seconds", "2", *extra)

    def test_entries_held_when_the_plan_closes_go_to_the_impl_logs(self):
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "plan", "claude_session": WRITER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.env["CLAUDE_CODE_SESSION_ID"] = WRITER_ID
        self.transcript(WRITER_ID)
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]})
        (self.base_dir / ".scratch").mkdir()
        (self.base_dir / ".scratch/plan-log-pending.json").write_text(json.dumps([
            {"kind": "qa", "entry": "### Q1 — retry\n\nheld question", "mirror": False},
            {"kind": "decision", "entry": "### main: cache\n\nmirrored", "mirror": True},
            {"kind": "decision", "entry": "### D1 — cache\n\nsingle", "mirror": False}]))
        r = self.fresh(self.base_dir, "--then", "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual((self.base_dir / ".task-qa.md").read_text(),
                         "### Q1 — retry\n\nheld question\n\n")
        self.assertEqual((self.base_dir / ".task-decisions.md").read_text(),
                         "### D1 — cache\n\nsingle\n\n")
        self.assertFalse((self.base_dir / ".scratch/plan-log-pending.json").exists())
        self.assertFalse((self.base_dir / "PLAN.md").exists())

    def fake_opencode(self):
        """An `opencode session list` that knows a newer session of the base."""
        bin_dir = self.root / "bin"
        bin_dir.mkdir(exist_ok=True)
        tool = bin_dir / "opencode"
        tool.write_text("#!/bin/sh\necho '[{\"id\": \"ses_latest\", \"directory\": \"'"
                        "\"$FAKE_BASE\"'\", \"updated\": 1}]'\n")
        tool.chmod(tool.stat().st_mode | stat.S_IEXEC)
        self.env["PATH"] = f"{bin_dir}{os.pathsep}{os.environ['PATH']}"

    def test_stage_no_longer_takes_fresh(self):
        base = self.pair_base("plan")
        self.assertEqual(self.stage(base, "plan", "--fresh").returncode, 2)
        self.assertEqual(self.agterm_calls(), [])

    def test_fresh_restarts_both_agents_in_new_conversations_for_each_purpose(self):
        sys.path.insert(0, str(HERE))
        import pair
        # (recorded stage, --then, stage after, effort): the same stage resets too, and
        # no --then keeps the recorded stage
        cases = (("plan", "plan", "plan", "xhigh"), ("impl", "impl", "impl", "high"),
                 ("plan", "impl", "impl", "high"), ("plan", "finish", "impl", "high"),
                 ("impl", "finish", "impl", "high"), ("impl", None, "impl", "high"),
                 ("impl", "reviews", "plan", "xhigh"), ("plan", "reviews", "plan", "xhigh"),
                 ("plan", None, "plan", "xhigh"))
        for recorded, then, stage, effort in cases:
            with self.subTest(recorded=recorded, then=then):
                self.log.unlink(missing_ok=True)
                base = self.pair_base(recorded)
                # a single session beside the pair never lends the writer its conversation
                (base / ".task-stage").write_text(json.dumps({"claude_session": OTHER_ID}))
                r = self.fresh(base, *(("--then", then) if then else ()))
                self.assertEqual(r.returncode, 0, r.stderr)
                cfg = self.cfg()
                sid = cfg["claude_session"]["writer"]
                self.assertNotIn(sid, (WRITER_ID, OTHER_ID))
                self.assertEqual(cfg["stage"], stage)
                self.assertNotIn("fresh", cfg)
                purpose = then or "continue"
                prompt = pair.FRESH_PROMPTS[purpose].format(decisions=".pair/decisions.md",
                                                             stage=stage.upper())
                self.assertEqual(self.new_commands(), [
                    "zsh -lc 'claude --dangerously-skip-permissions "
                    f"--session-id {sid} --model claude-opus-5-5 --effort {effort} "
                    f"\"{prompt}\"'"])
                brief = (base / ".pair/reviewer-brief.txt").read_text()
                self.assertIn(pair.FRESH_CATCH_UPS[purpose], brief)
                self.assertNotIn("This task was re-opened", brief)
                self.assertEqual((cfg["agterm_session"], cfg["opencode"]["session"]),
                                 (NEW_SID, "ses_reviewer"))

    def test_fresh_prompts_name_the_purpose_and_the_decisions_log_of_the_mode(self):
        sys.path.insert(0, str(HERE))
        import pair
        self.assertEqual(set(pair.FRESH_PROMPTS),
                         {"plan", "impl", "finish", "reviews", "continue"})
        self.assertEqual(set(pair.FRESH_CATCH_UPS), set(pair.FRESH_PROMPTS))
        starts = {"plan": "Plan discussion:", "impl": "Implementation:", "finish": "Finish:",
                  "reviews": "Plan review:", "continue": "Fresh start:"}
        for purpose, text in pair.FRESH_PROMPTS.items():
            self.assertTrue(text.startswith(starts[purpose]))
            for decisions in (".pair/decisions.md", ".task-decisions.md"):
                prompt = text.format(decisions=decisions, stage="IMPL")
                self.assertFalse(pair.SHELL_SPECIAL.intersection(prompt), purpose)
                # the router hooks key on it to skip their routing pass
                self.assertTrue(pair.FRESH_PROMPT_RE.match(prompt), purpose)
                if purpose in ("impl", "finish", "reviews"):
                    self.assertIn(decisions, prompt)
                    self.assertIn(pair.USER_DECISIONS_FILE, prompt)
                if purpose == "continue":
                    self.assertNotIn(decisions, prompt)
                    self.assertIn("stays in its IMPL stage", prompt)
                    self.assertIn("Load no skill and read nothing about the task", prompt)
                else:
                    self.assertIn(pair.TASK_SKILL_ONLY, prompt)
        for purpose, text in pair.FRESH_CATCH_UPS.items():
            self.assertFalse(pair.SHELL_SPECIAL.intersection(text), purpose)
            self.assertIn("never the old messages", text)
            self.assertIn("cleared on purpose", text)
            if purpose in ("impl", "finish", "reviews"):
                self.assertIn(pair.USER_DECISIONS_FILE, text)
        self.assertIn(pair.USER_DECISIONS_FILE, pair.CATCH_UP)
        self.assertIn(pair.USER_DECISIONS_FILE, pair.SESSION_START_NOTE)

    def test_a_later_stage_resumes_the_new_conversations(self):
        base = self.pair_base("plan")
        self.assertEqual(self.fresh(base, "--then", "plan").returncode, 0)
        sid = self.cfg()["claude_session"]["writer"]
        self.transcript(sid)
        self.sessions({"id": NEW_SID, "name": "ABC-1 thing", "foreground": ["claude"],
                       "hasSplit": True, "splitForegroundShell": "zsh",
                       "splitForeground": ["opencode"]})
        self.assertEqual(self.stage(base, "impl").returncode, 0)
        self.assertEqual(self.new_commands()[-1],
                         "zsh -lc 'claude --dangerously-skip-permissions "
                         f"--resume {sid} --model claude-opus-5-5 --effort high'")
        self.assertIn("This task was re-opened", (base / ".pair/reviewer-brief.txt").read_text())

    def test_fresh_with_an_opencode_writer_never_takes_an_old_or_the_latest_session(self):
        sys.path.insert(0, str(HERE))
        import pair
        base = self.pair_base("plan", writer="opencode")
        cfg = self.cfg()
        cfg["opencode_session"] = "ses_writer"
        self.write_cfg(cfg)
        self.fake_opencode()
        # the fake is live: without a recorded session --resume takes the newest one
        (base / ".task-stage").write_text("{}")
        self.assertEqual(self.run_pair("opencode-args", "--base", str(base), "--single",
                                       "--resume").stdout.strip(),
                         "--agent planning --session ses_latest")
        self.stub.sessions[0]["id"] = "ses_fresh"
        r = self.fresh(base, "--then", "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        prompt = pair.FRESH_PROMPTS["impl"].format(decisions=".pair/decisions.md", stage="IMPL")
        self.assertEqual(self.new_commands(), [
            f"zsh -lc 'opencode --auto --agent main \"{prompt}\"'"])
        cfg = self.cfg()
        sid = cfg["claude_session"]["reviewer"]
        self.assertNotEqual(sid, REVIEWER_ID)
        start = (base / ".pair/reviewer-start.sh").read_text()
        self.assertIn(f"--session-id {sid} --model claude-opus-5-5 --effort high ", start)
        first = start.rstrip("\n").rsplit(" \"", 1)[1].rstrip("\"")
        # the router hooks skip it, and the reviewer pair hook never forwards it
        self.assertTrue(pair.FRESH_PROMPT_RE.match(first), first)
        self.assertIn(pair.FRESH_CATCH_UPS["impl"], (base / ".pair/reviewer-brief.txt").read_text())
        self.assertEqual((cfg["stage"], cfg["opencode_session"]), ("impl", "ses_fresh"))
        self.assertNotIn("fresh", cfg)

    def test_a_later_stage_resumes_the_new_conversations_of_an_opencode_writer(self):
        base = self.pair_base("plan", writer="opencode")
        self.stub.sessions[0]["id"] = "ses_fresh"
        self.assertEqual(self.fresh(base, "--then", "plan").returncode, 0)
        sid = self.cfg()["claude_session"]["reviewer"]
        self.transcript(sid)
        self.sessions({"id": NEW_SID, "name": "ABC-1 thing", "foreground": ["opencode"],
                       "hasSplit": True, "splitForegroundShell": "zsh",
                       "splitForeground": ["claude"]})
        self.assertEqual(self.stage(base, "impl").returncode, 0)
        self.assertEqual(self.new_commands()[-1],
                         "zsh -lc 'opencode --auto --agent main --session ses_fresh'")
        self.assertIn(f"--resume {sid} --model claude-opus-5-5 --effort high ",
                      (base / ".pair/reviewer-start.sh").read_text())

    def test_fresh_of_a_single_claude_session(self):
        sys.path.insert(0, str(HERE))
        import pair
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "plan", "claude_session": WRITER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.env["CLAUDE_CODE_SESSION_ID"] = WRITER_ID
        self.transcript(WRITER_ID)
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]})
        r = self.fresh(self.base_dir, "--then", "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        rec = self.single()
        self.assertNotEqual(rec["claude_session"], WRITER_ID)
        self.assertEqual(rec["stage"], "impl")
        self.assertNotIn("fresh", rec)
        prompt = pair.FRESH_PROMPTS["impl"].format(decisions=".task-decisions.md", stage="IMPL")
        self.assertEqual(self.new_commands(), [
            "zsh -lc 'claude --dangerously-skip-permissions "
            f"--session-id {rec['claude_session']} --model claude-opus-5-5 --effort high "
            f"\"{prompt}\"'"])
        self.assertFalse(any(c["argv"][:2] == ["session", "split"] for c in self.agterm_calls()))
        self.assertFalse((self.base_dir / ".pair").exists())

    def install_auto_context_plugin(self, enabled=True):
        """An installed superpowers whose SessionStart hook loads a skill."""
        source = self.root / "superpowers-7.0.0"
        (source / "skills" / "brainstorming").mkdir(parents=True)
        (source / "hooks").mkdir()
        (source / "hooks" / "hooks.json").write_text('{"hooks": {"SessionStart": []}}')
        config = self.root / "claude"
        (config / "plugins").mkdir(parents=True, exist_ok=True)
        plugin_id = "superpowers@claude-plugins-official"
        (config / "settings.json").write_text(json.dumps({"enabledPlugins": {plugin_id: enabled}}))
        (config / "plugins" / "installed_plugins.json").write_text(json.dumps(
            {"plugins": {plugin_id: [{"installPath": str(source)}]}}))
        return source

    def test_a_fresh_claude_conversation_gets_plugin_skills_without_their_hooks(self):
        source = self.install_auto_context_plugin()
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "plan", "claude_session": WRITER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]})
        r = self.fresh(self.base_dir)
        self.assertEqual(r.returncode, 0, r.stderr)
        folder = self.base_dir.resolve() / ".task-fresh"
        copy = folder / "plugins" / "superpowers"
        self.assertIn(f"--settings \"{folder / 'claude-writer-settings.json'}\" "
                      f"--plugin-dir \"{copy}\" ", self.new_commands()[0])
        self.assertEqual(json.loads((folder / "claude-writer-settings.json").read_text()),
                         {"enabledPlugins": {"superpowers@claude-plugins-official": False}})
        self.assertEqual(json.loads((copy / ".claude-plugin/plugin.json").read_text())["name"],
                         "superpowers")
        self.assertEqual((copy / "skills").resolve(), (source / "skills").resolve())
        self.assertFalse((copy / "hooks").exists())
        # a later restart resumes that conversation with the plugin as installed
        rec = self.single()
        self.transcript(rec["claude_session"])
        self.env["CLAUDE_CODE_SESSION_ID"] = rec["claude_session"]
        self.sessions({"id": NEW_SID, "name": "ABC-1 thing", "foreground": ["claude"]})
        self.env["AGTERM_SESSION_ID"] = NEW_SID
        self.assertEqual(self.stage(self.base_dir, "impl").returncode, 0)
        self.assertNotIn("--plugin-dir", self.new_commands()[-1])

    def test_a_fresh_pair_writer_keeps_its_pair_hooks_in_the_last_settings(self):
        self.install_auto_context_plugin()
        base = self.pair_base("plan")
        self.assertEqual(self.fresh(base, "--then", "impl").returncode, 0)
        path = base.resolve() / ".task-fresh" / "claude-writer-settings.json"
        self.assertIn(f"--settings \"{path}\"", self.new_commands()[0])
        # Claude Code applies only the last --settings: this one, after the launcher's
        settings = json.loads(path.read_text())
        self.assertEqual(settings["enabledPlugins"],
                         {"superpowers@claude-plugins-official": False})
        self.assertIn("UserPromptSubmit", settings["hooks"])
        self.assertIn("PreToolUse", settings["hooks"])

    def test_a_fresh_claude_conversation_leaves_a_disabled_plugin_alone(self):
        self.install_auto_context_plugin(enabled=False)
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "impl", "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]})
        self.assertEqual(self.fresh(self.base_dir).returncode, 0)
        self.assertNotIn("--settings", self.new_commands()[0])
        self.assertFalse((self.base_dir / ".task-fresh").exists())

    def test_fresh_of_a_single_opencode_session_keeps_the_stage_and_takes_no_session(self):
        sys.path.insert(0, str(HERE))
        import pair
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "plan", "opencode_session": "ses_old",
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.fake_opencode()
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["opencode"]})
        r = self.fresh(self.base_dir)
        self.assertEqual(r.returncode, 0, r.stderr)
        prompt = pair.FRESH_PROMPTS["continue"].format(decisions=".task-decisions.md", stage="PLAN")
        self.assertEqual(self.new_commands(), [
            f"zsh -lc 'opencode --auto --agent planning \"{prompt}\"'"])
        rec = self.single()
        self.assertEqual(rec["stage"], "plan")
        self.assertNotIn("opencode_session", rec)
        self.assertNotIn("fresh", rec)

    def test_fresh_of_a_single_session_beside_a_pair_leaves_the_pair_alone(self):
        sys.path.insert(0, str(HERE))
        import pair
        base = self.pair_base("plan")
        (base / ".pair/inbox/writer/0001-reviewer.md").write_text("Chat from reviewer: hi\n")
        before = (base / ".pair/config.json").read_text()
        (base / ".task-stage").write_text(json.dumps({
            "stage": "plan", "claude_session": OTHER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.transcript(OTHER_ID)
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        r = self.fresh(base, "--single", "--then", "finish")
        self.assertEqual(r.returncode, 0, r.stderr)
        rec = self.single()
        self.assertNotIn(rec["claude_session"], (OTHER_ID, WRITER_ID))
        self.assertEqual(rec["stage"], "impl")
        self.assertNotIn("fresh", rec)
        prompt = pair.FRESH_PROMPTS["finish"].format(decisions=".task-decisions.md", stage="IMPL")
        self.assertEqual(self.new_commands(), [
            "zsh -lc 'claude --dangerously-skip-permissions "
            f"--session-id {rec['claude_session']} --model claude-opus-5-5 --effort high "
            f"\"{prompt}\"'"])
        self.assertEqual(self.launcher_log.read_text().split(),
                         ["abc-1-thing", "--print-command", "--single"])
        self.assertEqual((base / ".pair/config.json").read_text(), before)
        self.assertTrue((base / ".pair/inbox/writer/0001-reviewer.md").exists())
        self.assertFalse(any(c["argv"][:2] == ["session", "split"] for c in self.agterm_calls()))

    def test_fresh_is_refused_before_any_change(self):
        base = self.pair_base("plan")
        (base / ".pair/inbox/reviewer/0001-writer.md").write_text("Chat from writer: hi\n")
        thread = base / ".pair/threads/t1"
        thread.mkdir(parents=True)
        (thread / "meta.json").write_text(json.dumps({
            "id": "t1", "topic": "retry", "files": ["a.go"], "status": "open",
            "created_ms": 1}))
        before = (base / ".pair/config.json").read_text()
        r = self.fresh(base, "--then", "plan")
        self.assertEqual(r.returncode, 5, r.stderr)
        self.assertIn("t1 is open", r.stderr)
        (thread / "meta.json").write_text(json.dumps({
            "id": "t1", "topic": "retry", "files": ["a.go"], "status": "agreed",
            "created_ms": 1}))
        cfg = self.cfg()
        del cfg["launcher"]
        self.write_cfg(cfg)
        before_launcher = (base / ".pair/config.json").read_text()
        r = self.fresh(base, "--then", "plan")
        self.assertEqual(r.returncode, 1)
        self.assertIn("--launcher", r.stderr)
        self.assertEqual((base / ".pair/config.json").read_text(), before_launcher)
        self.assertNotEqual(before, before_launcher)
        self.assertTrue((base / ".pair/inbox/reviewer/0001-writer.md").exists())
        self.assertEqual(self.agterm_calls(), [])
        # a single session outside agterm: nothing to restart, the record stays
        self.env["AGTERM_SESSION_ID"] = ""
        single = {"stage": "impl", "claude_session": WRITER_ID,
                  "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}
        (base / ".task-stage").write_text(json.dumps(single))
        r = self.fresh(base, "--single")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no agterm session", r.stderr)
        self.assertEqual(self.single(), single)

    def assert_plan_stays_closed(self, base, *extra):
        """stage plan and fresh --then plan refuse with exit 1 and touch nothing."""
        records = [base / ".pair/config.json", base / ".task-stage"]
        before = [p.read_bytes() if p.exists() else None for p in records]
        for command in (("stage", "--base", str(base), "plan"),
                        ("fresh", "--base", str(base), "--then", "plan")):
            r = self.run_pair(*command, "--foreground", "--wait-seconds", "2", *extra)
            self.assertEqual(r.returncode, 1, r.stderr)
            self.assertIn("one-way", r.stderr)
            self.assertIn("implementation work", r.stderr)
        self.assertEqual([p.read_bytes() if p.exists() else None for p in records], before)
        self.assertEqual(self.agterm_calls(), [])

    def test_the_plan_never_reopens_from_a_pair_on_impl(self):
        self.assert_plan_stays_closed(self.pair_base("impl"))

    def test_the_plan_never_reopens_from_a_single_session_on_impl(self):
        (self.base_dir / ".task-stage").write_text(json.dumps({
            "stage": "impl", "claude_session": WRITER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.assert_plan_stays_closed(self.base_dir)
        self.assertFalse((self.base_dir / ".pair").exists())

    def test_the_plan_never_reopens_from_a_single_session_beside_a_pair_on_impl(self):
        base = self.pair_base("impl")
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.assert_plan_stays_closed(base, "--single")
        (base / ".task-stage").write_text(json.dumps({"stage": "plan"}))
        self.assert_plan_stays_closed(base, "--single")

    def test_the_refusal_beats_an_open_thread(self):
        base = self.pair_base("impl")
        thread = base / ".pair/threads/t1"
        thread.mkdir(parents=True)
        (thread / "meta.json").write_text(json.dumps({
            "id": "t1", "topic": "retry", "files": ["a.go"], "status": "open",
            "created_ms": 1}))
        self.assert_plan_stays_closed(base)

    def test_a_stale_plan_record_never_reopens_the_plan(self):
        base = self.pair_base("plan")
        (base / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        self.assert_plan_stays_closed(base)
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.assert_plan_stays_closed(base, "--single")

    def test_reviews_reopens_the_plan_in_both_records(self):
        base = self.pair_base("impl")
        (base / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        r = self.fresh(base, "--then", "reviews")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.cfg()["stage"], "plan")
        self.assertEqual(self.single()["stage"], "plan")
        self.assertEqual(self.run_pair("plan-open", "--base", str(base)).returncode, 0)
        # the reworked plan is approved again: back to IMPL
        self.sessions({"id": NEW_SID, "name": "ABC-1 thing", "foreground": ["claude"],
                       "hasSplit": True, "splitForegroundShell": "zsh",
                       "splitForeground": ["opencode"]})
        self.assertEqual(self.fresh(base, "--then", "impl").returncode, 0)
        self.assertEqual(self.cfg()["stage"], "impl")
        self.assertEqual(self.run_pair("plan-open", "--base", str(base)).returncode, 1)

    def test_reviews_reopens_a_single_session_beside_an_impl_pair(self):
        base = self.pair_base("impl")
        (base / ".task-stage").write_text(json.dumps({
            "stage": "impl", "claude_session": OTHER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        r = self.fresh(base, "--single", "--then", "reviews")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.single()["stage"], "plan")
        self.assertEqual(self.cfg()["stage"], "plan")
        self.assertIn("--effort xhigh", self.new_commands()[-1])

    def test_reviews_is_refused_before_any_change_while_a_companion_is_open(self):
        base = self.pair_base("impl")
        (base / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        self.sessions({"id": "OLD-1", "name": "ABC-1 thing", "foreground": ["claude"]},
                      {"id": "C-1", "name": "test core ABC-1", "parent": "OLD-1"})
        records = [base / ".pair/config.json", base / ".task-stage"]
        before = [p.read_bytes() for p in records]
        r = self.fresh(base, "--then", "reviews")
        self.assertEqual(r.returncode, 5, r.stderr)
        self.assertEqual([p.read_bytes() for p in records], before)

    def test_a_task_never_on_impl_keeps_its_plan_stage(self):
        base = self.pair_base("plan")
        self.assertEqual(json.loads(self.stage(base, "plan").stdout),
                         {"stage": "plan", "changed": False})
        self.assertEqual(self.fresh(base, "--then", "plan").returncode, 0)
        self.assertEqual(self.cfg()["stage"], "plan")

    def test_fresh_on_a_single_session_beside_an_impl_pair_stays_impl(self):
        base = self.pair_base("impl")
        (base / ".task-stage").write_text(json.dumps({
            "claude_session": OTHER_ID,
            "launcher": {"script": str(self.launcher), "branch": "abc-1-thing"}}))
        self.env["AGTERM_SESSION_ID"] = "OLD-1"
        self.assertEqual(self.fresh(base, "--single").returncode, 0)
        self.assertEqual(self.single()["stage"], "impl")
        self.assertIn("--effort high", self.new_commands()[-1])

    def test_a_stale_record_starts_on_the_impl_models(self):
        base = self.pair_base("plan")
        (base / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        r = self.run_pair("claude-args", "--base", str(base))
        self.assertIn("--effort high", r.stdout)
        self.assertEqual(self.cfg()["stage"], "impl")

    def test_a_pair_turned_into_a_single_session_keeps_impl(self):
        base = self.pair_base("impl")
        r = self.run_pair("claude-args", "--base", str(base), "--single")
        self.assertIn("--effort high", r.stdout)
        self.assertEqual(self.single()["stage"], "impl")

    def test_a_reopened_pair_takes_impl_from_the_single_record(self):
        base = self.pair_base("plan")
        (base / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        r = self.run_pair("init", "--base", str(base), "--writer", "claude")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.cfg()["stage"], "impl")

    def test_plan_open_is_exit_1_only_on_impl(self):
        def plan_open():
            return self.run_pair("plan-open", "--base", str(self.base_dir))
        self.assertEqual(plan_open().returncode, 0)
        (self.base_dir / ".task-stage").write_text(json.dumps({"stage": "plan"}))
        self.assertEqual(plan_open().returncode, 0)
        (self.base_dir / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        r = plan_open()
        self.assertEqual(r.returncode, 1)
        self.assertIn("one-way", r.stderr)
        (self.base_dir / ".task-stage").unlink()
        self.pair_base("impl")
        self.assertEqual(plan_open().returncode, 1)

    def script(self, name, *args, **env):
        return subprocess.run(["bash", str(HERE / name), *args], capture_output=True, text=True,
                              cwd=self.base_dir, timeout=60,
                              env={k: v for k, v in {**os.environ, **env}.items()
                                   if not k.startswith("AGTERM")})

    def test_the_plan_gate_scripts_refuse_on_impl_and_run_on_plan(self):
        plan = self.base_dir / "PLAN.md"
        plan.write_text("# Plan\n")
        record = self.base_dir / ".task-stage"
        record.write_text(json.dumps({"stage": "plan"}))
        for args in (("plan-delta",), ("plan-save",)):
            self.assertEqual(self.script("review-snapshot.sh", *args).returncode, 0, args)
        gate = ("plannotator-session.sh", "annotate", "PLAN.md", "--gate")
        self.assertEqual(self.script(*gate, PLANNOTATOR="/usr/bin/true").returncode, 0)
        shutil.rmtree(self.base_dir / ".scratch")
        record.write_text(json.dumps({"stage": "impl"}))
        for args in (("plan-delta",), ("plan-save",)):
            r = self.script("review-snapshot.sh", *args)
            self.assertEqual(r.returncode, 1, args)
            self.assertIn("one-way", r.stderr)
        for path in ("PLAN.md", "./PLAN.md", str(plan)):
            r = self.script("plannotator-session.sh", "annotate", path,
                            PLANNOTATOR="/usr/bin/true")
            self.assertEqual(r.returncode, 1, path)
            self.assertIn("one-way", r.stderr)
        self.assertFalse((self.base_dir / ".scratch/plan-review").exists())
        self.assertFalse((self.base_dir / ".scratch/plannotator.lock").exists())
        r = self.script("plannotator-session.sh", "annotate", ".scratch/decisions-review.md",
                        PLANNOTATOR="/usr/bin/true")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_the_hook_line_on_impl_closes_the_plan(self):
        base = self.pair_base("impl")
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": "add wildcards", "session_id": WRITER_ID}))
        self.assertIn("TASK STAGE: IMPL", r.stdout)
        self.assertIn("PLAN.md is closed", r.stdout)
        self.assertIn("all feedback is implementation work", r.stdout)

    def test_the_hook_line_on_plan_stays_short(self):
        base = self.pair_base("plan")
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": "drop step 3", "session_id": WRITER_ID}))
        self.assertIn("TASK STAGE: PLAN (SKILL.md, Stages).", r.stdout)
        self.assertNotIn("PLAN.md is closed", r.stdout)

    def test_the_hook_line_takes_impl_from_a_stale_record(self):
        base = self.pair_base("plan")
        (base / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": "go on", "session_id": WRITER_ID}))
        self.assertIn("TASK STAGE: IMPL", r.stdout)
        self.assertEqual(self.cfg()["stage"], "impl")

    def test_fresh_archives_the_backlog_so_the_new_agents_never_get_it(self):
        base = self.pair_base("plan")
        (base / ".pair/inbox/reviewer/0001-writer.md").write_text("Chat from writer: old\n")
        (base / ".pair/inbox/writer/0002-reviewer.md").write_text("Chat from reviewer: old\n")
        held = base / ".pair/held/reviewer"
        held.mkdir(parents=True)
        (held / "0003-writer.md").write_text("Chat from writer: FYI: running fresh\n")
        r = self.fresh(base, "--then", "impl")
        self.assertEqual(r.returncode, 0, r.stderr)
        # launch flushes an unread OpenCode backlog: nothing was left to flush
        self.assertEqual(self.stub.prompts(), [])
        for role in ("writer", "reviewer"):
            self.assertEqual(list((base / ".pair/inbox" / role).glob("*.md")), [])
        self.assertEqual(list(held.glob("*.md")), [])
        self.assertTrue((base / ".pair/inbox/writer/read/0002-reviewer.md").exists())
        archived = "".join(p.read_text() for p in (base / ".pair/inbox/reviewer/read").glob("*"))
        self.assertIn("Chat from writer: old", archived)
        self.assertIn("FYI: running fresh", archived)

    def test_reviewer_process_is_recognised_with_either_stage_agent(self):
        sys.path.insert(0, str(HERE))
        import pair
        cfg = {"roles": {"reviewer": {"kind": "opencode"}}, "opencode": {"port": 4100}}
        for agent in ("review", "review"):
            self.assertTrue(pair.is_reviewer_process(
                self.base_dir, cfg, f"opencode --auto --agent {agent} --port 4100 --prompt x"))
        self.assertFalse(pair.is_reviewer_process(
            self.base_dir, cfg, "opencode --auto --agent task --port 4100"))

    def test_claude_reviewer_launch_and_restart_use_the_stage_flags(self):
        base = self.pair_base("plan", writer="opencode")
        cfg = self.cfg()
        cfg["claude_session"] = {}
        self.write_cfg(cfg)
        r = self.run_pair("launch", "--base", str(base), "--session", NEW_SID,
                          "--wait-seconds", "2")
        self.assertEqual(r.returncode, 0, r.stderr)
        sid = self.cfg()["claude_session"]["reviewer"]
        start = (base / ".pair/reviewer-start.sh").read_text()
        self.assertIn(f"--session-id {sid} --model claude-opus-5-5 --effort xhigh ", start)
        self.transcript(sid)
        r = self.run_pair("restart-reviewer", "--base", str(base), "--wait-seconds", "0.5")
        self.assertEqual(r.returncode, 0, r.stderr)
        start = (base / ".pair/reviewer-start.sh").read_text()
        self.assertIn(f"--resume {sid} --model claude-opus-5-5 --effort xhigh ", start)

    def test_claude_args_resume_id_records_the_running_conversation(self):
        make_base(self.base_dir, writer="claude", stage="plan")
        self.transcript(WRITER_ID)
        r = self.run_pair("claude-args", "--base", str(self.base_dir), "--resume-id", WRITER_ID)
        self.assertEqual(r.stdout.strip(), f"--resume {WRITER_ID} --model claude-opus-5-5 --effort xhigh")
        self.assertEqual(self.cfg()["claude_session"]["writer"], WRITER_ID)
        r = self.run_pair("claude-args", "--base", str(self.base_dir), "--resume-id", "nope")
        self.assertEqual(r.returncode, 1)


class HeldFyiTests(PairTestCase):
    """An FYI needs no reply: it waits and goes out with the next message, as one."""

    def send(self, base, to, body):
        return self.run_pair("send", "--base", str(base), "--to", to, "--stdin", stdin=body)

    def test_fyi_answered_is_refused_and_answered_alone_goes_out(self):
        base = make_base(self.base_dir, writer="opencode")
        r = self.send(base, "reviewer", "FYI: answered: user asked x -> y")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("answer the user in chat first", r.stderr)
        r = self.send(base, "reviewer", "FYI: answered alone: user asked x -> y")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_fyis_wait_and_go_out_as_one_message_with_the_next_other_message(self):
        base = make_base(self.base_dir, writer="opencode")
        (base / ".pair" / "waiter-reviewer.pid").write_text(f"{os.getpid()}\n")
        for body in ("FYI: step 1 typed", "FYI: step 2 typed"):
            r = self.send(base, "reviewer", body)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertTrue(json.loads(r.stdout)["held"])
        self.assertEqual(list((base / ".pair/inbox/reviewer").glob("*.md")), [])
        r = self.send(base, "reviewer", "DECISION: retry policy")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("wait", "--base", str(base), "--role", "reviewer", "--timeout", "5")
        self.assertIn("PAIR MESSAGES for the reviewer (2):", r.stdout)
        out = r.stdout
        self.assertLess(out.index("Chat from writer: FYI: step 1 typed"),
                        out.index("Chat from writer: FYI: step 2 typed"))
        self.assertLess(out.index("FYI: step 2 typed"), out.index("DECISION: retry policy"))
        self.assertEqual(list((base / ".pair/held/reviewer").glob("*.md")), [])

    def test_a_full_batch_goes_to_opencode_as_one_prompt(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        self.env["PAIR_FYI_BATCH"] = "2"
        self.assertEqual(self.send(base, "reviewer", "FYI: a").returncode, 0)
        self.assertEqual(stub.prompts(), [])
        r = self.send(base, "reviewer", "FYI: b")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual([p["parts"][0]["text"] for p in stub.prompts()],
                         ["Chat from writer: FYI: a\nChat from writer: FYI: b"])

    def test_idle_releases_the_held_fyis_and_a_user_message_does_not(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        self.send(base, "reviewer", "FYI: before the user")
        r = self.run_pair("user-prompt", "--base", str(base), "--role", "writer",
                          stdin=json.dumps({"prompt": "go on"}))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(stub.prompts(), [])
        self.send(base, "reviewer", "FYI: last one")
        r = self.run_pair("idle", "--base", str(base), "--role", "writer")
        self.assertEqual(r.returncode, 0, r.stderr)
        text = stub.prompts()[-1]["parts"][0]["text"]
        self.assertIn("FYI: before the user", text)
        self.assertIn("FYI: last one", text)

    def test_flush_releases_the_held_fyis(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        self.send(base, "reviewer", "FYI: pending")
        r = self.run_pair("flush", "--base", str(base), "--to", "reviewer")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual([p["parts"][0]["text"] for p in stub.prompts()],
                         ["Chat from writer: FYI: pending"])


class EditGuardTests(PairTestCase):
    """The writer's PreToolUse hook: no edit to a file whose decision is still open."""

    def setUp(self):
        super().setUp()
        self.base = make_base(self.base_dir, writer="claude")

    def thread(self, tid, files, status):
        path = self.base / ".pair/threads" / tid
        path.mkdir(parents=True)
        (path / "meta.json").write_text(json.dumps(
            {"id": tid, "topic": f"topic {tid}", "files": files, "status": status,
             "created_ms": 0}))

    def guard(self, file_path, tool="Edit"):
        r = self.run_pair("edit-guard", "--base", str(self.base), "--role", "writer",
                          stdin=json.dumps({"tool_name": tool, "cwd": str(self.base),
                                            "tool_input": {"file_path": file_path}}))
        self.assertEqual(r.returncode, 0, r.stderr)
        return json.loads(r.stdout)["hookSpecificOutput"] if r.stdout.strip() else None

    def test_a_file_or_directory_of_an_open_thread_is_denied(self):
        self.thread("t1", ["svc/a.go"], "open")
        self.thread("t2", ["pkg/"], "open")
        for target in ("svc/a.go", str(self.base / "svc/a.go"), "pkg/new/b.go"):
            with self.subTest(target=target):
                out = self.guard(target)
                self.assertEqual(out["permissionDecision"], "deny")
                self.assertIn("still open", out["permissionDecisionReason"])

    def test_agreed_closed_unrelated_outside_and_pair_files_pass(self):
        self.thread("t1", ["svc/a.go"], "agreed")
        self.thread("t2", ["svc/b.go"], "closed")
        self.thread("t3", ["svc/c.go"], "open")
        for target in ("svc/a.go", "svc/b.go", "svc/d.go", str(self.root / "elsewhere.go"),
                       ".pair/files/view-x-writer.md"):
            with self.subTest(target=target):
                self.assertIsNone(self.guard(target))

    def test_plan_md_at_the_base_is_denied_on_impl(self):
        for target in ("PLAN.md", str(self.base / "PLAN.md")):
            with self.subTest(target=target):
                out = self.guard(target, tool="Write")
                self.assertEqual(out["permissionDecision"], "deny")
                self.assertIn("PLAN.md is closed", out["permissionDecisionReason"])
        self.assertIsNone(self.guard("repo/docs/PLAN.md"))

    def test_plan_md_is_denied_when_only_the_single_record_is_on_impl(self):
        make_base(self.base_dir, writer="claude", stage="plan")
        (self.base / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        self.assertEqual(self.guard("PLAN.md")["permissionDecision"], "deny")

    def test_plan_md_passes_on_plan(self):
        make_base(self.base_dir, writer="claude", stage="plan")
        self.assertIsNone(self.guard("PLAN.md"))

    def test_bad_input_never_blocks(self):
        r = self.run_pair("edit-guard", "--base", str(self.base), "--role", "writer",
                          stdin="not json")
        self.assertEqual((r.returncode, r.stdout), (0, ""))

    def test_only_the_writer_gets_the_hook(self):
        sys.path.insert(0, str(HERE))
        import pair
        self.assertIn("PreToolUse", pair.claude_settings(self.base, "writer")["hooks"])
        self.assertNotIn("PreToolUse", pair.claude_settings(self.base, "reviewer")["hooks"])


class DecisionLogTests(PairTestCase):
    def test_settled_threads_and_main_channel_decisions_go_to_one_log(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        r = self.run_pair("thread", "open", "--base", str(base), "--topic", "retry",
                          "--files", "a.go", "--stdin", stdin="DECISION: retry policy")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("thread", "close", "--base", str(base), "--id", "t1", "--status",
                          "agreed", "--stdin", stdin="Retry 3 times in a.go Fetch.")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("thread", "close", "--base", str(base), "--id", "t1", "--status",
                          "closed")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("decided", "--base", str(base), "--topic", "cache key", "--stdin",
                          stdin="Key on tenant and id.")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual((base / ".pair/decisions.md").read_text(),
                         "## t1 agreed: retry [a.go]\n\nRetry 3 times in a.go Fetch.\n\n"
                         "## main: cache key\n\nKey on tenant and id.\n\n")

    def test_user_decided_works_in_a_single_session(self):
        date = time.strftime("%Y-%m-%d")
        for words in ("Use a wildcard for the name.", "  Keep tags as an array.\n"):
            r = self.run_pair("user-decided", "--base", str(self.base_dir), "--stdin", stdin=words)
            self.assertEqual(r.returncode, 0, r.stderr)
        self.assertFalse((self.base_dir / ".pair").exists())
        self.assertEqual((self.base_dir / ".task-user-decisions.md").read_text(),
                         f"### {date}\n\nUse a wildcard for the name.\n\n"
                         f"### {date}\n\nKeep tags as an array.\n\n")

    def test_user_decided_keeps_long_words_whole(self):
        words = "x" * 5000
        r = self.run_pair("user-decided", "--base", str(self.base_dir), "--stdin", stdin=words)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn(words, (self.base_dir / ".task-user-decisions.md").read_text())

    def test_user_decided_refuses_empty_words(self):
        r = self.run_pair("user-decided", "--base", str(self.base_dir), "--stdin", stdin=" \n")
        self.assertEqual(r.returncode, 1)
        self.assertFalse((self.base_dir / ".task-user-decisions.md").exists())

    def test_user_decided_stays_out_of_git(self):
        subprocess.run(["git", "init", "-q", str(self.base_dir)], check=True)
        r = self.run_pair("user-decided", "--base", str(self.base_dir), "--stdin", stdin="Yes.")
        self.assertEqual(r.returncode, 0, r.stderr)
        status = subprocess.run(["git", "-C", str(self.base_dir), "status", "--porcelain"],
                                capture_output=True, text=True, check=True)
        self.assertEqual(status.stdout, "")


PLAN_TEXT = ("# Plan\n\n## Not requested\n\nnone\n\n## Q&A\n\n"
             "## Decisions\n\n### U99 — scope (2026-01-01)\n\n"
             "- asked: 2026-01-01 (chat): “keep x”\n- decided: I decided to keep x.\n"
             "- done: not done yet: S1\n\n"
             "### D1 — planner choice (2026-01-01, PLAN, main)\n\nx\n\n"
             "### t98 agreed: cache [a.go]\n\ny\n\n"
             "## Steps\n\n```\n## not a heading\n```\n")


class PlanLogTests(PairTestCase):
    """Q&A and model decisions: PLAN.md on PLAN, the task logs on IMPL (SKILL.md, Q&A log)."""

    def plan(self, text=PLAN_TEXT):
        (self.base_dir / "PLAN.md").write_text(text)

    def qa(self, topic, body):
        return self.run_pair("qa", "--base", str(self.base_dir), "--topic", topic, "--stdin",
                             stdin=body)

    def section(self, heading):
        text = (self.base_dir / "PLAN.md").read_text()
        start = text.index(heading + "\n")
        nxt = text.find("\n## ", start + len(heading))
        return text[start:nxt if nxt >= 0 else len(text)]

    def open_gate(self):
        lock = self.base_dir / ".scratch/plannotator.lock"
        lock.mkdir(parents=True)
        (lock / "args").write_text("annotate PLAN.md --gate")
        self.env["PLANNOTATOR"] = shutil.which("true")
        return lock

    def test_qa_on_plan_goes_to_the_qa_section_with_numbered_entries(self):
        self.plan()
        date = time.strftime("%Y-%m-%d")
        for topic, body in (("scope", "- question: why?\n- answer: because"),
                            ("retry", "- question: how many?\n- writer: 3\n- reviewer: 5\n"
                                      "- merged: 3\n- result: agreed")):
            r = self.qa(topic, body)
            self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.section("## Q&A"),
                         f"## Q&A\n\n### Q1 — scope ({date}, PLAN)\n\n- question: why?\n"
                         f"- answer: because\n\n### Q2 — retry ({date}, PLAN)\n\n"
                         "- question: how many?\n- writer: 3\n- reviewer: 5\n- merged: 3\n"
                         "- result: agreed\n")
        self.assertTrue((self.base_dir / "PLAN.md").read_text().endswith(
            "## Steps\n\n```\n## not a heading\n```\n"))

    def test_a_missing_section_is_added_and_a_fenced_heading_is_no_boundary(self):
        self.plan("# Plan\n\n```\n## Q&A\n```\n")
        r = self.qa("scope", "body")
        self.assertEqual(r.returncode, 0, r.stderr)
        text = (self.base_dir / "PLAN.md").read_text()
        self.assertTrue(text.startswith("# Plan\n\n```\n## Q&A\n```\n\n## Q&A\n\n### Q1 — scope"),
                        text)

    def test_qa_before_plan_md_exists_is_held_then_flushed(self):
        r = self.qa("ticket", "- question: which env?\n- answer: prod")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("plan-log-pending.json", json.loads(r.stdout)["logged"])
        self.plan()
        r = self.run_pair("plan-log-flush", "--base", str(self.base_dir))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(json.loads(r.stdout), {"written": 1, "held": 0})
        self.assertIn("### Q1 — ticket", self.section("## Q&A"))
        self.assertFalse((self.base_dir / ".scratch/plan-log-pending.json").exists())

    def test_an_open_plan_gate_holds_entries_in_order(self):
        self.plan()
        lock = self.open_gate()
        for topic in ("first", "second"):
            self.assertEqual(self.qa(topic, "body").returncode, 0)
        self.assertEqual((self.base_dir / "PLAN.md").read_text(), PLAN_TEXT)
        r = self.run_pair("plan-log-flush", "--base", str(self.base_dir))
        self.assertEqual(json.loads(r.stdout), {"written": 0, "held": 2})
        shutil.rmtree(lock)
        self.assertEqual(self.qa("third", "body").returncode, 0)
        section = self.section("## Q&A")
        self.assertLess(section.index("### Q1 — first"), section.index("### Q2 — second"))
        self.assertLess(section.index("### Q2 — second"), section.index("### Q3 — third"))

    def test_qa_on_impl_goes_to_the_task_log_and_continues_the_numbers(self):
        self.plan(PLAN_TEXT.replace("## Q&A\n\n", "## Q&A\n\n### Q1 — old (2026-01-01, PLAN)\n\nx\n\n"))
        (self.base_dir / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        r = self.qa("rename", "- question: ok?\n- answer: yes")
        self.assertEqual(r.returncode, 0, r.stderr)
        date = time.strftime("%Y-%m-%d")
        self.assertEqual((self.base_dir / ".task-qa.md").read_text(),
                         f"### Q2 — rename ({date}, IMPL)\n\n- question: ok?\n- answer: yes\n\n")
        self.assertNotIn("rename", (self.base_dir / "PLAN.md").read_text())

    def test_model_decided_routes_by_stage_and_stays_above_reviews(self):
        self.plan()
        date = time.strftime("%Y-%m-%d")
        r = self.run_pair("model-decided", "--base", str(self.base_dir), "--topic", "cache",
                          "--by", "main", "--stdin",
                          stdin="- decided: I decided to use lru\n- done: not done yet")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn(f"### D2 — cache ({date}, PLAN, main)\n\n- decided: I decided to use lru\n"
                      "- done: not done yet\n",
                      self.section("## Decisions"))
        self.assertNotIn("## Model decisions", (self.base_dir / "PLAN.md").read_text())
        (self.base_dir / ".task-stage").write_text(json.dumps({"stage": "impl"}))
        (self.base_dir / ".task-decisions.md").write_text(
            "## Reviews\n\n- 2026-01-01 — D1 — kept\n")
        r = self.run_pair("model-decided", "--base", str(self.base_dir), "--topic", "retry",
                          "--by", "fixer", "--stdin",
                          stdin="- decided: retry 3 times\n- done: fetch.go:Fetch")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual((self.base_dir / ".task-decisions.md").read_text(),
                         f"### D3 — retry ({date}, IMPL, fixer)\n\n- decided: retry 3 times\n"
                         "- done: fetch.go:Fetch\n\n"
                         "## Reviews\n\n- 2026-01-01 — D1 — kept\n")

    def test_a_pair_decision_on_plan_is_mirrored_into_plan_md(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port, stage="plan")
        self.plan()
        r = self.run_pair("decided", "--base", str(base), "--topic", "cache key", "--stdin",
                          stdin="- decided: key on tenant and id\n- done: not done yet")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("### main: cache key\n\n- decided: key on tenant and id\n- done: not done yet\n",
                      self.section("## Decisions"))
        self.assertEqual((base / ".pair/decisions.md").read_text(),
                         "## main: cache key\n\n- decided: key on tenant and id\n- done: not done yet\n\n")

    def test_a_held_decision_is_flushed_into_decisions(self):
        self.plan()
        lock = self.open_gate()
        r = self.run_pair("model-decided", "--base", str(self.base_dir), "--topic", "cache",
                          "--by", "main", "--stdin",
                          stdin="- decided: I decided to use lru\n- done: not done yet")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual((self.base_dir / "PLAN.md").read_text(), PLAN_TEXT)
        shutil.rmtree(lock)
        r = self.run_pair("plan-log-flush", "--base", str(self.base_dir))
        self.assertEqual(json.loads(r.stdout), {"written": 1, "held": 0})
        self.assertIn("### D2 — cache (", self.section("## Decisions"))
        self.assertNotIn("## Model decisions", (self.base_dir / "PLAN.md").read_text())

    def test_qa_refuses_an_empty_entry(self):
        self.plan()
        r = self.qa("scope", " \n")
        self.assertEqual(r.returncode, 1)
        self.assertEqual((self.base_dir / "PLAN.md").read_text(), PLAN_TEXT)
