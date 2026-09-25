#!/usr/bin/env python3
"""Render an invoice payload (JSON) to PDF.

    make_invoice.py payload.json --out invoice.pdf [--html-only] [--keep-html]

Reads the payload from a file or from stdin ("-"). Money is computed with Decimal
in the payload's currency and never re-derived by the template. Exits non-zero with
a plain-English message when the payload is incomplete or no renderer is available.
"""

import argparse
import datetime as _dt
import html
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path

TEMPLATE = Path(__file__).resolve().parent.parent / "assets" / "invoice.html"

# ---------------------------------------------------------------- money


def money(value):
    """Parse a number/str into Decimal. Rejects junk loudly."""
    if value is None or value == "":
        return Decimal("0")
    try:
        return Decimal(str(value).replace(",", "").replace(" ", ""))
    except Exception:
        raise SystemExit(f"not a number: {value!r}")


def q2(value):
    return value.quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)


def fmt(value, cur):
    """1234.5 -> '1,234.50 EUR' (symbol before the amount when one is given)."""
    n = f"{q2(value):,.2f}"
    sym = cur.get("symbol")
    code = cur.get("code", "")
    if sym:
        return f"{sym}{n}" + (f" {code}" if cur.get("show_code") else "")
    return f"{n} {code}".strip()


# ---------------------------------------------------------------- html bits


def esc(value):
    return html.escape(str(value if value is not None else ""))


def lines_html(value):
    """A string (with newlines) or a list of strings -> <div class=line> rows."""
    if not value:
        return ""
    items = value if isinstance(value, list) else str(value).splitlines()
    return "".join(f'<div class="line">{esc(i)}</div>' for i in items if str(i).strip())


def kv_rows(pairs, strip_dupe_suffix=False):
    """JSON cannot repeat a key, so a payload disambiguates with a '#N' suffix
    ("Intermediary Bank #2"). That suffix is a payload artefact and never prints."""
    out = []
    for k, v in pairs:
        if v in (None, "", []):
            continue
        label = re.sub(r"\s*#\d+$", "", str(k)) if strip_dupe_suffix else k
        out.append(f'<tr><td class="k">{esc(label)}</td><td class="v">{esc(v)}</td></tr>')
    return "".join(out)


def party_html(label, party):
    if not party:
        return ""
    rows = kv_rows(
        [
            (party.get("tax_id_label", "Tax ID"), party.get("tax_id")),
            (party.get("reg_no_label", "Reg. no"), party.get("reg_no")),
            ("Email", party.get("email")),
            ("Phone", party.get("phone")),
        ]
    )
    return (
        f'<div class="party">'
        f'<div class="label">{esc(label)}</div>'
        f'<div class="name">{esc(party.get("name", ""))}</div>'
        f'{lines_html(party.get("address"))}'
        f'{f"<table class=kv>{rows}</table>" if rows else ""}'
        f"</div>"
    )


# ---------------------------------------------------------------- build


def build_html(payload):
    inv = payload.get("invoice") or {}
    cur = payload.get("currency") or {}
    if isinstance(cur, str):
        cur = {"code": cur}

    missing = [k for k in ("number", "issue_date") if not inv.get(k)]
    if not payload.get("issuer", {}).get("name"):
        missing.append("issuer.name")
    if not payload.get("client", {}).get("name"):
        missing.append("client.name")
    if not payload.get("items"):
        missing.append("items")
    if missing:
        raise SystemExit("payload is missing required field(s): " + ", ".join(missing))

    # line items
    rows, subtotal = [], Decimal("0")
    for item in payload["items"]:
        qty = money(item.get("quantity", 1))
        price = money(item.get("unit_price", item.get("amount", 0)))
        amount = money(item["amount"]) if "amount" in item and "unit_price" not in item else qty * price
        subtotal += amount
        unit = item.get("unit", "")
        qty_cell = f"{qty:,.2f}".rstrip("0").rstrip(".")
        rows.append(
            "<tr>"
            f'<td class="desc">{esc(item.get("description", ""))}'
            f'{f"<span class=sub>{esc(item["note"])}</span>" if item.get("note") else ""}</td>'
            f'<td class="num">{esc(qty_cell)}{f" {esc(unit)}" if unit else ""}</td>'
            f'<td class="num">{esc(fmt(price, cur))}</td>'
            f'<td class="num">{esc(fmt(amount, cur))}</td>'
            "</tr>"
        )

    # discount / tax / total
    sums, running = [], subtotal
    show_subtotal = False

    disc = payload.get("discount") or {}
    if disc:
        amount = (
            q2(subtotal * money(disc["percent"]) / 100) if disc.get("percent") is not None
            else money(disc.get("amount", 0))
        )
        if amount:
            label = disc.get("label") or (
                f"Discount ({money(disc['percent']).normalize()}%)" if disc.get("percent") is not None
                else "Discount"
            )
            sums.append((label, "-" + fmt(amount, cur)))
            running -= amount
            show_subtotal = True

    tax = payload.get("tax") or {}
    rate = money(tax.get("rate_percent", 0)) if tax else Decimal("0")
    if tax and (rate or tax.get("amount") is not None):
        amount = money(tax["amount"]) if tax.get("amount") is not None else q2(running * rate / 100)
        label = tax.get("label") or f"VAT {rate.normalize()}%"
        sums.append((label, fmt(amount, cur)))
        running += amount
        show_subtotal = True

    total = q2(running)
    sum_rows = ""
    if show_subtotal:
        sum_rows += f'<tr><td class="k">Subtotal</td><td class="v">{esc(fmt(subtotal, cur))}</td></tr>'
    for label, value in sums:
        sum_rows += f'<tr><td class="k">{esc(label)}</td><td class="v">{esc(value)}</td></tr>'
    sum_rows += f'<tr class="grand"><td class="k">{esc(payload.get("total_label", "Total due"))}</td><td class="v">{esc(fmt(total, cur))}</td></tr>'
    if inv.get("due_date"):
        sum_rows += f'<tr class="due"><td class="k">Due {esc(inv["due_date"])}</td><td class="v"></td></tr>'

    # header
    meta = kv_rows(
        [
            ("Invoice no.", inv.get("number")),
            ("Issue date", inv.get("issue_date")),
            ("Due date", inv.get("due_date")),
            ("Period", inv.get("period")),
            ("PO / Ref", inv.get("reference")),
        ]
    )
    logo = payload.get("logo_path")
    logo_html = ""
    if logo and Path(logo).exists():
        logo_html = f'<img class="logo" src="file://{esc(Path(logo).resolve())}">'

    # payment / notes blocks
    payment = payload.get("payment") or {}
    bank = payment.get("bank") or {}
    bank_rows = kv_rows(list(bank.items()) if isinstance(bank, dict) else [], strip_dupe_suffix=True)
    blocks = ""
    if bank_rows or payment.get("terms") or payment.get("instructions"):
        blocks += '<div class="block"><div class="label">Payment details</div>'
        if payment.get("terms"):
            blocks += f'<div class="line">{esc(payment["terms"])}</div>'
        if bank_rows:
            blocks += f'<table class="kv">{bank_rows}</table>'
        if payment.get("instructions"):
            blocks += f'<div class="notes">{esc(payment["instructions"])}</div>'
        blocks += "</div>"
    if payload.get("notes"):
        blocks += f'<div class="block"><div class="label">Notes</div><div class="notes">{esc(payload["notes"])}</div></div>'

    body = (
        '<div class="head"><div>'
        f'<h1 class="title">{esc(payload.get("doc_title", "Invoice"))}</h1>'
        f'{f"<div class=subtitle>{esc(inv["subtitle"])}</div>" if inv.get("subtitle") else ""}'
        f'<table class="meta">{meta}</table>'
        f"</div><div>{logo_html}</div></div>"
        '<div class="parties">'
        + party_html(payload.get("issuer", {}).get("label", "From"), payload.get("issuer"))
        + party_html(payload.get("client", {}).get("label", "Bill to"), payload.get("client"))
        + "</div>"
        '<table class="items"><thead><tr>'
        '<th class="desc">Description</th><th class="num">Qty</th>'
        '<th class="num">Unit price</th><th class="num">Amount</th>'
        "</tr></thead><tbody>" + "".join(rows) + "</tbody></table>"
        f'<div class="totals"><table class="sum">{sum_rows}</table></div>'
        + (f'<div class="blocks">{blocks}</div>' if blocks else "")
        + (f'<div class="footer">{esc(payload["footer"])}</div>' if payload.get("footer") else "")
    )

    template = TEMPLATE.read_text(encoding="utf-8")
    doc = (
        template.replace("__PAGE_SIZE__", payload.get("page_size", "A4"))
        .replace("__TITLE__", esc(f'{payload.get("doc_title", "Invoice")} {inv.get("number", "")}'.strip()))
        .replace("__BODY__", body)
    )
    return doc, total, cur


# ---------------------------------------------------------------- render

CHROME_CANDIDATES = [
    "google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge",
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
]


def find_chrome():
    env = os.environ.get("CHROME")
    if env and (shutil.which(env) or Path(env).exists()):
        return env
    for cand in CHROME_CANDIDATES:
        found = shutil.which(cand) if "/" not in cand else (cand if Path(cand).exists() else None)
        if found:
            return found
    return None


def looks_like_pdf(path):
    path = Path(path)
    if not path.exists() or path.stat().st_size < 1024:
        return False
    with path.open("rb") as fh:
        return fh.read(5) == b"%PDF-"


def render_chrome(chrome, html_path, pdf_path, timeout=90):
    """Chrome writes the PDF and then often refuses to exit, so poll for the file
    and kill it once the output is complete rather than waiting on the process."""
    Path(pdf_path).unlink(missing_ok=True)
    with tempfile.TemporaryDirectory() as profile:
        proc = subprocess.Popen(
            [chrome, "--headless=new", "--disable-gpu", "--no-sandbox",
             f"--user-data-dir={profile}", "--no-pdf-header-footer",
             f"--print-to-pdf={pdf_path}", f"file://{html_path}"],
            stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
        )
        deadline = time.monotonic() + timeout
        stable = 0
        try:
            while time.monotonic() < deadline:
                if looks_like_pdf(pdf_path):
                    stable += 1
                    if stable >= 3:  # size settled across ~0.9s — the write finished
                        break
                if proc.poll() is not None:
                    break
                time.sleep(0.3)
        finally:
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
    return looks_like_pdf(pdf_path)


def render_pdf(html_path, pdf_path):
    """Chrome headless first, then weasyprint, then wkhtmltopdf. Returns the tool used."""
    chrome = find_chrome()
    if chrome and render_chrome(chrome, html_path, pdf_path):
        return "chrome"

    try:
        from weasyprint import HTML  # noqa: PLC0415

        HTML(filename=str(html_path)).write_pdf(str(pdf_path))
        return "weasyprint"
    except ImportError:
        pass

    if shutil.which("wkhtmltopdf"):
        subprocess.run(["wkhtmltopdf", "--enable-local-file-access", str(html_path), str(pdf_path)],
                       check=True, capture_output=True)
        return "wkhtmltopdf"

    raise SystemExit(
        "no PDF renderer found. Install one of:\n"
        "  - Google Chrome / Chromium (set $CHROME to its binary if it is in an unusual place)\n"
        "  - wkhtmltopdf\n"
        "  - python -m pip install weasyprint"
    )


def launch(argv):
    subprocess.Popen(argv, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                     shell=(os.name == "nt"))


def open_file(path):
    launch(["open", str(path)] if sys.platform == "darwin"
           else ["start", "", str(path)] if os.name == "nt"
           else ["xdg-open", str(path)])


def reveal_file(path):
    """Select the file in the file manager. Only macOS and Windows can select a
    specific file; elsewhere the containing folder is opened instead."""
    path = Path(path)
    if sys.platform == "darwin":
        launch(["open", "-R", str(path)])
    elif os.name == "nt":
        launch(["explorer", f"/select,{path}"])
    else:
        launch(["xdg-open", str(path.parent)])


def main():
    ap = argparse.ArgumentParser(description="Render an invoice payload to PDF.")
    ap.add_argument("payload", help='JSON payload file, or "-" for stdin')
    ap.add_argument("--out", help="output .pdf path (default: alongside the payload)")
    ap.add_argument("--html-only", action="store_true", help="write the .html and stop")
    ap.add_argument("--keep-html", action="store_true", help="keep the intermediate .html")
    ap.add_argument("--open", action="store_true", dest="open_after",
                    help="open the finished PDF in the default viewer/browser")
    ap.add_argument("--reveal", action="store_true",
                    help="show the finished PDF in the file manager (Finder/Explorer)")
    ap.add_argument("--force", action="store_true",
                    help="overwrite an invoice that already exists at --out")
    args = ap.parse_args()

    raw = sys.stdin.read() if args.payload == "-" else Path(args.payload).read_text(encoding="utf-8")
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise SystemExit(f"payload is not valid JSON: {exc}")

    doc, total, cur = build_html(payload)

    number = str((payload.get("invoice") or {}).get("number", "invoice"))
    default = Path(args.payload).with_suffix(".pdf") if args.payload != "-" else Path(f"{number}.pdf")
    pdf_path = Path(args.out).expanduser().resolve() if args.out else default.resolve()

    # An invoice that exists has been issued: re-rendering it silently is how a
    # second document goes out under a number the client already has.
    if pdf_path.exists() and not args.force and not args.html_only:
        stat = pdf_path.stat()
        when = _dt.datetime.fromtimestamp(stat.st_mtime).strftime("%Y-%m-%d %H:%M")
        raise SystemExit(
            f"ALREADY INVOICED — {pdf_path}\n"
            f"  written {when}, {stat.st_size:,} bytes\n"
            f"Invoice {number} exists, so nothing was rendered and nothing was changed.\n"
            f"Open that file rather than making a second one. To deliberately replace it, "
            f"re-run with --force."
        )

    pdf_path.parent.mkdir(parents=True, exist_ok=True)
    html_path = pdf_path.with_suffix(".html")
    html_path.write_text(doc, encoding="utf-8")

    if args.html_only:
        print(f"HTML  {html_path}")
        return

    tool = render_pdf(html_path, pdf_path)
    if not args.keep_html:
        html_path.unlink(missing_ok=True)

    print(f"PDF    {pdf_path}")
    print(f"Total  {fmt(total, cur)}")
    print(f"Via    {tool}")

    for flag, action, done in ((args.open_after, open_file, "Opened in the default viewer"),
                               (args.reveal, reveal_file, "Revealed in the file manager")):
        if not flag:
            continue
        try:
            action(pdf_path)
            print(done)
        except OSError as exc:
            print(f"{done.split()[0].lower()} failed ({exc}) — the PDF is written, "
                  f"open it by hand", file=sys.stderr)


if __name__ == "__main__":
    main()
