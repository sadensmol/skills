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

**Address, SSH user, container names and deploy layout are per-workspace — never
hardcoded here.** Take them from the workspace's own skill (the `<workspace>-*`
plugin loaded for the session), from an env var, or ask. Below, `$JETSON_HOST`
means that workspace's `<user>@<host>` and `<ctr>` its container name.

## Connecting over SSH (and debugging without wedging it)

**✅ Passwordless SSH is normally already set up.** A key is installed on the board, so a bare
`ssh "$JETSON_HOST" '<cmd>'` connects with **no password** — no `expect`, no
`sshpass`. This is the default path; use it. `scp`/`rsync` to the board work the same
way with no password.

**⚠️ A container env change needs a RECREATE, never `docker restart`.** Full rule and
the `IMAGE_TAG` caveat: `references/ssh-and-debug.md`.

**⚠️ Still ASK FIRST — permission (MANDATORY, every session).** Before you run ANY
command that reaches the board (SSH, `scp`, `rsync`), ask the user for explicit
permission — name the host and what you intend to run (e.g. "read the app
container logs"). Wait for a clear yes. If the user declines, stop — do not
connect. A workspace skill may grant standing permission for its own board; that
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
- Set the shell call's own timeout low too (30–60 s). A 200 s tool timeout means
  200 s of nothing.
- **Never run an unbounded reader:** `docker logs` without `--tail`, `journalctl`
  without `--since`/`-n`, `find /`, `du -sh` on a mount. Cap them at the source.
- A command that times out is DATA — "this hangs" localises the fault (usually
  dead media or thrash). Do not silently retry it; check `uptime` and
  `ps -eo state,pid,comm | awk '$1 ~ /D/'` first.

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
- On 8 GB, never hold two models resident at once (`OLLAMA_MAX_LOADED_MODELS=1`) —
  full text in `references/jp72-llm-gpu-findings.md`.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| The board is unreachable by IP, the SSH key is missing, you need `sudo` or base64 quoting, you change a container's env, or you want the common read-only checks | `references/ssh-and-debug.md` |
| You run a model or CUDA workload on the GPU, pick an Ollama image or PyTorch wheels, or a model falls back to CPU / deadlocks | `references/jp72-llm-gpu-findings.md` |
| The task is to build/flash a custom JetPack image, bring up a carrier board, or change a pre-flash setting | `references/bsp-flashing-skills.md` |
| Any task a bundled NVIDIA skill covers (table below) | its `SKILL.md` under `references/nvidia-jetson-device-skills/` |

## Bundled NVIDIA device skills

Apache-2.0, from [NVIDIA-AI-IOT/jetson-device-skills](https://github.com/NVIDIA-AI-IOT/jetson-device-skills)
— also published under [NVIDIA/skills](https://github.com/NVIDIA/skills/tree/main/skills).
Only the skill bodies (`SKILL.md`, `references/`, `scripts/`) and the `LICENSE` are
vendored here, in `references/nvidia-jetson-device-skills/`. Load the one that fits the task —
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

**Note:** these skills assume they run **on the Jetson**. From a dev machine, run
their commands over the SSH recipe above.

The 24 BSP / flashing `jetson-*` skills are **not bundled** — fetch one on demand only
for a custom-image or pre-flash task: `references/bsp-flashing-skills.md`.
