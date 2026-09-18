---
name: sadensmol-jetson-orin-nano
description: Working on an NVIDIA Jetson Orin Nano device (JetPack 7.2) — connecting to it over SSH, debugging the running containerized stack, running/serving LLMs and VLMs (Ollama, vLLM, llama.cpp), auditing memory, tuning inference for the 8 GB unified RAM, benchmarking, video codec work, and JetPack-7/CUDA-13 package + container choices. Use when the user mentions a Jetson/Orin board, JetPack 7, L4T r39, CUDA 13, `tegrastats`, running models on the Jetson GPU, or debugging services on the device. Bundles NVIDIA-AI-IOT's device-side Jetson skills (Apache-2.0) plus the SSH-connect/debug recipe.
---

# Jetson Orin Nano

Guidance for working on a live NVIDIA **Jetson Orin Nano 8 GB** running
**JetPack 7.2**. Two layers:

1. **The connect + debug recipe** (below) — how to reach a board and inspect the
   running stack. Hard-won, including what NOT to do.
2. **NVIDIA's device-side Jetson skills** (in `references/nvidia-jetson-device-skills/`)
   — load the specific one for the task (LLM serving, memory audit, diagnostics, …).

## The board

| Field | Value |
|---|---|
| Model | Jetson Orin Nano 8 GB (Ampere iGPU, compute capability **sm_87**) |
| OS | JetPack **7.2** / L4T **r39.2** / **CUDA 13.2** / Python 3.12 / Ubuntu 24.04 |
| Memory | **8 GB unified** CPU+GPU (~7.4 GB usable); NVMe (Crucial P310, fast) |
| No | `nvidia-smi` (use `tegrastats`); no `jetson-containers`/`autotag` on JP7 |

**Address, SSH user, container names and deploy layout are per-project — never
hardcoded here.** Take them from the project's own skill (the `<project>-*`
plugin loaded for the session), from an env var, or ask. Below, `$JETSON_HOST`
means that project's `<user>@<host>` and `<ctr>` its container name.

## Connecting over SSH (and debugging without wedging it)

**✅ Passwordless SSH is normally already set up.** A key is installed on the board, so a bare
`ssh "$JETSON_HOST" '<cmd>'` connects with **no password** — no `expect`, no
`sshpass`. This is the default path; use it. `scp`/`rsync` to the board work the same
way with no password.

**Host unreachable by IP? Use mDNS.** A recorded LAN IP goes stale (or the
board's routing drops it) and `ssh <user>@<ip>` just times out. The board's
`<hostname>.local` still resolves — try it before concluding the board is down,
and `hostname -I` over that session tells you the current addresses.

**⚠️ A container env change needs a RECREATE, never `docker restart`.** Compose
resolves `environment:` / `env_file` at container-CREATE time, so a restart
silently keeps the OLD values — `docker inspect <ctr> --format '{{.Config.Env}}'`
is the check that proves which the running container actually has. Re-run
`docker compose up -d --no-deps <service>` to apply. Pin `IMAGE_TAG` to the tag
already on disk; a tag the board never pulled makes compose hit the registry and
401 on a private repo.

**⚠️ Still ASK FIRST — permission (MANDATORY, every session).** Before you run ANY
command that reaches the board (SSH, `scp`, `rsync`), ask the user for explicit
permission — name the host and what you intend to run (e.g. "read the app
container logs"). Wait for a clear yes. If the user declines, stop — do not
connect. A project skill may grant standing permission for its own board; that
overrides this.

```bash
ssh "$JETSON_HOST" 'hostname'   # passwordless — key already installed
```

**⚠️ WATCHDOG EVERY COMMAND — never let a board command hang (MUST FOLLOW).**
`ConnectTimeout` bounds ONLY the TCP/auth handshake. Once the session is open a
remote command can block forever, and on this board that is the NORMAL case: a
wedged USB stick parks `lsblk`, `docker exec`, `umount` and `df` in
uninterruptible sleep, and a thrashing 8 GB box does the same to `docker ps` and
`docker logs`. A single unbounded call then burns minutes of the user's time and
tells you nothing. Bound the remote side, not just the connection:

```bash
ssh -o ConnectTimeout=5 -o ServerAliveInterval=5 -o ServerAliveCountMax=2 \
    "$JETSON_HOST" 'timeout 20 docker exec <ctr> lsblk'
```

- `timeout <N>` on the **remote** command is the part that actually saves you —
  `ServerAliveInterval/CountMax` only drop a dead link, not a live-but-stuck one.
- Set the Bash tool's own `timeout` low too (30–60 s). A 200 s tool timeout means
  200 s of nothing.
- **Never run an unbounded reader:** `docker logs` without `--tail`, `journalctl`
  without `--since`/`-n`, `find /`, `du -sh` on a mount. Cap them at the source.
- A command that times out is DATA — "this hangs" localises the fault (usually
  dead media or thrash). Do not silently retry it; check `uptime` and
  `ps -eo state,pid,comm | awk '$1 ~ /D/'` first.

**Password fallback (only if the key is ever missing).** A bare `ssh`/`scp` offers every agent key
first and the server drops you with **"Too many authentication failures"** before
the password. Force password-only, and drive it with `expect`:
```bash
/usr/bin/expect <<'EOF'
set timeout 60
spawn ssh -o PubkeyAuthentication=no -o PreferredAuthentications=password \
     -o IdentitiesOnly=yes -o NumberOfPasswordPrompts=1 \
     -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
     $env(JETSON_HOST) {docker ps --format "{{.Names}} {{.Status}}"; free -h}
expect { -re {[Pp]assword:} { send "<board-password>\r"; exp_continue } eof }
EOF
```
(Keep the password out of committed files — inject it, or use the key.)

**Quoting:** for `docker exec … python -c "…SQL with 'quotes'…"`, base64-encode the
payload and `echo <b64> | base64 -d | …` on the far side.

**`sudo`** needs a password: `echo '<board-password>' | sudo -S <cmd>`.

**⚠️ Do NOT wedge the box (learned the hard way).** It is **8 GB with swap that
fills**; under memory pressure it thrashes into unresponsiveness and needs a
physical reboot. Rules:
- **Never** bring up all containers at once when RAM is tight; start one service,
  confirm it's healthy, then the next.
- **One** command at a time; do **not** retry a hung command in a loop — a hang
  usually means the box is thrashing, and piling on more probes finishes it off.
- `docker ps`/`docker logs` **hanging** is a symptom of thrash, not a reason to
  retry. Back off, check `free -h`.
- The GPU **cannot execute from swap** — more swap prevents OOM but never speeds a
  GPU workload; pages must fault back into RAM first.

Common checks (single, read-only):
```bash
docker ps --format "{{.Names}} {{.Status}}"        # what's up
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"
free -h; swapon --show                              # RAM + swap
sudo tegrastats --interval 1000                     # GR3D_FREQ > 0% = GPU busy
docker exec <ollama-ctr> ollama ps                  # loaded models + PROCESSOR (want 100% GPU)
```

## JetPack 7.2 LLM/GPU findings on this board (verified)

- **The GPU works on JP7.2** for containers via `runtime: nvidia` and natively.
  For in-process PyTorch, install torch/torchvision from the **cu132** index
  (`https://download.pytorch.org/whl/cu132`) — Orin uses SBSA on JP7.2, so upstream
  CUDA-13.2 wheels run; `torch.cuda.is_available()` is `True`.
- A `python:3.12-slim` container also needs `NVIDIA_VISIBLE_DEVICES=all` +
  `NVIDIA_DRIVER_CAPABILITIES=all`, or the runtime injects no driver libs and CUDA
  is invisible even with `runtime: nvidia`.
- **Ollama TEXT works on GPU; VISION/captioning is broken on JP7 with the generic
  `ollama/ollama:latest`** — any image request deadlocks at 0 % CPU (`clip_ctx: CLIP
  using CPU backend`; an upstream mtmd/sm_87 bug). Memory, swap, image size, and
  `ipc:host` do **not** fix it.
- NVIDIA's JP7 ollama image `ghcr.io/nvidia-ai-iot/ollama:*` has the sm_87 kernels,
  BUT the published `r38.2…cu130` tag **crashes the scheduler on GPU init on a
  `r39.2/cu132` box** (CUDA-minor mismatch). A matching `r39/cu132` tag or a native
  JP7.2 ollama build is the path for GPU captioning; until then captioning stays
  blocked while everything else runs on the GPU.
- On 8 GB, never hold two models resident at once: `OLLAMA_MAX_LOADED_MODELS=1`, and
  free any in-process model (e.g. `torch.cuda.empty_cache()`) before asking Ollama
  to load a large model — a pinned CUDA allocator makes Ollama fall back to CPU.

## Bundled NVIDIA device skills

Apache-2.0, from [NVIDIA-AI-IOT/jetson-device-skills](https://github.com/NVIDIA-AI-IOT/jetson-device-skills)
— also published verbatim under [NVIDIA/skills](https://github.com/NVIDIA/skills/tree/main/skills)
(the copies here are byte-identical to `NVIDIA/skills@main`). See
`references/nvidia-jetson-device-skills/`. Load the one that fits the task —
each has its own `SKILL.md` and helper scripts:

| Skill | Use for |
|---|---|
| `jetson-diagnostic` | Read-only device health snapshot (identity, mem, GPU, thermal, power, storage, services). |
| `jetson-print-device-info` | Print module model, L4T, kernel, OS, power mode. |
| `jetson-memory-audit` | Measure DRAM/NvMap usage; verify before/after reclamation. |
| `jetson-headless-mode` | Reclaim GUI/daemon RAM (headless edge node). |
| `jetson-inference-mem-tune` | Pick serving stack + per-runtime memory flags (vLLM/SGLang/llama.cpp/TRT). |
| `jetson-llm-serve` | Stand up vLLM/SGLang serving (upstream vLLM on Orin JP7.2+). |
| `jetson-llm-benchmark` | Benchmark vLLM/llama.cpp/Ollama with structured JSON. |
| `jetson-speculative-decoding` | EAGLE-3 / draft-model spec decoding for vLLM when TPOT-bound. |
| `jetson-package` | Jetson container / wheel / PyPI-index choices; Orin sm_87 vs Thor sm_110. |
| `jetson-video-setup` / `-capability` / `-recipe` / `-benchmark` / `-pipeline` | NVENC/NVDEC + PyNvVideoCodec setup, capability checks, plans, benchmarks, pipelines. |

There's also an agent, `references/nvidia-jetson-device-skills/../agents/jetson-perf-investigator.md`
(perf triage) in the upstream repo.

**Note:** these skills assume they run **on the Jetson**. From a dev machine, run
their commands over the SSH recipe above.

## NOT bundled: the BSP / flashing skills — fetch one on demand

[NVIDIA/skills](https://github.com/NVIDIA/skills/tree/main/skills) ships 24 more
`jetson-*` skills covering **BSP customization, custom carrier boards, and
flashing**. They are deliberately **not vendored**: they drive a host-side
`Linux_for_Tegra` workspace (download BSP → init image/source → derive carrier →
edit DT/ODMDATA → build → promote → flash → validate), which a stock dev kit
never needs. Pull one **only** when the task really is "build/flash a custom
JetPack image" or "change a pre-flash board setting".

| Group | Skills | Use for |
|---|---|---|
| Entry / workspace | `jetson-quick-start`, `jetson-init-target`, `jetson-set-target`, `jetson-download-bsp`, `jetson-init-image`, `jetson-init-source`, `jetson-link-docs`, `jetson-generate-kb`, `jetson-print-bsp-info` | Set up a target profile + BSP workspace on the host PC; inspect an unpacked `Linux_for_Tegra`. |
| Carrier board | `jetson-derive-carrier`, `jetson-customize-pinmux`, `jetson-customize-uphy`, `jetson-customize-pcie`, `jetson-customize-usb`, `jetson-customize-camera`, `jetson-customize-mgbe` | Custom carrier bring-up: pinmux, UPHY lanes, PCIe, USB, MIPI/GMSL cameras, MGBE (Thor). |
| Pre-flash tuning | `jetson-customize-nvpmodel`, `jetson-customize-clocks`, `jetson-customize-fan`, `jetson-optimize-memory` | Power modes, clock caps/DVFS, fan curves, DRAM reclamation via BCT/reserved-memory — **pre-flash**, not live tuning (live tuning = `jetson-inference-mem-tune` / `jetson-headless-mode` above). |
| Build → flash | `jetson-build-source`, `jetson-promote-image`, `jetson-flash-image`, `jetson-validate-image` | Rebuild DT/kernel/OOT modules, stage them into the image, flash a DUT in RCM mode, validate the flashed board. |

Fetch just the one you need (sparse checkout, no full clone):

```bash
S=jetson-flash-image   # ← the skill you want
git clone --depth 1 --filter=blob:none --sparse https://github.com/NVIDIA/skills.git /tmp/nvskills \
  && git -C /tmp/nvskills sparse-checkout set "skills/$S" && ls "/tmp/nvskills/skills/$S"
```

If a BSP skill turns out to be used repeatedly, vendor it into
`references/nvidia-jetson-device-skills/` and add it to the bundled table above.
