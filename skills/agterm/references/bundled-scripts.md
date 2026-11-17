# Bundled scripts

Read this when `session overlay open --image` is refused and you fall back to the image script, or wrap a long-running command so the session row shows its status (`agst`). File names below are relative to the skill root.

## Displaying an image inline

Use `session overlay open --image FILE` first (`references/html-pages.md` → *Native pages*). This script is the
fallback for an agterm that refuses `--image`.

This skill bundles `scripts/show-image.sh`. It opens an overlay (a real terminal) and renders the
image there via the kitty graphics protocol, which ghostty draws natively — no kitty binary and no
external image tool, just `base64` + `printf`. Run it with the image path (optional size percent,
default 60):

The script sits next to `SKILL.md`, so resolve `scripts/show-image.sh` against the directory this
`SKILL.md` was loaded from — not against your working directory. That one form is correct wherever
the skill came from: a plugin install (`~/.claude/plugins/cache/…`, `~/.codex/plugins/cache/…`) or the
app's own **Help ▸ Install Agent Skill…** copy (`~/.claude/skills/agterm/`, `~/.codex/skills/agterm/`).

```bash
bash <this-skill-directory>/scripts/show-image.sh <image> [size-percent]
```

The size percent sizes the overlay PANEL; the image itself is scaled to fit that panel and centered
in it, so a large screenshot needs no resizing beforehand.

Do NOT print graphics escapes to your own tool stdout (the agent harness escapes the control bytes)
and do NOT run an image viewer in your tool shell (no controlling terminal). The overlay is what makes
it render. Outside agterm (`AGTERM_ENABLED` unset) there is no overlay — fall back to `open <image>`.

## Long-command status wrapper (`agst`)

This skill bundles `scripts/agst.sh`, the cookbook's `long-commands-status` wrapper, so nothing is
copied to `~/bin`. It sets the session's row `active` while a command runs, then `completed` on exit 0
or `blocked` on non-zero; the exit code passes through, and outside agterm it just `exec`s the command.
Resolve it against this skill's directory, physically (`pwd -P`), and call it through `sh`:

```bash
AGST="$(cd <this-skill-directory> && pwd -P)/scripts/agst.sh"
sh "$AGST" make test
agtermctl session new … --command "zsh -lc 'sh \"$AGST\" task dev'"
```

`whats-new.md` step 2 refreshes it when the recipe changes upstream.
