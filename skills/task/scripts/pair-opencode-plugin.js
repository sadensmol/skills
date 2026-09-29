// OpenCode side of pair mode (../pair.md), loaded by the reviewer config pair.py writes.
//
// pair.py feeds the reviewer with prompt_async into the session bound in
// .pair/config.json. A /new in the pane starts another session: it has no pair
// context and gets none of the peer messages, so the pane looks stuck. When the
// user types into such a session, this plugin forwards the text to the pair
// session, puts the pane back on it, and drops the stray session.
import { readFileSync } from "node:fs"
import { join } from "node:path"

export const PairPanePlugin = async ({ directory, serverUrl }) => {
  const pairConfig = () => {
    try {
      return JSON.parse(readFileSync(join(directory, ".pair", "config.json"), "utf8"))
    } catch {
      return null
    }
  }
  const call = async (method, path, body) => {
    try {
      const res = await fetch(new URL(path, serverUrl), {
        method,
        headers: { "content-type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      const text = await res.text()
      return res.ok && text ? JSON.parse(text) : null
    } catch {
      return null
    }
  }

  return {
    "chat.message": async ({ sessionID }, { parts }) => {
      const cfg = pairConfig()
      const pair = cfg?.opencode?.session
      // unbound yet: this is the first session, the one pair.py launch binds
      if (!pair || sessionID === pair) return
      const info = await call("GET", `/session/${sessionID}`)
      // a subagent session has a parent and belongs to the pair session
      if (!info || info.parentID) return

      const text = parts
        .filter((p) => p.type === "text" && !p.synthetic)
        .map((p) => p.text)
        .join("\n")
        .trim()
      await call("POST", "/tui/select-session", { sessionID: pair })
      if (text) {
        const agent = cfg.roles?.reviewer?.agent
        await call("POST", `/session/${pair}/prompt_async`, {
          parts: [{ type: "text", text: `Message from the user: ${text}` }],
          ...(agent ? { agent } : {}),
        })
      }
      // after this hook returns the stray session starts its own turn: stop it
      setTimeout(async () => {
        await call("POST", `/session/${sessionID}/abort`)
        await call("DELETE", `/session/${sessionID}`)
      }, 1000)
    },
  }
}
