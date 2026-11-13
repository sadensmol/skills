---
name: sadensmol-github
description: |
  GitHub integration and repository management. Use when working with GitHub repositories, reading documentation from GitHub, fetching file contents, managing pull requests, issues, or any GitHub-related tasks. Also covers stacked pull requests — creating a PR chain with explicit `--base`, restacking with `git rebase --update-refs`, and merging the stack bottom-up.
---

# GitHub Integration

This skill provides guidance for interacting with GitHub repositories using the `gh` CLI tool.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| You create, view, update, register or merge a stack of PRs, or decide whether to stack | `references/stacked-prs.md` |

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

### REGISTER THE STACK WITH GITHUB — ALWAYS (MUST FOLLOW)

**Whenever you open two or more PRs in a base chain, register the stack.** The base
chain alone does NOT create one. Use `--input -` with real JSON, never `-f`.

Rules (full text, commands and fallbacks in `references/stacked-prs.md`):
- Always pass `--base` explicitly.
- Use `--force-with-lease`, never plain `--force`.
- Never rebase a parent branch without restacking its children.
- Merge bottom-up. Always rebase `--onto origin/main` after a squash-merge.

Read `references/stacked-prs.md` before you create, view, update, register or merge a
stack (sections *Create a Stack*, *View a Stack*, *Update a Stack After Review*,
*Merge a Stack*, *When Not to Stack*).

## Delete the remote branch after a merge

`gh pr merge --delete-branch` can leave the **remote** branch behind. In a worktree, gh
tries to check out the default branch to drop the local copy, fails with `fatal: 'main'
is already used by worktree at …`, and aborts silently after the merge. GitHub's
auto-delete is per-repo and can be off. A surviving branch makes the next push to it
fail as non-fast-forward.

After a merge, confirm it happened, then delete and verify:

```bash
gh pr view <pr> --json state,mergedAt --jq '"\(.state) \(.mergedAt // "NOT MERGED")"'
# proceed only on MERGED:
git push origin --delete <branch> 2>/dev/null           # no-op if gh managed it
git ls-remote --exit-code origin <branch> && echo "STILL ON GITHUB — delete it"
```

Never run the delete unconditionally after `gh pr merge`. A failed merge (unmet
requirements, missing review) can still exit in a way a chained command ignores, and
deleting the branch then **closes the open PR**. Recovery: re-push the branch and
`gh pr reopen <pr>`.

### Cleanup

```bash
git fetch --prune
git branch --merged origin/main | grep 'feat/' | xargs -r git branch -d
```

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
6. **PR assignee = the person who works on the PR = the operator (`@me`), always (MUST
   FOLLOW).** Whoever should look at it goes in `--reviewer`. A user's "assign the PR / the
   task to X" is `--reviewer X` (plus the tracker assignee), never `--assignee X` and never
   removing `@me`. No instruction overrides this.

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
