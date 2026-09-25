---
name: sadensmol-invoice
description: "Generate an invoice as a PDF — build the payload, compute the totals, render the document. Use when the user asks to create/make/generate/issue an invoice or a bill, to invoice a client for a month or a period, to re-issue or amend an invoice, or to produce a proforma/credit note. Resolves WHOSE data to use from the active workspace: a `<project>-invoice` skill owns that project's billing data and always wins; a plain personal session falls back to the local profile. Owns the mechanics (payload schema, numbering, money math, rendering, output location) and never holds anyone's real billing data."
---

# Invoice

Turn billing facts into a PDF. This skill is the **generic layer**: it owns the payload
shape, the arithmetic, the renderer and the output rules. It holds **no** real party,
bank or rate data — that comes from the resolved data source below.

## 1. Resolve the data source FIRST (MUST FOLLOW)

Never start assembling an invoice before you know whose data it is built from.

```bash
if [ -n "${AGTERM_WORKSPACE_ID:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  agtermctl tree --json 2>/dev/null | python3 -c "import sys,json,os;d=json.load(sys.stdin);w=os.environ.get('AGTERM_WORKSPACE_ID');print(next((x.get('name') for x in d.get('result',{}).get('tree',{}).get('workspaces',[]) if x.get('id')==w),'(unknown)'))"
else
  echo "(not in agterm)"
fi
pwd
```

The **agterm workspace name is authoritative** when present — a session lives under a
workspace named after its project, and that stays right even when the cwd has wandered
into a skills checkout or a worktree. The cwd is the fallback: normalise it
(`pwd | sed 's|/work_|/|g'`) and match it against the local routes file
(`~/.agents/sadensmol-router-routes.json`, or `~/.claude/sadensmol-router-routes.json`)
whose entries are `{ "cwd": "<substring>", "skill": "<plugin>:<skill>" }`. The plugin
namespace before the `:` is `<project>`.

Then, in order:

1. **A `<project>-invoice` skill exists → load it and use ITS data, always.** Working in
   a project's workspace means invoicing that project. Its skill carries the issuer, the
   client, the bank details, the default line items, the currency and the numbering
   series. **Project data wins over anything in this file, over the personal profile, and
   over what you remember from an earlier session.**
2. **No project invoice skill → the personal profile.** Read
   `~/.agents/sadensmol-invoice.json` (fallback `~/.claude/sadensmol-invoice.json`) — a
   local, unpublished file holding the same fields. Absent → go to 3.
3. **Nothing resolves → ASK the user**, once, for the missing fields (§2). Offer to save
   the stable ones into the personal profile so the next invoice needs no questions.

**Never mix sources.** Do not take the issuer from a project skill and the bank details
from the personal profile because one of them looked incomplete. Say which source is
short and ask.

## 2. Never invent invoice data (MUST FOLLOW)

An invoice is a financial document someone acts on. A plausible-looking wrong IBAN, tax
id, company number or address is worse than a missing one: it gets sent, it gets paid to
the wrong place, or it fails an audit.

- **Never guess, autocomplete, or "reconstruct" any of:** legal names, addresses,
  registration/tax/VAT numbers, IBANs, SWIFT/BIC codes, account numbers, rates, amounts,
  or the tax treatment.
- **Never carry a value over from an example, a template, or another client's invoice.**
- Missing a required field → ask for that field, and stop until it is answered.
- The user's stated amount is the amount. Do not "correct" it, re-derive it from a rate,
  or add a tax line they did not ask for.

Required before rendering: issuer name, client name, invoice number, issue date,
currency, and at least one line item with an amount. Everything else is optional.

## 3. Numbering and dates

- **The number comes from the data source's series**, and must be unique. Check the
  output directory for the highest existing number in that series and take the next one;
  when the series is date-based, follow the documented format exactly.
- **Never reuse a number.** A corrected invoice is either a new number or an explicit
  credit note — ask which, never silently re-render over an issued file.
- Dates are ISO `YYYY-MM-DD` unless the data source says otherwise. The due date comes
  from the payment terms (issue date + N days); do not invent terms.
- **The issue date is the day the invoice is actually raised, and is never in the
  future.** Do not post-date it to the end of the period being billed — the period field
  carries that. A billing rule phrased as "invoice at the end of the month" describes
  *when* it is raised, not the date to print; read it that way unless the data source
  explicitly says the issue date is the month end.
- Ask which period is being billed when it is not obvious, rather than assuming "last
  month".

## 4. The payload

Write the payload as JSON, then render it. Only `issuer.name`, `client.name`,
`invoice.number`, `invoice.issue_date` and `items` are required.

```json
{
  "doc_title": "Invoice",
  "currency": {"code": "EUR", "symbol": "€"},
  "invoice": {"number": "…", "issue_date": "YYYY-MM-DD", "due_date": "YYYY-MM-DD",
              "period": "…", "reference": "…", "subtitle": "…"},
  "issuer": {"label": "From", "name": "…", "address": ["…"], "email": "…",
             "tax_id": "…", "tax_id_label": "…", "reg_no": "…", "reg_no_label": "…"},
  "client": {"label": "Bill to", "name": "…", "address": ["…"], "tax_id": "…"},
  "items": [{"description": "…", "note": "…", "quantity": 1, "unit": "h", "unit_price": 0}],
  "discount": {"percent": 0},
  "tax": {"rate_percent": 0, "label": "…"},
  "payment": {"terms": "…", "bank": {"<label>": "<value>"}, "instructions": "…"},
  "notes": "…",
  "footer": "…",
  "logo_path": "…"
}
```

Notes on the shape:

- An item is either `quantity` × `unit_price`, or a flat `{"description", "amount"}`.
- `payment.bank` is a free-form ordered map — whatever labels the data source uses
  (`IBAN`, `SWIFT`, `Intermediary bank`, …) render as given, in order. This is how a
  project supplies correspondent/intermediary banks without a schema change. JSON cannot
  repeat a key, so a second row with the same label is written `Intermediary bank #2`;
  the renderer strips a trailing `#N` so both print identically.
- `tax` and `discount` are omitted entirely when they do not apply. A zero-rate line
  (reverse charge, out of scope) still needs an explicit `{"rate_percent": 0, "label": …,
  "amount": 0}` when the label must appear.
- Money: the script computes subtotal/discount/tax/total with `Decimal`, half-up to 2
  places. Do not pre-compute totals in the payload.

## 5. Render

```bash
scripts/make_invoice.py <payload.json> --out <output.pdf>
```

It prints the PDF path, the computed total and the renderer used.

| Flag | Effect |
|---|---|
| `--open` | open the finished PDF in the default viewer/browser |
| `--reveal` | show it in the file manager (Finder/Explorer), selected |
| `--force` | overwrite an invoice that already exists at `--out` |
| `--html-only` | write the HTML for a layout check and stop |
| `--keep-html` | keep the intermediate HTML alongside the PDF |

**Always pass `--open --reveal`.** The document is looked at, and its folder is put in
front of the user so they can attach or file it without hunting for the path.

Rendering uses headless Chrome, then WeasyPrint, then `wkhtmltopdf` — whichever is
present. None available → it says so and names the options; install one rather than
falling back to a text file. Point `$CHROME` at the browser binary when it lives
somewhere unusual.

**Where it goes:** the path the data source specifies; absent that, ask. Never write a
finished invoice into a scratch/temp directory and call it delivered.

**The filename must say it is an invoice.** A serial alone (`SG-2026-09.pdf`) is
meaningless in a folder six months later, and worse next to unrelated files — lead with
the word (`invoice-SG-2026-09.pdf`) unless the data source dictates another convention.

## 5a. Already invoiced → report it, never re-render (MUST FOLLOW)

**Check whether the invoice already exists before building anything.** An invoice that
exists has been issued; producing a second document under a number the client already
holds is the worst failure this skill has.

```bash
ls -l "<output-dir>"/*<number>* 2>/dev/null
```

The generator enforces this too — it refuses to write over an existing `--out` and exits
non-zero with `ALREADY INVOICED`, so a slip cannot silently overwrite.

When it already exists, **say so with the facts, and stop**:

- the full path, and when it was written
- the invoice number, period, issue date and total — **read them out of the existing
  PDF**, do not restate what you would have generated
- reveal it (`--reveal`'s equivalent: the platform's show-in-file-manager) and offer to
  open it

Then stop. Do **not** render, do not pass `--force`, and do not ask an open-ended "what
now?" — the user asked for an invoice and already has one; the answer is that document.
Re-render only on an explicit instruction to replace it, and only then with `--force`.

## 6. Check before handing it over (MUST FOLLOW)

Read the generated PDF back and confirm, against the payload:

- the total, and that the arithmetic matches what the user asked for
- issuer, client, invoice number, dates, currency
- the bank block — every digit of the IBAN/account, character for character
- it fits the intended page count and nothing is clipped

Then report the path and the total. State the invoice number you used and why (next in
series), so a duplicate is caught immediately.

## 7. Handling and delivery (MUST FOLLOW)

- **An invoice is private financial data.** It stays a local file. Do **not** publish it
  as an artifact, upload it, paste its bank details into a chat/issue/commit, or send it
  anywhere — not even to a service the session is already connected to.
- **Emailing or sending it to the client is the user's call**, every time. Never send on
  your own initiative, and never treat "generate the invoice" as authorisation to deliver
  it.
- **Real billing data never enters this skill, its scripts, or any public repository.** It
  lives in the project skill or the local personal profile. If you find yourself about to
  write an IBAN or a tax id into an example here, stop.
