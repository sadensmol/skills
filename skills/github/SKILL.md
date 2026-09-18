---
name: sadensmol-github
description: |
  GitHub integration and repository management. Use when working with GitHub repositories, reading documentation from GitHub, fetching file contents, managing pull requests, issues, or any GitHub-related tasks. Also covers stacked pull requests — creating a PR chain with explicit `--base`, restacking with `git rebase --update-refs`, and merging the stack bottom-up.
---

# GitHub Integration

This skill provides guidance for interacting with GitHub repositories using the `gh` CLI tool.

## Reading Files from GitHub

To read file contents from GitHub repositories, use the GitHub API via `gh` command:

### Basic Syntax

```bash
gh api repos/{owner}/{repo}/contents/{path} --jq .content | base64 -d
```

### Examples

**Read a markdown file:**
```bash
gh api repos/{owner}/{repo}/contents/docs/error-handling.md --jq .content | base64 -d
```

**Read a file with spaces in path:**
```bash
gh api repos/{owner}/{repo}/contents/docs/integrations/01.%20general.md --jq .content | base64 -d
```

**Read first 20 lines:**
```bash
gh api repos/{owner}/{repo}/contents/docs/architecture.md --jq .content | base64 -d | head -20
```

### URL Encoding

Files with spaces or special characters in their names need URL encoding:
- Space → `%20`
- Example: `01. general.md` → `01.%20general.md`

## View Pull Requests

```bash
# List PRs
gh pr list

# View specific PR
gh pr view 123

# View PR diff
gh pr diff 123

# View PR comments
gh api repos/{owner}/{repo}/pulls/123/comments
```

## Stacked Pull Requests

A stack is a chain of small PRs. Each PR uses the previous branch as its base, not `main`.
Use a stack when one change is too large to review at once, or when a later change depends on an earlier one.

```
main ← feat/step-1 (PR #1) ← feat/step-2 (PR #2) ← feat/step-3 (PR #3)
```

### REGISTER THE STACK WITH GITHUB — ALWAYS (MUST FOLLOW)

Chaining the base branches is only **half** of a stack. GitHub has a native
stacked-PR feature (public preview) in which a stack is a **registered object**
with its own number, and the base chain alone does NOT create one — GitHub only
detects the shape and offers a "This pull request can be stacked… **Create
stack**" banner that a human has to click. **Whenever you open two or more PRs
in a base chain, register the stack.** Leaving it unregistered silently gives up:

- **automatic rebasing** — GitHub restacks the branches itself when the bottom
  merges ("rebasing is the trickiest part of working with stacks"); unregistered,
  you do the `rebase --onto` dance by hand, which is where duplicate commits come
  from;
- **CI on every layer** — checks that run on the default branch run for *all* PRs
  in the stack, including mid-stack ones;
- **branch protection mid-stack** — enforced on every PR in the stack, not just
  the bottom one;
- **auto-retarget** — merging any PR retargets the ones above it.

Create it bottom-to-top, right after the last PR of the chain exists:

```bash
# pull_requests is ordered BOTTOM first, TOP last.
gh api -X POST -H "X-GitHub-Api-Version: 2026-03-10" \
  /repos/<owner>/<repo>/stacks --input - <<'JSON'
{"pull_requests": [365, 366]}
JSON
```

**Use `--input -` with real JSON, never `-f 'pull_requests[]=365'`** — `-f` sends
strings and the endpoint rejects them (`422: "365" is not of type integer`).

The response carries the stack's own `number` (e.g. `367`) and its `base.ref`.

Inspect, extend and dissolve:

```bash
gh api /repos/<owner>/<repo>/stacks                       # all stacks
gh api /repos/<owner>/<repo>/stacks?pull_request=366      # the stack a PR is in
gh api /repos/<owner>/<repo>/stacks/<stack-number>        # one stack

# append PRs above the current top (ordered bottom-most first)
gh api -X POST /repos/<owner>/<repo>/stacks/<n>/add --input - <<'JSON'
{"pull_requests": [367]}
JSON

gh api -X POST /repos/<owner>/<repo>/stacks/<n>/unstack   # dissolve
```

Constraints (from the GitHub docs): every branch must live in the **same repo** —
cross-fork stacks are unsupported — PRs merge **bottom-up**, and the feature is
in public preview. There is also a `gh stack` extension for the local
branch workflow; the REST calls above need no extension and are what to use from
a script.

Docs: <https://docs.github.com/en/pull-requests/get-started/about-stacked-prs>
and <https://docs.github.com/en/rest/pulls/stacks>.

### Create a Stack

```bash
# Bottom of the stack: base is main
git switch -c feat/step-1 main
git commit -am "step 1"
git push -u origin feat/step-1
gh pr create --base main --head feat/step-1 --title "Step 1" --fill

# Next PR: base is the previous branch
git switch -c feat/step-2 feat/step-1
git commit -am "step 2"
git push -u origin feat/step-2
gh pr create --base feat/step-1 --head feat/step-2 --title "Step 2" --fill
```

Always pass `--base` explicitly. Without it `gh` targets the default branch, and the PR diff then shows the parent's commits too.

State the stack position in each PR body, so reviewers read in order:

```bash
gh pr create --base feat/step-1 --head feat/step-2 --title "Step 2" \
  --body "Stack: #1 → **#2** → #3. Review #1 first."
```

**Then register the stack** (see above) — the `--base` chain is the prerequisite,
not the stack itself.

### View a Stack

```bash
# Show base and head of every open PR
gh pr list --json number,title,baseRefName,headRefName \
  --jq '.[] | "\(.number) \(.headRefName) → \(.baseRefName)"'

# Diff of one PR only (against its own base)
gh pr diff 2
```

### Update a Stack After Review

Change the commit in the PR that owns it, then restack everything above it.
`--update-refs` moves the branch pointers of all branches in the range:

```bash
git switch feat/step-3            # top of the stack
git rebase --update-refs main     # rewrites step-1..step-3, moves all refs

git push --force-with-lease origin feat/step-1 feat/step-2 feat/step-3
```

Set it as the default:

```bash
git config --global rebase.updateRefs true
```

Rules:
- Use `--force-with-lease`, never plain `--force`. It aborts if somebody else pushed.
- Never rebase a parent branch without restacking its children. The children then show the parent's old commits as new changes.
- Push the whole stack in one command, so intermediate PRs never point at a missing commit.

### Merge a Stack

Merge bottom-up. Merge the PR whose base is `main` first.
GitHub then retargets the child PR to `main` automatically.

**For a REGISTERED stack the manual restack below is unnecessary** — GitHub
rebases the remaining branches itself, and merging the top PR merges the whole
stack. The commands below are the fallback for a chain that was never registered
(and the reason to always register one).

```bash
gh pr merge 1 --squash --delete-branch   # bottom PR

# Rebase the rest onto the new main, then push
git fetch origin
git switch feat/step-3
git rebase --update-refs --onto origin/main feat/step-1
git push --force-with-lease origin feat/step-2 feat/step-3
```

Repeat until the stack is empty.

Squash-merge creates a new commit hash for the parent. The child branch still holds the original commits.
Always rebase `--onto origin/main` after a squash-merge. Skipping it produces duplicate or conflicting commits in the child PR.

Merge-commit merges keep the hashes, so a plain `git rebase origin/main` is enough.

### Cleanup

```bash
git fetch --prune
git branch --merged origin/main | grep 'feat/' | xargs -r git branch -d
```

### When Not to Stack

- The parts are independent → open parallel PRs off `main` instead.
- The stack is more than 3–4 deep → each rebase touches every branch, and conflicts multiply.
- Reviewers merge out of order → close the stack and open one PR.

## View Issues

```bash
# List issues
gh issue list

# View specific issue
gh issue view 456
```

## Repository Information

```bash
# View repository details
gh repo view {owner}/{repo}

# List branches
gh api repos/{owner}/{repo}/branches

# List tags
gh api repos/{owner}/{repo}/tags
```

## File and Directory Listing

```bash
# List directory contents
gh api repos/{owner}/{repo}/contents/{path}

# Get file metadata
gh api repos/{owner}/{repo}/contents/{file_path} --jq '{name, size, path, sha}'
```

## Best Practices

1. **Always use base64 decode** when reading file contents: `--jq .content | base64 -d`
2. **URL encode special characters** in file paths (spaces, dots at start of filename)
3. **Check file existence** before reading to avoid errors
4. **Use pagination** for large directory listings
5. **Cache frequently accessed** documentation locally during a session to reduce API calls

## Error Handling

**File not found:**
```bash
# Returns 404 error
# Check path and ensure file exists in repository
```

**Authentication required:**
```bash
# Ensure gh CLI is authenticated
gh auth status
gh auth login
```

**Rate limiting:**
```bash
# Check API rate limit
gh api rate_limit
```

## Tips

- Use `--jq` for JSON parsing and filtering
- Pipe to `head`, `tail`, or `grep` for large files
- Use `| wc -l` to count lines
- Use `| less` for interactive viewing of large files
