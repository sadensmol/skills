#!/usr/bin/env python3
"""Create and inspect a workspace folder's local .workspace/ (never committed).

Usage:
  workspace.py init --folder DIR --name NAME --email EMAIL --user-name "FULL NAME"
      create DIR/.workspace/ (700) with settings.json, workspace.json, private-names.txt and
      projects/ (files 600); register the Linear MCP server linear-NAME in DIR/.mcp.json and
      enable it in DIR/.claude/settings.local.json; add .workspace/ to .git/info/exclude when
      DIR is a git repository. Existing files and keys are kept, never overwritten.
  workspace.py set --folder DIR [--file settings|workspace] <dotted.key> <value>
      set one key of DIR/.workspace/settings.json (default) or workspace.json
      (value parsed as JSON when it parses)
  workspace.py status --folder DIR
      one line per setup item: "ok <item>" or "missing <item>"; exit 0 either way

Values are never printed; only key names and states. Exit 1 on any error, with the reason.
"""

import argparse
import json
import os
import subprocess
import sys
import tempfile

PHASES = ["todo", "planning", "implementing", "qa", "review", "pr", "blocked", "done", "canceled"]
LINEAR_URL = "https://mcp.linear.app/mcp"


def fail(msg):
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(1)


def read_json(path, default):
    if not os.path.exists(path):
        return default
    try:
        with open(path) as f:
            return json.load(f)
    except json.JSONDecodeError as e:
        fail(f"{path} is not valid JSON: {e}")


def write_json(path, data, mode):
    d = os.path.dirname(path)
    fd, tmp = tempfile.mkstemp(dir=d, prefix=".tmp.", suffix=".json")
    with os.fdopen(fd, "w") as f:
        json.dump(data, f, indent=2)
        f.write("\n")
    os.chmod(tmp, mode)
    os.replace(tmp, path)


def keep(obj, dotted, value):
    keys = dotted.split(".")
    for k in keys[:-1]:
        obj = obj.setdefault(k, {})
    obj.setdefault(keys[-1], value)


def lookup(obj, dotted):
    for k in dotted.split("."):
        if not isinstance(obj, dict) or k not in obj:
            return None
        obj = obj[k]
    return obj


def paths(folder):
    ws = os.path.join(folder, ".workspace")
    return {
        "ws": ws,
        "settings": os.path.join(ws, "settings.json"),
        "workspace": os.path.join(ws, "workspace.json"),
        "names": os.path.join(ws, "private-names.txt"),
        "projects": os.path.join(ws, "projects"),
        "mcp": os.path.join(folder, ".mcp.json"),
        "claude_local": os.path.join(folder, ".claude", "settings.local.json"),
    }


def cmd_init(a):
    folder = os.path.abspath(os.path.expanduser(a.folder))
    os.makedirs(folder, exist_ok=True)
    p = paths(folder)
    os.makedirs(p["ws"], mode=0o700, exist_ok=True)
    os.chmod(p["ws"], 0o700)
    os.makedirs(p["projects"], mode=0o700, exist_ok=True)

    server = f"linear-{a.name}"
    auth = os.path.join("~", ".mcp-auth", a.name)

    settings = read_json(p["settings"], {})
    keep(settings, "company", a.name)
    keep(settings, "agterm_workspace", a.name)
    write_json(p["settings"], settings, 0o600)

    ws = read_json(p["workspace"], {})
    keep(ws, "user.email", a.email)
    keep(ws, "user.name", a.user_name)
    keep(ws, "linear.server", server)
    keep(ws, "linear.authStore", auth)
    keep(ws, "linear.mcpConfig", p["mcp"])
    keep(ws, "browser.server", f"playwright-{a.name}")
    keep(ws, "opencode.mcp.linear", {
        "type": "local",
        "command": ["npx", "-y", "mcp-remote", LINEAR_URL, "--auth-timeout", "600"],
        "environment": {"MCP_REMOTE_CONFIG_DIR": "{env:HOME}/.mcp-auth/" + a.name},
        "enabled": True,
    })
    write_json(p["workspace"], ws, 0o600)

    names = []
    if os.path.exists(p["names"]):
        with open(p["names"]) as f:
            names = [line.strip() for line in f if line.strip()]
    if a.name not in names:
        names.append(a.name)
    with open(p["names"], "w") as f:
        f.write("\n".join(names) + "\n")
    os.chmod(p["names"], 0o600)

    mcp = read_json(p["mcp"], {})
    mcp.setdefault("mcpServers", {}).setdefault(server, {
        "type": "stdio",
        "command": "npx",
        "args": ["-y", "mcp-remote", LINEAR_URL, "--auth-timeout", "600"],
        "env": {"MCP_REMOTE_CONFIG_DIR": os.path.expanduser(auth)},
    })
    write_json(p["mcp"], mcp, 0o644)

    os.makedirs(os.path.dirname(p["claude_local"]), exist_ok=True)
    local = read_json(p["claude_local"], {})
    enabled = local.setdefault("enabledMcpjsonServers", [])
    if server not in enabled:
        enabled.append(server)
    write_json(p["claude_local"], local, 0o644)

    git = subprocess.run(["git", "-C", folder, "rev-parse", "--git-dir"], capture_output=True, text=True)
    if git.returncode == 0:
        exclude = os.path.join(folder, git.stdout.strip(), "info", "exclude")
        os.makedirs(os.path.dirname(exclude), exist_ok=True)
        lines = open(exclude).read().splitlines() if os.path.exists(exclude) else []
        if ".workspace/" not in lines:
            with open(exclude, "a") as f:
                f.write(".workspace/\n")
    print("OK")


def cmd_set(a):
    path = paths(os.path.abspath(os.path.expanduser(a.folder)))[a.file]
    if not os.path.exists(path):
        fail(f"{path} is missing; run init first")
    settings = read_json(path, {})
    try:
        value = json.loads(a.value)
    except json.JSONDecodeError:
        value = a.value
    obj = settings
    keys = a.key.split(".")
    for k in keys[:-1]:
        obj = obj.setdefault(k, {})
    obj[keys[-1]] = value
    write_json(path, settings, 0o600)
    print(f"set {a.key}")


def cmd_status(a):
    folder = os.path.abspath(os.path.expanduser(a.folder))
    p = paths(folder)

    def line(ok, item):
        print(f"{'ok' if ok else 'missing'} {item}")

    line(os.path.isdir(p["ws"]) and (os.stat(p["ws"]).st_mode & 0o077) == 0, ".workspace/ (mode 700)")
    ws = read_json(p["workspace"], {})
    for key in ("user.email", "user.name", "linear.server", "linear.authStore", "linear.mcpConfig", "browser.server"):
        line(bool(lookup(ws, key)), f"workspace.json {key}")
    server = lookup(ws, "linear.server")
    mcp = read_json(p["mcp"], {})
    line(bool(server) and server in mcp.get("mcpServers", {}), ".mcp.json Linear server")
    s = read_json(p["settings"], {})
    for key in ("company", "agterm_workspace", "slack.workspace", "linear.workspace", "linear.team", "linear.default_label"):
        line(bool(lookup(s, key)), f"settings.json {key}")
    for ph in PHASES:
        line(bool(lookup(s, f"linear.statuses.{ph}")), f"settings.json linear.statuses.{ph}")


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    i = sub.add_parser("init")
    i.add_argument("--folder", required=True)
    i.add_argument("--name", required=True)
    i.add_argument("--email", required=True)
    i.add_argument("--user-name", required=True)
    s = sub.add_parser("set")
    s.add_argument("--folder", required=True)
    s.add_argument("--file", choices=["settings", "workspace"], default="settings")
    s.add_argument("key")
    s.add_argument("value")
    st = sub.add_parser("status")
    st.add_argument("--folder", required=True)
    a = ap.parse_args()
    {"init": cmd_init, "set": cmd_set, "status": cmd_status}[a.cmd](a)


if __name__ == "__main__":
    main()
