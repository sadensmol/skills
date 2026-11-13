# JetPack 7.2 LLM/GPU findings on this board (verified)

Read this when you run a model or a CUDA workload on the GPU, pick an Ollama image or
PyTorch wheels, or a model falls back to CPU or deadlocks.

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
