"""Tests for plan-links.py: python3 -m unittest discover -s <plan skill>/scripts"""
import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("plan-links.py")
spec = importlib.util.spec_from_file_location("plan_links", SCRIPT)
pl = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pl)

PLAN = """\
# Plan

## Task

### Problem facts
| Id | Fact |
|---|---|
| F1 | prod detach failed |
| ~~F2~~ | dropped |

## Plan

#### S3 — Schema check (CI), forces the rule
- **what:** see [F1] and [S3](#old-anchor); bare S3 or other archive stays text.
- skip: `[F1]` and \\[F1\\] and https://x.io/[F1] and [doc](a.md) and [[F1]]
~~S4 — Lock rehearsals~~ dropped: no images

```mermaid
flowchart LR
  A["[F1]"]
```

#### Setup
#### Setup

## Decisions

### U1 — Partition by default (2026-11-11)
- asked: 2026-11-11: "keep [F1] as typed
  and [S3] here too"
- asked by: user → writer
- decided: I decided to apply [S4].
"""


def write(text: str) -> Path:
    d = Path(tempfile.mkdtemp())
    p = d / "PLAN.md"
    p.write_text(text, encoding="utf-8")
    return p


def run(path: Path, mode: str, stdin: str = "") -> subprocess.CompletedProcess:
    return subprocess.run([sys.executable, str(SCRIPT), str(path), mode], input=stdin,
                          capture_output=True, text=True)


class SlugTest(unittest.TestCase):
    def test_matches_plannotator_rules(self):
        self.assertEqual(pl.slug("S3 — Schema check (CI), forces the rule"),
                         "s3-schema-check-ci-forces-the-rule")
        self.assertEqual(pl.slug("U14 — Key-change cost; `x` *y* (2026-11-11)"),
                         "u14-key-change-cost-x-y-2026-11-11")
        self.assertEqual(pl.slug("Q&A"), "q-a")

    def test_repeated_heading_gets_a_suffix(self):
        plan = pl.Plan(PLAN.split("\n"))
        self.assertIn("setup", plan.anchors)
        self.assertIn("setup-1", plan.anchors)


class DefinitionsTest(unittest.TestCase):
    def test_heading_table_row_and_struck_line(self):
        plan = pl.Plan(PLAN.split("\n"))
        self.assertEqual(plan.target("S3"), "s3-schema-check-ci-forces-the-rule")
        self.assertEqual(plan.target("F1"), "problem-facts")
        self.assertEqual(plan.target("F2"), "problem-facts")
        self.assertEqual(plan.target("S4"), "s3-schema-check-ci-forces-the-rule")
        self.assertEqual(plan.target("U1"), "u1-partition-by-default-2026-11-11")


class FixTest(unittest.TestCase):
    def test_fix_links_retargets_and_keeps_other_bytes(self):
        p = write(PLAN)
        r = run(p, "fix")
        self.assertEqual(r.returncode, 0, r.stderr)
        got = p.read_text(encoding="utf-8").split("\n")
        want = PLAN.split("\n")
        self.assertEqual(len(got), len(want))
        changed = [n for n, (a, b) in enumerate(zip(want, got)) if a != b]
        self.assertEqual([want[n][:12] for n in changed], ["- **what:** ", "- decided: I"])
        line = got[changed[0]]
        self.assertIn("[F1](#problem-facts)", line)
        self.assertIn("[S3](#s3-schema-check-ci-forces-the-rule)", line)
        self.assertIn("bare S3 or other", line)
        self.assertIn("[S4](#s3-schema-check-ci-forces-the-rule)", got[changed[1]])

    def test_skips_code_urls_links_escapes_fences_and_user_words(self):
        p = write(PLAN)
        run(p, "fix")
        text = p.read_text(encoding="utf-8")
        for kept in ("`[F1]`", "\\[F1\\]", "https://x.io/[F1]", "[doc](a.md)", "[[F1]]",
                     'A["[F1]"]', 'keep [F1] as typed', "  and [S3] here too"):
            self.assertIn(kept, text)

    def test_fix_is_idempotent_and_check_passes(self):
        p = write(PLAN)
        run(p, "fix")
        once = p.read_text(encoding="utf-8")
        self.assertIn("0 link(s) updated", run(p, "fix").stdout)
        self.assertEqual(p.read_text(encoding="utf-8"), once)
        self.assertEqual(run(p, "check").returncode, 0)

    def test_check_is_read_only_and_asks_for_fix(self):
        p = write(PLAN)
        r = run(p, "check")
        self.assertEqual(r.returncode, 1)
        self.assertIn("need `fix`", r.stderr)
        self.assertEqual(p.read_text(encoding="utf-8"), PLAN)


class ErrorTest(unittest.TestCase):
    def test_unknown_id_dead_anchor_and_two_definitions(self):
        text = PLAN + "\n## More\n| S3 | again |\n\nsee [K9] and [x](#nowhere) and [S3]\n"
        p = write(text)
        r = run(p, "fix")
        self.assertEqual(r.returncode, 1)
        self.assertIn("[K9] has no definition", r.stderr)
        self.assertIn("dead anchor #nowhere", r.stderr)
        self.assertIn("S3 has 2 definitions", r.stderr)
        self.assertIn("see [K9] and [x](#nowhere) and [S3]", p.read_text(encoding="utf-8"))


class ResolveTest(unittest.TestCase):
    def test_resolve_filters_stdin_and_leaves_the_plan(self):
        p = write(PLAN)
        entry = "- decided: I decided to run [S3].\n- question: what is [F1]?\n"
        r = run(p, "resolve", entry)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout, "- decided: I decided to run "
                         "[S3](#s3-schema-check-ci-forces-the-rule).\n- question: what is [F1]?\n")
        self.assertEqual(p.read_text(encoding="utf-8"), PLAN)

    def test_resolve_warns_but_passes_an_unknown_id(self):
        p = write(PLAN)
        r = run(p, "resolve", "- where: [K9]")
        self.assertEqual(r.returncode, 0)
        self.assertEqual(r.stdout, "- where: [K9]")
        self.assertIn("[K9] has no definition", r.stderr)


if __name__ == "__main__":
    unittest.main()
