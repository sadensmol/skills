#!/usr/bin/env python3
"""Read and update the workspace registry in <workspace folder>/.workspace/ (local, never committed):

  settings.json            the workspace: its name (key `company`), agterm workspace, Slack and
                           Linear workspaces, the Linear statuses and default label
  projects/<project>.json  one file per project: slack_channel, linear_team, linear_project,
                           repos (each with how it deploys), and any project-only settings,
                           tokens and secrets

Loaded, the project files appear as the list `products` (the autopilot schema's word, kept
by the subcommands). Only the keys above are ever printed; a project's other keys (tokens,
secrets) never are.

Usage:
  registry.py path                       print the settings.json path
  registry.py check                      validate the registry; prints OK
  registry.py get <dotted.key>           print one value (JSON for objects and lists)
  registry.py products                   one line per product: channel team project repos
  registry.py product-names              the product names (Slack channels), space-separated
  registry.py repos                      every repo name, one per line
  registry.py product-of-project <name>  the product whose Linear project (or channel) is <name>
  registry.py repo-of-product <name>     the first repo of product <name>
  registry.py add-product --channel C --team T [--project P] --repo NAME[:BRANCH]... --deploy NAME=JSON...
                                         JSON is the repo's deploy object, for example
                                         '{"job": "deploy", "check": "<command>"}' or '{"manual": "<how>"}'
  registry.py render-guidance            print the Linear Agent guidance fields, filled in

The workspace folder is SADENSMOL_DIR when set, else the first folder that holds
.workspace/, walking up from the current directory. Exit 1 on any error, with the reason.
"""

import argparse
import json
import os
import sys
import tempfile

PHASES = ["todo", "planning", "implementing", "qa", "review", "pr", "blocked", "done", "canceled"]
REGISTRY = os.path.join(".workspace", "settings.json")
PROJECTS = os.path.join(".workspace", "projects")
TEMPLATE = os.path.join(os.path.dirname(os.path.realpath(__file__)), "..", "references", "agent-guidance.md")


def fail(msg):
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(1)


def workspace_dir():
    env = os.environ.get("SADENSMOL_DIR")
    if env:
        return env
    d = os.getcwd()
    while True:
        if os.path.isdir(os.path.join(d, ".workspace")):
            return d
        parent = os.path.dirname(d)
        if parent == d:
            fail("no .workspace/ folder above the current directory; set SADENSMOL_DIR")
        d = parent


def registry_path():
    return os.path.join(workspace_dir(), REGISTRY)


def projects_dir():
    return os.path.join(workspace_dir(), PROJECTS)


def read_json(path):
    try:
        with open(path) as f:
            return json.load(f)
    except json.JSONDecodeError as e:
        fail(f"{path} is not valid JSON: {e}")


def load():
    path = registry_path()
    if not os.path.exists(path):
        fail(f"{path} is missing; create it as sadensmol-project -> Registry says")
    reg = read_json(path)
    if "products" in reg:
        fail(f"{path} lists projects; move each one to {projects_dir()}/<project>.json")
    reg["products"] = []
    pdir = projects_dir()
    if os.path.isdir(pdir):
        for name in sorted(os.listdir(pdir)):
            if name.endswith(".json"):
                reg["products"].append(read_json(os.path.join(pdir, name)))
    return reg


def save_project(product):
    pdir = projects_dir()
    os.makedirs(pdir, mode=0o700, exist_ok=True)
    path = os.path.join(pdir, product_name(product) + ".json")
    fd, tmp = tempfile.mkstemp(dir=pdir, prefix=".project.", suffix=".json")
    with os.fdopen(fd, "w") as f:
        json.dump(product, f, indent=2)
        f.write("\n")
    os.chmod(tmp, 0o600)
    os.replace(tmp, path)
    return path


def problems(reg):
    out = []
    for key in ("company", "agterm_workspace"):
        if not reg.get(key):
            out.append(f"key {key} is missing")
    if not reg.get("slack", {}).get("workspace"):
        out.append("key slack.workspace is missing")
    lin = reg.get("linear", {})
    for key in ("workspace", "default_label"):
        if not lin.get(key):
            out.append(f"key linear.{key} is missing")
    statuses = lin.get("statuses", {})
    for ph in PHASES:
        if not statuses.get(ph):
            out.append(f"key linear.statuses.{ph} is missing")
    for ph in statuses:
        if ph not in PHASES:
            out.append(f"key linear.statuses.{ph} is not a phase")
    products = reg.get("products", [])
    if not products:
        out.append(f"no project file in {projects_dir()}")
    seen = set()
    for i, p in enumerate(products):
        for key in ("slack_channel", "linear_team"):
            if not p.get(key):
                out.append(f"key products[{i}].{key} is missing")
        if p.get("slack_channel") in seen:
            out.append(f"products[{i}]: channel {p.get('slack_channel')} is listed twice")
        seen.add(p.get("slack_channel"))
        if not p.get("repos"):
            out.append(f"key products[{i}].repos has no repo")
        for j, r in enumerate(p.get("repos", [])):
            for key in ("name", "default_branch"):
                if not r.get(key):
                    out.append(f"key products[{i}].repos[{j}].{key} is missing")
            out.extend(deploy_problems(f"products[{i}].repos[{j}]", r.get("deploy")))
    for i, p in enumerate(products):
        shared = sum(1 for q in products if q.get("linear_team") == p.get("linear_team")) > 1
        if shared and not p.get("linear_project"):
            out.append(f"products[{i}]: team {p.get('linear_team')} is shared, so linear_project is required")
    return out


def deploy_problems(where, deploy):
    if deploy is None:
        return [f"key {where}.deploy is missing"]
    if not isinstance(deploy, dict):
        return [f"key {where}.deploy must be an object"]
    out = []
    for key in ("job", "check", "manual"):
        if deploy.get(key) is not None and not isinstance(deploy[key], str):
            out.append(f"key {where}.deploy.{key} must be a string")
    automatic = bool(deploy.get("job")) or bool(deploy.get("check"))
    if bool(deploy.get("manual")) == automatic:
        out.append(f"key {where}.deploy needs manual alone, or job or check (without manual)")
    minutes = deploy.get("timeout_minutes")
    if minutes is not None and (isinstance(minutes, bool) or not isinstance(minutes, int) or minutes <= 0):
        out.append(f"key {where}.deploy.timeout_minutes must be a whole number above zero")
    return out


def product_name(p):
    return p["slack_channel"]


def find_product(reg, name):
    for p in reg.get("products", []):
        if name in (p.get("slack_channel"), p.get("linear_project")):
            return p
    return None


def cmd_get(reg, key):
    if key.split(".")[0] == "products":
        fail("get reads settings.json only; project files may hold secrets, so use products / repos")
    cur = reg
    for part in key.split("."):
        if not isinstance(cur, dict) or part not in cur:
            fail(f"key {key} is missing in {registry_path()}")
        cur = cur[part]
    print(json.dumps(cur, indent=2) if isinstance(cur, (dict, list)) else cur)


def cmd_add_product(reg, args):
    channel = args.channel.lstrip("#")
    if find_product(reg, channel):
        fail(f"product {channel} is already in the registry")
    if args.project and find_product(reg, args.project):
        fail(f"Linear project {args.project} already belongs to a product")
    deploys = {}
    for spec in args.deploy or []:
        name, _, raw = spec.partition("=")
        try:
            deploys[name] = json.loads(raw)
        except json.JSONDecodeError as e:
            fail(f"--deploy {name}: the deploy object is not valid JSON: {e}")
    repos = []
    for spec in args.repo:
        name, _, branch = spec.partition(":")
        repo = {"name": name, "default_branch": branch or "main"}
        if name in deploys:
            repo["deploy"] = deploys.pop(name)
        repos.append(repo)
    if deploys:
        fail(f"--deploy names a repo that no --repo gives: {', '.join(sorted(deploys))}")
    product = {"slack_channel": channel, "linear_team": args.team}
    if args.project:
        product["linear_project"] = args.project
    product["repos"] = repos
    reg.setdefault("products", []).append(product)
    errs = problems(reg)
    if errs:
        fail("the registry would be invalid: " + "; ".join(errs))
    print(f"added project {channel}: {save_project(product)}")


def render_guidance(reg):
    with open(TEMPLATE) as f:
        text = f.read()
    products = reg.get("products", [])
    teams = sorted({p["linear_team"] for p in products})
    shared = len(teams) == 1 and len(products) > 1
    summary = ", ".join(
        f"{product_name(p)} (team {p['linear_team']}"
        + (f", project {p['linear_project']}" if p.get("linear_project") else "")
        + f", repo {p['repos'][0]['name']}, #{p['slack_channel']})"
        for p in products
    )
    routing = "\n".join(
        f"- #{p['slack_channel']} → team {p['linear_team']}"
        + (f", project {p['linear_project']}" if p.get("linear_project") else "")
        for p in products
    )
    if shared:
        model = (
            f"All projects live in ONE team: {teams[0]}. One project = one Linear project in that team + "
            "one git repo + one Slack channel, all with the same name. A feature = a milestone "
            "inside the Linear project."
        )
    else:
        model = (
            "One project = one team + one git repo + one Slack channel. A feature = a Linear project "
            "inside the project team."
        )
    st = reg["linear"]["statuses"]
    values = {
        "workspace": reg["company"],
        "model": model,
        "projects": summary,
        "routing": routing,
        "todo": st["todo"],
        "implementing": st["implementing"],
        "done": st["done"],
        "canceled": st["canceled"],
        "autopilot_statuses": ", ".join(st[k] for k in ("planning", "qa", "review", "pr", "blocked")),
    }
    for k, v in values.items():
        text = text.replace("{{" + k + "}}", v)
    print(text, end="")


def main():
    ap = argparse.ArgumentParser(add_help=True)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("path")
    sub.add_parser("check")
    g = sub.add_parser("get")
    g.add_argument("key")
    sub.add_parser("products")
    sub.add_parser("product-names")
    sub.add_parser("repos")
    pp = sub.add_parser("product-of-project")
    pp.add_argument("name")
    rp = sub.add_parser("repo-of-product")
    rp.add_argument("name")
    add = sub.add_parser("add-product")
    add.add_argument("--channel", required=True)
    add.add_argument("--team", required=True)
    add.add_argument("--project")
    add.add_argument("--repo", action="append", required=True)
    add.add_argument("--deploy", action="append")
    sub.add_parser("render-guidance")
    args = ap.parse_args()

    if args.cmd == "path":
        print(registry_path())
        return
    reg = load()
    if args.cmd == "check":
        errs = problems(reg)
        if errs:
            fail("; ".join(errs))
        print("OK")
    elif args.cmd == "get":
        cmd_get(reg, args.key)
    elif args.cmd == "products":
        for p in reg.get("products", []):
            print(product_name(p), p["linear_team"], p.get("linear_project", "-"), ",".join(r["name"] for r in p["repos"]))
    elif args.cmd == "product-names":
        print(" ".join(product_name(p) for p in reg.get("products", [])))
    elif args.cmd == "repos":
        for p in reg.get("products", []):
            for r in p["repos"]:
                print(r["name"])
    elif args.cmd == "product-of-project":
        p = find_product(reg, args.name)
        if not p:
            fail(f"no product has Linear project or channel {args.name}")
        print(product_name(p))
    elif args.cmd == "repo-of-product":
        p = find_product(reg, args.name)
        if not p:
            fail(f"no product {args.name}")
        print(p["repos"][0]["name"])
    elif args.cmd == "add-product":
        cmd_add_product(reg, args)
    elif args.cmd == "render-guidance":
        errs = problems(reg)
        if errs:
            fail("; ".join(errs))
        render_guidance(reg)


if __name__ == "__main__":
    main()
