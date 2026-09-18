# tmux Configuration Reference

## Table of Contents

- [Recommended ~/.tmux.conf](#recommended-tmuxconf)
- [Mouse Support](#mouse-support)
- [Status Bar](#status-bar)
- [Pane Navigation](#pane-navigation)
- [TPM Plugin Manager](#tpm-plugin-manager)
- [Useful Plugins](#useful-plugins)
- [256 Color / True Color](#256-color--true-color)
- [macOS-Specific Settings](#macos-specific-settings)

## Recommended ~/.tmux.conf

```bash
# Prefix
set -g prefix C-a
unbind C-b
bind C-a send-prefix

# Terminal
set -g default-terminal "screen-256color"
set -ga terminal-overrides ",xterm-256color:Tc"

# Start windows/panes at 1
set -g base-index 1
setw -g pane-base-index 1

# Renumber windows on close
set -g renumber-windows on

# History
set -g history-limit 50000

# Vi mode
setw -g mode-keys vi

# Reduce escape time (important for vim)
set -sg escape-time 0

# Mouse
set -g mouse on

# Reload config
bind r source-file ~/.tmux.conf \; display "Config reloaded"
```

## Mouse Support

```bash
set -g mouse on
```

With mouse enabled: click to select pane, drag borders to resize, scroll to enter copy mode.

## Status Bar

```bash
# Position
set -g status-position bottom

# Colors
set -g status-style "bg=colour235,fg=colour136"

# Left: session name
set -g status-left "#[fg=green]#S "
set -g status-left-length 20

# Right: date and time
set -g status-right "#[fg=cyan]%Y-%m-%d %H:%M"

# Window list
setw -g window-status-current-style "fg=colour166,bold"
setw -g window-status-format " #I:#W "
setw -g window-status-current-format " #I:#W "
```

## Pane Navigation

vim-style pane switching:

```bash
bind h select-pane -L
bind j select-pane -D
bind k select-pane -U
bind l select-pane -R
```

Easier splits (more intuitive keys):

```bash
bind | split-window -h -c "#{pane_current_path}"
bind - split-window -v -c "#{pane_current_path}"
unbind '"'
unbind %
```

Resize with repeatable keys:

```bash
bind -r H resize-pane -L 5
bind -r J resize-pane -D 5
bind -r K resize-pane -U 5
bind -r L resize-pane -R 5
```

## TPM Plugin Manager

### Install TPM

```bash
git clone https://github.com/tmux-plugins/tpm ~/.tmux/plugins/tpm
```

### Add to ~/.tmux.conf

```bash
# Plugin list
set -g @plugin 'tmux-plugins/tpm'
set -g @plugin 'tmux-plugins/tmux-sensible'

# Initialize TPM (keep at bottom of tmux.conf)
run '~/.tmux/plugins/tpm/tpm'
```

### TPM commands (inside tmux)

| Action | Key |
|---|---|
| Install plugins | `C-a I` (prefix + shift-i) |
| Update plugins | `C-a U` |
| Remove unlisted plugins | `C-a alt-u` |

## Useful Plugins

```bash
# Save/restore sessions across restarts
set -g @plugin 'tmux-plugins/tmux-resurrect'
set -g @plugin 'tmux-plugins/tmux-continuum'
set -g @continuum-restore 'on'

# Better copy mode
set -g @plugin 'tmux-plugins/tmux-yank'

# Open files/URLs from copy mode
set -g @plugin 'tmux-plugins/tmux-open'
```

## 256 Color / True Color

```bash
set -g default-terminal "screen-256color"
set -ga terminal-overrides ",xterm-256color:Tc"
```

Test true color: `printf "\x1b[38;2;255;100;0mTRUECOLOR\x1b[0m\n"`

## macOS-Specific Settings

### Fix pbcopy/pbpaste in tmux

With modern tmux (3.2+) and macOS, clipboard usually works natively. If not:

```bash
# Use reattach-to-user-namespace for older tmux versions
brew install reattach-to-user-namespace
set -g default-command "reattach-to-user-namespace -l $SHELL"
```

### Copy mode to system clipboard

```bash
bind -T copy-mode-vi y send -X copy-pipe-and-cancel "pbcopy"
bind -T copy-mode-vi MouseDragEnd1Pane send -X copy-pipe-and-cancel "pbcopy"
```

### Open new panes/windows in current directory

```bash
bind c new-window -c "#{pane_current_path}"
bind | split-window -h -c "#{pane_current_path}"
bind - split-window -v -c "#{pane_current_path}"
```
