---
name: sadensmol-tmux
description: "tmux terminal multiplexer reference for macOS. Use when: (1) creating or managing tmux sessions, windows, or panes, (2) splitting terminal layouts, (3) scripting multi-pane development environments, (4) setting up tmux configuration, (5) using copy mode or scrollback, (6) automating tmux from other skills or scripts, (7) user mentions tmux, terminal multiplexer, or multi-pane terminal setup. Designed as a callable reference from other skills needing terminal management."
---

# tmux on macOS

Default prefix: `Ctrl-b` (denoted `C-b`). Override in `~/.tmux.conf` with `set -g prefix C-a`.

## Installation

```bash
brew install tmux
```

Verify: `tmux -V`

## Core Concepts

- **Session**: top-level container, persists after detach
- **Window**: tab within a session
- **Pane**: split within a window

## Sessions

| Action | Command |
|---|---|
| New named session | `tmux new -s NAME` |
| List sessions | `tmux ls` |
| Attach to session | `tmux attach -t NAME` |
| Detach | `C-b d` |
| Kill session | `tmux kill-session -t NAME` |
| Rename session | `C-b $` |
| Switch session | `C-b s` (interactive list) |

## Windows

| Action | Command |
|---|---|
| New window | `C-b c` |
| Next / Previous | `C-b n` / `C-b p` |
| Go to window N | `C-b 0`..`C-b 9` |
| Rename window | `C-b ,` |
| Close window | `C-b &` |
| List windows | `C-b w` (interactive) |

## Panes

| Action | Command |
|---|---|
| Split horizontal | `C-b "` |
| Split vertical | `C-b %` |
| Navigate panes | `C-b arrow-key` |
| Cycle panes | `C-b o` |
| Zoom/unzoom pane | `C-b z` |
| Resize pane | `C-b C-arrow` (hold Ctrl) |
| Close pane | `C-b x` or `exit` |
| Swap panes | `C-b {` / `C-b }` |
| Show pane numbers | `C-b q` then press number |
| Convert pane to window | `C-b !` |

## Copy Mode

Enable vi keys: `setw -g mode-keys vi` in `~/.tmux.conf`.

| Action | Command |
|---|---|
| Enter copy mode | `C-b [` |
| Start selection | `Space` |
| Copy selection | `Enter` |
| Paste buffer | `C-b ]` |
| Search forward / back | `/` / `?` |
| Scroll up / down | `C-u` / `C-d` |
| Exit copy mode | `q` |

macOS system clipboard integration:
```bash
bind -T copy-mode-vi y send -X copy-pipe-and-cancel "pbcopy"
```

## Scripting & Automation

### Multi-pane layout example

```bash
#!/bin/bash
SESSION="dev"

tmux new-session -d -s "$SESSION" -n "editor"
tmux send-keys -t "$SESSION:editor" "vim ." Enter

tmux split-window -h -t "$SESSION:editor"
tmux send-keys -t "$SESSION:editor.1" "make watch" Enter

tmux split-window -v -t "$SESSION:editor.1"
tmux send-keys -t "$SESSION:editor.2" "tail -f logs/app.log" Enter

tmux select-pane -t "$SESSION:editor.0"
tmux attach -t "$SESSION"
```

### Key scripting commands

```bash
# Target syntax: SESSION:WINDOW.PANE
tmux new-session -d -s NAME -n WINDOW_NAME
tmux send-keys -t "SESSION:WINDOW.PANE" "command" Enter

# Splits
tmux split-window -h -t TARGET   # vertical split
tmux split-window -v -t TARGET   # horizontal split

# Layout presets
tmux select-layout -t TARGET even-horizontal
tmux select-layout -t TARGET even-vertical
tmux select-layout -t TARGET main-horizontal
tmux select-layout -t TARGET main-vertical
tmux select-layout -t TARGET tiled

# Resize
tmux resize-pane -t TARGET -D 10   # down 10 lines
tmux resize-pane -t TARGET -R 20   # right 20 cols

# Check session exists
tmux has-session -t NAME 2>/dev/null && echo "exists" || echo "not found"
```

## Configuration

For detailed `~/.tmux.conf` setup including mouse support, status bar, and TPM plugin manager, see [references/config.md](references/config.md).

## API for Other Skills

When calling tmux from another skill or script:

1. **Check for existing session**: `tmux has-session -t NAME 2>/dev/null`
2. **Create if missing**: `tmux new-session -d -s NAME`
3. **Send commands**: `tmux send-keys -t NAME "command" Enter`
4. **Always use `-d`** (detached) when creating sessions programmatically
5. **Use explicit targets** (`-t SESSION:WINDOW.PANE`) to avoid ambiguity

### Helper pattern for skills

```bash
ensure_tmux_session() {
  local session="$1"
  tmux has-session -t "$session" 2>/dev/null || tmux new-session -d -s "$session"
}

send_to_pane() {
  local target="$1"  # e.g. "dev:editor.0"
  local cmd="$2"
  tmux send-keys -t "$target" "$cmd" Enter
}
```
