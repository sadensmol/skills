#!/usr/bin/env python3
"""Run one MCP with settings from the active agterm workspace's skills."""

import json
import os
from pathlib import Path
import re
import subprocess
import sys


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
    workspace = next((w["name"] for w in workspaces if w.get("id") == workspace_id), "")
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", workspace):
        raise ValueError("cannot resolve the active workspace's skills namespace")
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
