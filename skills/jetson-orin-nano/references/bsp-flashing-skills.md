# NOT bundled: the BSP / flashing skills — fetch one on demand

Read this when the task is to build or flash a custom JetPack image, bring up a custom
carrier board, or change a pre-flash board setting.

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
| Pre-flash tuning | `jetson-customize-nvpmodel`, `jetson-customize-clocks`, `jetson-customize-fan`, `jetson-optimize-memory` | Power modes, clock caps/DVFS, fan curves, DRAM reclamation via BCT/reserved-memory — **pre-flash**, not live tuning (live tuning = `jetson-inference-mem-tune` / `jetson-headless-mode` in the bundled table in `SKILL.md`). |
| Build → flash | `jetson-build-source`, `jetson-promote-image`, `jetson-flash-image`, `jetson-validate-image` | Rebuild DT/kernel/OOT modules, stage them into the image, flash a DUT in RCM mode, validate the flashed board. |

Fetch just the one you need (sparse checkout, no full clone):

```bash
S=jetson-flash-image   # ← the skill you want
git clone --depth 1 --filter=blob:none --sparse https://github.com/NVIDIA/skills.git .scratch/nvskills \
  && git -C .scratch/nvskills sparse-checkout set "skills/$S" && ls ".scratch/nvskills/skills/$S"
```

If a BSP skill turns out to be used repeatedly, vendor it into
`references/nvidia-jetson-device-skills/` and add it to the bundled table in `SKILL.md`.
