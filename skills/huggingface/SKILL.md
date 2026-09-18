---
name: sadensmol-huggingface
description: Download and manage Hugging Face models and datasets with the `hf` CLI. Use when a task needs model weights fetched from the Hub, when a download 401s or reports "Access denied. This repository requires approval.", when checking whether weights are already cached before triggering a multi-GB pull, when wiring a local model path into code, or when the user mentions Hugging Face, `hf`, `huggingface-cli`, `HF_TOKEN`, `HF_HOME`, an `org/model` repo id, snapshot_download, or a gated/licence-gated model.
---

# Hugging Face

## The CLI is `hf`, and it is already authenticated

Use **`hf`**. `huggingface-cli` is the old name for the same tool and still resolves —
prefer `hf` in anything written down.

**A token is already configured on this machine.** Do NOT ask the user to log in, do
NOT prompt for a token, and do NOT tell them to `export HF_TOKEN`. Verify instead:

```bash
hf auth whoami          # prints user=<name> orgs=<...> when a token is live
```

Only if that fails is auth the problem.

## Check the cache before downloading anything

Model pulls are gigabytes. Never start one to find out whether it was needed.

```bash
hf cache ls                                     # what is already local
ls ~/.cache/huggingface/hub/models--<org>--<name>   # one specific repo
```

Cache layout — `$HF_HOME` overrides the root, default `~/.cache/huggingface`:

```
~/.cache/huggingface/
  token                                   # the credential
  hub/models--<org>--<name>/
    blobs/                                # the real files
    snapshots/<sha>/                      # symlinks — this is the path a loader wants
```

`hf download` prints the snapshot path on success; that is the path to hand to code
that needs a local directory.

## Downloading

```bash
hf download <org>/<model>                        # whole repo
hf download <org>/<model> --include "*.json"     # just what is needed
hf download <org>/<model> config.json            # one named file
```

**Prefer `--include` over a full pull** when only a config or a single weight file is
needed — repos routinely carry several formats of the same weights.

**A CLI download warms the same cache the Python loaders read.** After `hf download`,
`transformers`' `from_pretrained`, `diffusers`' `from_pretrained` and
`huggingface_hub.snapshot_download` all find the files locally with no second fetch
and no network. Use this to pre-stage weights before running a service that would
otherwise download them inside a request.

## Gated repos: a valid token is NOT enough

```
Error: Access denied. This repository requires approval.
```

This is **not** an auth failure and no token, env var, or CLI flag fixes it. The repo
is licence-gated, and the account owning the token must accept the licence **once, in
a browser**, on that model's page (`https://huggingface.co/<org>/<model>`). Approval
is per repo — being approved for one Stability/Meta/Mistral model grants nothing for
the next.

**When this appears:** say plainly that the token is fine and the licence is the
blocker, give the exact model URL to click, and — where the task allows — offer a
non-gated alternative model so work is not stalled on someone else's approval queue.
Do not retry the download hoping it resolves, and never suggest scraping the file
from a mirror.

## Picking a model to suggest

Check three things before recommending weights, in this order:

1. **Gated or not** — a gated repo blocks any unattended pipeline (CI, a fresh
   machine, a container build) until a human clicks through.
2. **Licence** — many strong open weights are **CC-BY-NC** (non-commercial). Fine for
   personal use, disqualifying for anything shipped. Say which it is; never assume
   "open weights" means "usable commercially".
3. **Size on disk vs resident cost** — the download size and the RAM/VRAM the loaded
   model occupies are different numbers, and the second one is what fails at runtime.
