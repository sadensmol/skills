# Rendering options

Read this when you need a render flag beyond `--open --reveal`, or no PDF renderer is found.

It prints the PDF path, the computed total and the renderer used.

| Flag | Effect |
|---|---|
| `--open` | open the finished PDF in the default viewer/browser |
| `--reveal` | show it in the file manager (Finder/Explorer), selected |
| `--force` | overwrite an invoice that already exists at `--out` |
| `--html-only` | write the HTML for a layout check and stop |
| `--keep-html` | keep the intermediate HTML alongside the PDF |

Rendering uses headless Chrome, then WeasyPrint, then `wkhtmltopdf` — whichever is
present. None available → it says so and names the options; install one rather than
falling back to a text file. Point `$CHROME` at the browser binary when it lives
somewhere unusual.
