#!/usr/bin/env python3
"""Run one MCP with settings from the active agterm workspace.

The settings come from the workspace's local registry first: the `.workspace/` folder above a
session of that agterm workspace whose settings.json names it (`agterm_workspace`), key
`opencode.mcp.<server>` of its workspace.json. Else from the workspace's private skills:
~/.agents/skills/<workspace>/<server>/opencode.json, key `mcp.<server>`.
"""

import json
import os
from pathlib import Path
import re
import subprocess
import sys


def private(path):
    st = path.stat()
    return st.st_uid == os.getuid() and st.st_mode & 0o077 == 0


def trusted(folder):
    local = folder / ".workspace"
    files = [local / "settings.json", local / "workspace.json"]
    if not all(p.is_file() and not p.is_symlink() for p in files) or local.is_symlink():
        return False
    if not all(private(p) for p in [local, *files]):
        return False
    tracked = subprocess.run(
        ["git", "-C", str(folder), "ls-files", "--error-unmatch", "--", ".workspace"],
        capture_output=True, timeout=10,
    )
    return tracked.returncode != 0


def registry_folder(start, workspace):
    folder = Path(start).resolve()
    for candidate in (folder, *folder.parents):
        registry = candidate / ".workspace" / "settings.json"
        if registry.is_file():
            if not trusted(candidate):
                return None
            if json.loads(registry.read_text()).get("agterm_workspace") == workspace:
                return candidate
            return None
    return None


def local_settings(node, workspace, server):
    sessions = node.get("sessions", [])
    own = os.environ.get("AGTERM_SESSION_ID")
    cwds = [s.get("cwd") for s in sessions if s.get("id") == own] + [s.get("cwd") for s in sessions]
    for cwd in filter(None, cwds):
        folder = registry_folder(cwd, workspace)
        if folder is None:
            continue
        settings = folder / ".workspace" / "workspace.json"
        config = json.loads(settings.read_text()).get("opencode", {}).get("mcp", {}).get(server)
        if config is not None:
            return settings, config
    return None, None


def main():
    if len(sys.argv) != 2 or not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", sys.argv[1]):
        raise ValueError("usage: workspace-mcp.py <server-name>")
    server = sys.argv[1]
    workspace_id = os.environ.get("AGTERM_WORKSPACE_ID")
    if not workspace_id:
        raise ValueError("workspace MCP requires an active agterm workspace")
    tree_command = ["agtermctl", "tree", "--json"]
    if os.environ.get("AGTERM_SOCKET"):
        tree_command.extend(["--socket", os.environ["AGTERM_SOCKET"]])
    result = subprocess.run(
        tree_command,
        check=True, capture_output=True, text=True, timeout=10,
    )
    workspaces = json.loads(result.stdout).get("result", {}).get("tree", {}).get("workspaces", [])
    node = next((w for w in workspaces if w.get("id") == workspace_id), {})
    workspace = node.get("name", "")
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", workspace):
        raise ValueError("cannot resolve the active workspace's skills namespace")
    settings, config = local_settings(node, workspace, server)
    if config is None:
        settings = Path.home() / ".agents" / "skills" / workspace / server / "opencode.json"
        config = json.loads(settings.read_text())["mcp"][server]
    command = config.get("command")
    if config.get("type") != "local" or config.get("enabled") is False:
        raise ValueError(f"{settings}: expected an enabled local MCP")
    if not isinstance(command, list) or not command or not all(isinstance(arg, str) and arg for arg in command):
        raise ValueError(f"{settings}: expected a non-empty command array")
    environment = os.environ.copy()
    for key, value in config.get("environment", {}).items():
        environment[key] = re.sub(r"\{env:([A-Za-z_][A-Za-z0-9_]*)\}",
                                  lambda match: os.environ[match[1]], value)
    print(f"MCP {server}: workspace {workspace}", file=sys.stderr)
    os.execvpe(command[0], command, environment)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        print(f"workspace MCP: {error}", file=sys.stderr)
        sys.exit(1)
