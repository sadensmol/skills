# Invoice payload shape

Read this when you write the payload JSON for `scripts/make_invoice.py`.

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
  workspace supplies correspondent/intermediary banks without a schema change. JSON cannot
  repeat a key, so a second row with the same label is written `Intermediary bank #2`;
  the renderer strips a trailing `#N` so both print identically.
- `tax` and `discount` are omitted entirely when they do not apply. A zero-rate line
  (reverse charge, out of scope) still needs an explicit `{"rate_percent": 0, "label": …,
  "amount": 0}` when the label must appear.
- Money: the script computes subtotal/discount/tax/total with `Decimal`, half-up to 2
  places. Do not pre-compute totals in the payload.
