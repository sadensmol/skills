#!/usr/bin/env python3
"""Write a self-contained PDF viewer page (pdf.js) in the plannotator theme.

Usage: python3 -I pdf-page.py <pdf> <out.html> --theme dark|light|system
  The theme comes from scripts/plannotator-theme.sh. Open the page with --js, as a page
  companion (agterm references/html-pages.md).

Zoom: the −/+/Fit buttons, Cmd/Ctrl + = - 0, Cmd/Ctrl + wheel, and the WebKit trackpad
pinch (gesturestart/gesturechange). Links: internal ones jump in the page; http(s) ones
open in a new window, which agterm sends through its open-in-browser prompt.
In dark mode the PDF canvas is inverted, images included; for original colors rebuild
with --theme light.
Assets: the PDF is embedded inline; pdf.js loads from cdnjs (needs network); the PDF
bytes are never sent. When pdf.js, its viewer or its worker fails, the page shows the error.
After the render, document.title is "<pdf stem> — N pages".

Python 3, stdlib only.
"""

from __future__ import annotations

import argparse
import base64
import html
import json
import sys
from pathlib import Path
from string import Template

PALETTE = Path(__file__).resolve().parent / "plannotator-palette.css"
THEMES = ("dark", "light", "system")
PDFJS = "https://cdnjs.cloudflare.com/ajax/libs/pdf.js/3.11.174"

PAGE = Template("""<!doctype html>
<html lang="en" data-theme="$theme">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="$scheme">
<title>$title</title>
<link rel="stylesheet" href="$pdfjs/pdf_viewer.min.css">
<style>
$palette
html,body{margin:0;height:100%;background:var(--bg);color:var(--fg);font:13px -apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif}
#bar{position:fixed;top:0;left:0;right:0;height:36px;display:flex;gap:6px;align-items:center;padding:0 10px;background:var(--q);border-bottom:1px solid var(--line);z-index:10}
#bar button{background:transparent;color:var(--fg);border:1px solid var(--line);border-radius:4px;padding:3px 9px;cursor:pointer;font:inherit}
#bar span{color:var(--muted);margin-left:6px}
#viewerContainer{position:absolute;top:37px;left:0;right:0;bottom:0;overflow:auto;background:var(--bg)}
.pdfViewer .page{border:0;margin:8px auto}
#err{padding:16px;color:var(--partial);white-space:pre-wrap}
:root[data-theme=dark] .pdfViewer .page{background:var(--bg)}
:root[data-theme=dark] .pdfViewer .page canvas{filter:invert(1) hue-rotate(180deg)}
@media (prefers-color-scheme: dark){
:root[data-theme=system] .pdfViewer .page{background:var(--bg)}
:root[data-theme=system] .pdfViewer .page canvas{filter:invert(1) hue-rotate(180deg)}
}
</style>
</head>
<body>
<div id="bar"><button id="out" title="Zoom out (Cmd -)">−</button><button id="in" title="Zoom in (Cmd =)">+</button>
<button id="fit" title="Fit width (Cmd 0)">Fit</button><span id="zoom"></span><span id="pages"></span></div>
<div id="viewerContainer"><div id="viewer" class="pdfViewer"></div></div>
<script src="$pdfjs/pdf.min.js"></script>
<script src="$pdfjs/pdf_viewer.min.js"></script>
<script>
(async () => {
  const container = document.getElementById("viewerContainer");
  const fail = msg => {
    const err = document.createElement("div");
    err.id = "err";
    err.textContent = msg;
    container.replaceChildren(err);
  };
  try {
    if (typeof pdfjsLib === "undefined" || typeof pdfjsViewer === "undefined") {
      fail("pdf.js did not load from cdnjs ($pdfjs). The page needs network access.");
      return;
    }
    const WORKER = "$pdfjs/pdf.worker.min.js";
    try {
      const res = await fetch(WORKER);
      if (!res.ok) throw new Error("HTTP " + res.status);
      const src = URL.createObjectURL(new Blob([await res.text()], { type: "text/javascript" }));
      pdfjsLib.GlobalWorkerOptions.workerPort = new Worker(src);
    } catch (e) {
      pdfjsLib.GlobalWorkerOptions.workerSrc = WORKER;
    }
    const eventBus = new pdfjsViewer.EventBus();
    const linkService = new pdfjsViewer.PDFLinkService({ eventBus, externalLinkTarget: pdfjsViewer.LinkTarget.BLANK });
    const viewer = new pdfjsViewer.PDFViewer({ container, eventBus, linkService });
    linkService.setViewer(viewer);
    const zoomEl = document.getElementById("zoom"), pagesEl = document.getElementById("pages");
    eventBus.on("pagesinit", () => { viewer.currentScaleValue = "page-width"; });
    eventBus.on("scalechanging", e => { zoomEl.textContent = Math.round(e.scale * 100) + "%"; });
    eventBus.on("pagechanging", e => { pagesEl.textContent = "page " + e.pageNumber + " / " + viewer.pagesCount; });
    const setScale = s => { viewer.currentScale = Math.min(6, Math.max(0.25, s)); };
    const zoom = f => setScale(viewer.currentScale * f);
    const fit = () => { viewer.currentScaleValue = "page-width"; };
    document.getElementById("in").onclick = () => zoom(1.15);
    document.getElementById("out").onclick = () => zoom(1 / 1.15);
    document.getElementById("fit").onclick = fit;
    container.addEventListener("wheel", e => {
      if (!(e.ctrlKey || e.metaKey)) return;
      e.preventDefault();
      zoom(e.deltaY < 0 ? 1.08 : 1 / 1.08);
    }, { passive: false });
    let pinchStart = 1;
    document.addEventListener("gesturestart", e => { e.preventDefault(); pinchStart = viewer.currentScale; });
    document.addEventListener("gesturechange", e => { e.preventDefault(); setScale(pinchStart * e.scale); });
    window.addEventListener("keydown", e => {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key === "=" || e.key === "+") { e.preventDefault(); zoom(1.15); }
      else if (e.key === "-") { e.preventDefault(); zoom(1 / 1.15); }
      else if (e.key === "0") { e.preventDefault(); fit(); }
    });
    const bin = atob("$data");
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    const doc = await pdfjsLib.getDocument({ data: bytes }).promise;
    const pageWord = doc.numPages === 1 ? " page" : " pages";
    eventBus.on("pagerendered", () => { document.title = $stem + " — " + doc.numPages + pageWord; }, { once: true });
    viewer.setDocument(doc);
    linkService.setDocument(doc, null);
    pagesEl.textContent = "page 1 / " + doc.numPages;
  } catch (e) {
    fail("Cannot render the PDF: " + e);
  }
})();
</script>
</body>
</html>
""")


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("pdf", type=Path)
    ap.add_argument("out", type=Path)
    ap.add_argument("--theme", required=True, choices=THEMES,
                    help="plannotator theme mode, from scripts/plannotator-theme.sh")
    args = ap.parse_args(argv)
    try:
        page = PAGE.substitute(
            theme=args.theme,
            scheme="light dark" if args.theme == "system" else args.theme,
            title=html.escape(args.pdf.stem),
            stem=json.dumps(args.pdf.stem).replace("<", "\\u003c"),
            pdfjs=PDFJS,
            palette=PALETTE.read_text(encoding="utf-8"),
            data=base64.b64encode(args.pdf.read_bytes()).decode("ascii"),
        )
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(page, encoding="utf-8")
    except OSError as exc:
        print(f"pdf-page: {exc}", file=sys.stderr)
        return 2
    print(f"WROTE {args.out} (theme {args.theme})")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
