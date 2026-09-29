"""Tests for pair.py. Run: python3 -m unittest -v skills/task/scripts/test_pair.py"""

import http.server
import json
import os
import re
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
        self.sessions = []
        self.created = 0
        self.fail_prompts = False
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
                self._reply(200, stub.sessions if self.path == "/session" else {})

            def do_POST(self):
                size = int(self.headers.get("content-length") or 0)
                body = json.loads(self.rfile.read(size)) if size else None
                stub.requests.append(("POST", self.path, body))
                if self.path == "/session":
                    stub.created += 1
                    self._reply(200, {"id": "ses_new" if stub.created == 1
                                      else f"ses_new{stub.created}"})
                elif self.path.endswith("/prompt_async"):
                    self._reply(500 if stub.fail_prompts else 204, None)
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
        self.assertEqual(stub.requests, [
            ("POST", "/tui/select-session", {"sessionID": "ses_test"}),
            ("POST", "/session/ses_test/prompt_async", {
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
        self.assertIn(f"wait --base {base.resolve()} --role writer", r.stdout)
        self.assertIn("put the reviewer pane back on the pair session", r.stdout)
        old.communicate(timeout=10)
        self.assertEqual(old.returncode, 0)
        self.assertIn(("POST", "/tui/select-session", {"sessionID": "ses_test"}), stub.requests)

    def test_session_start_outside_pair_mode_is_silent(self):
        r = self.run_pair("session-start", "--role", "writer", cwd=str(self.base_dir))
        self.assertEqual((r.returncode, r.stdout), (0, ""))


class UserPromptHookTests(PairTestCase):
    def hook(self, base, role, prompt):
        return self.run_pair("user-prompt", "--base", str(base), "--role", role,
                             stdin=json.dumps({"prompt": prompt, "hook_event_name": "UserPromptSubmit"}))

    def test_writer_prompt_goes_verbatim_to_the_opencode_reviewer_and_reminds_the_writer(self):
        stub = StubOpencode()
        self.addCleanup(stub.close)
        base = make_base(self.base_dir, writer="claude", port=stub.port)
        r = self.hook(base, "writer", "why not subscribe in the constructor? you think otherwise?")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(stub.prompts(), [{
            "parts": [{"type": "text", "text": "The user said to the writer: why not subscribe "
                                               "in the constructor? you think otherwise?"}],
            "agent": "pair-reviewer"}])
        self.assertIn("PAIR MODE: pair.py forwarded this user message verbatim to the reviewer.",
                      r.stdout)
        self.assertIn("VIEW: user asked:", r.stdout)
        self.assertNotIn("{", r.stdout)

    def test_reviewer_prompt_is_queued_for_the_claude_writer(self):
        base = make_base(self.base_dir, writer="claude")
        (base / ".pair" / "waiter-writer.pid").write_text(f"{os.getpid()}\n")
        r = self.hook(base, "reviewer", "is this right?")
        self.assertEqual(r.returncode, 0, r.stderr)
        r = self.run_pair("wait", "--base", str(base), "--role", "writer", "--timeout", "5")
        self.assertIn("The user said to the reviewer: is this right?", r.stdout)

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

    def test_failure_never_blocks_the_prompt_and_says_so(self):
        r = self.hook(self.base_dir, "writer", "hello")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("could NOT forward", r.stdout)

    def test_unreachable_opencode_peer_keeps_the_message_queued(self):
        base = make_base(self.base_dir, writer="claude", port=free_port())
        r = self.hook(base, "writer", "hello")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("queued this user message for the reviewer", r.stdout)
        self.assertIn("Do not send it again", r.stdout)
        self.assertEqual(len(list((base / ".pair/inbox/reviewer").glob("*.md"))), 1)

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
        hook = json.loads((self.base_dir / ".pair/claude-writer-settings.json").read_text())
        [entry] = hook["hooks"]["UserPromptSubmit"]
        self.assertIn(" user-prompt --base ", entry["hooks"][0]["command"])
        self.assertTrue(entry["hooks"][0]["command"].endswith("--role writer"))
        self.assertFalse((self.base_dir / ".pair/claude-reviewer-settings.json").exists())
        [start] = hook["hooks"]["SessionStart"]
        self.assertEqual(start["matcher"], "clear")
        self.assertIn(" session-start --base ", start["hooks"][0]["command"])
        oc = json.loads((self.base_dir / ".pair/opencode-reviewer.json").read_text())
        self.assertEqual(oc["agent"]["pair-reviewer"]["permission"], {"edit": "deny", "bash": "allow"})
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
        self.assertIn(f"wait --base {base} --role writer", writer)
        self.assertIn("run_in_background", writer)
        self.assertIn("READ-ONLY", reviewer)
        self.assertIn("Every question of the user is discussed", writer)
        self.assertIn("send the reviewer VIEW: user asked", writer)
        self.assertIn("send the writer VIEW: user asked", reviewer)
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
        self.assertIn("OPENCODE_CONFIG=", line)
        self.assertIn(f"opencode --auto --agent pair-reviewer --port {self.stub.port} --prompt ", line)
        self.assertIn("reviewer-brief.txt", line)
        self.assertEqual(enter, "\n")
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
        line, _ = self.typed()
        self.assertTrue(line.startswith(
            "claude --dangerously-skip-permissions --disallowedTools Edit Write NotebookEdit "))
        self.assertIn("--settings ", line)
        self.assertIn("claude-reviewer-settings.json", line)
        self.assertIn("reviewer-brief.txt", line)
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
        r = self.respawn("SOMEONE-ELSE")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.strip(), "11111111-2222-3333-4444-555555555555")
        argvs = [c["argv"] for c in self.agterm_calls()]
        self.assertEqual(argvs[0], ["session", "new", "--cwd", str(self.base_dir), "--name", "T-1 thing",
                                    "--after", "OLD-1", "--command", "zsh -lc 'claude --continue'"])
        self.assertIn(["session", "close", "--target", "OLD-1"], argvs)

    def test_respawn_from_inside_the_old_session_closes_it_later(self):
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
        self.assertIn(("POST", "/session", {"parentID": "ses_test", "agent": "pair-reviewer",
                                            "title": "pair t1: retry"}), self.stub.requests)
        [prompt] = self.stub.prompts()
        self.assertEqual(prompt["agent"], "pair-reviewer")
        text = prompt["parts"][0]["text"]
        self.assertTrue(text.startswith("PAIR THREAD t1: you are the REVIEWER"))
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
        self.assertEqual(self.stub.requests[-1][1], "/session/ses_new/prompt_async")

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

    def test_the_brief_goes_with_the_first_delivery_after_a_failed_prompt(self):
        self.stub.fail_prompts = True
        r = self.open_thread()
        self.assertEqual(r.returncode, 1)
        self.assertIn("t1", r.stderr)
        self.stub.fail_prompts = False
        self.stub.requests.clear()
        r = self.run_pair("thread", "send", "--base", str(self.base), "--id", "t1", "--to",
                          "reviewer", "--stdin", stdin="second")
        self.assertEqual(r.returncode, 0, r.stderr)
        texts = [p["parts"][0]["text"] for p in self.stub.prompts()]
        self.assertEqual(len(texts), 2)
        self.assertTrue(texts[0].startswith("PAIR THREAD t1:"))
        self.assertTrue(texts[0].endswith("Chat from writer [t1]: PROPOSAL: add a retry"))
        self.assertEqual(texts[1], "Chat from writer [t1]: second")
        self.assertEqual(len([t for t in texts if "PAIR THREAD" in t]), 1)

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
        method, path, body = self.stub.requests[-1]
        self.assertEqual(path, "/session/ses_new/prompt_async")
        self.assertEqual(body["parts"][0]["text"],
                         "Chat from writer [t1]: PROPOSAL: the retry also needs a jitter")
        self.assertEqual(self.reply("t1", "AGREE: jitter is fine").returncode, 0)

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
        self.assertEqual(oc["agent"]["pair-reviewer"]["mode"], "all")

    def test_writer_brief_names_threads_only_with_an_opencode_reviewer(self):
        r = self.run_pair("brief", "--base", str(self.base), "--role", "writer")
        self.assertIn("thread open", r.stdout)
        oc = self.root / "oc"
        oc.mkdir()
        make_base(oc, writer="opencode")
        r = self.run_pair("brief", "--base", str(oc), "--role", "writer")
        self.assertNotIn("thread open", r.stdout)


if __name__ == "__main__":
    unittest.main()
