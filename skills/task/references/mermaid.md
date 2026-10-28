# Mermaid in markdown — validate every block before showing it

Any `.md` you write or edit with a ```` ```mermaid ```` block (`PLAN.md`, a design doc, an
R&D report) has **every** block validated before you open plannotator on it, hand it
over, or commit it. A broken block renders as a red "Mermaid Error" box and wastes the
gate round-trip.

Validate by rendering, not by reading. `mmdc` (`@mermaid-js/mermaid-cli`) is the
checker; if it is missing, install it or say you could not validate.

```bash
python3 - <<'EOF'
import pathlib, subprocess, tempfile, os, sys
doc = pathlib.Path("<file>.md")
lines = doc.read_text().splitlines()
blocks, cur, start = [], None, 0
for i, l in enumerate(lines, 1):
    if l.strip() == "```mermaid": cur, start = [], i
    elif cur is not None and l.strip() == "```": blocks.append((start, "\n".join(cur))); cur = None
    elif cur is not None: cur.append(l)
d, bad = tempfile.mkdtemp(), 0
for n, (line, body) in enumerate(blocks, 1):
    f = os.path.join(d, f"b{n}.mmd"); open(f, "w").write(body)
    r = subprocess.run(["mmdc", "-i", f, "-o", os.path.join(d, f"b{n}.svg")],
                       capture_output=True, text=True)
    print(("OK  " if r.returncode == 0 else "FAIL"), f"block {n} ({doc}:{line})")
    if r.returncode: bad += 1; print((r.stderr or r.stdout).strip()[:400])
sys.exit(1 if bad else 0)
EOF
```

Fix every `FAIL` and re-run until it exits 0. Only then open plannotator.

The two that bite most, both in `sequenceDiagram` message text:

| Breaks | Why | Write instead |
|---|---|---|
| `A->>B: Lock(); delete x; Unlock()` | `;` is a statement separator | commas, or `<br/>` |
| `A->>A: block on <-ch` | `<` breaks the parser; `&lt;` does not help | spell it out: `receive from ch` |

Also: `end` alone in a label closes a block; `#` starts a comment; a `participant`
alias with `-` or `:` needs quoting. Put punctuation-heavy text in a `Note over`.
