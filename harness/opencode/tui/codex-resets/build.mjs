// Compiles codex-resets.tsx into index.js, the file OpenCode loads.
//
// OpenCode's plugin loader transforms JSX with Bun's default, which is not what
// Solid needs, so the plugin ships compiled. Re-run after editing the source:
//
//   npm install && npm run build
import { transformAsync } from "@babel/core"
import { readFile, writeFile } from "node:fs/promises"
import { fileURLToPath } from "node:url"

const here = (name) => fileURLToPath(new URL(name, import.meta.url))
const source = here("codex-resets.tsx")

const result = await transformAsync(await readFile(source, "utf8"), {
  filename: source,
  babelrc: false,
  configFile: false,
  presets: [
    ["@babel/preset-typescript", { isTSX: true, allExtensions: true }],
    ["babel-preset-solid", { generate: "universal", moduleName: "@opentui/solid" }],
  ],
})
if (!result?.code) throw new Error(`babel returned no code for ${source}`)
await writeFile(here("index.js"), result.code)
console.log("built index.js")
