import { createEffect, createSignal } from "solid-js"
import { readFile } from "node:fs/promises"
import { homedir } from "node:os"
import { join } from "node:path"
import type { TuiPlugin } from "@opencode-ai/plugin/tui"

// Codex limit reset info from codex-resets.com and the weekly Codex usage of the
// OpenCode ChatGPT login, shown next to the prompt. The session prompt also shows
// the context use on the model line and drops the row below it (directory,
// context, ctrl+p hint), which stays only while the session runs.
const API = "https://codex-resets.com/api/resets"
const USAGE_API = "https://chatgpt.com/backend-api/wham/usage"
const AUTH_FILE = join(process.env.XDG_DATA_HOME ?? join(homedir(), ".local/share"), "opencode/auth.json")
const WEEK_S = 7 * 24 * 3600
const FETCH_MS = 10 * 60 * 1000
const USAGE_MS = 5 * 60 * 1000
const TICK_MS = 60 * 1000
const FRESH_MS = 6 * 3600 * 1000

type Window = { used_percent: number; limit_window_seconds: number; reset_at: number }
type Usage = { rate_limit?: { primary_window?: Window | null; secondary_window?: Window | null } | null }

// weekly window of the plan limits; the API puts it in primary or secondary
async function weekly(): Promise<Window | undefined> {
  const auth = JSON.parse(await readFile(AUTH_FILE, "utf8")).openai
  if (auth?.type !== "oauth" || !auth.access) return undefined
  const res = await fetch(USAGE_API, {
    headers: { Authorization: `Bearer ${auth.access}`, "ChatGPT-Account-Id": auth.accountId ?? "" },
    signal: AbortSignal.timeout(10_000),
  })
  if (!res.ok) return undefined
  const limits = ((await res.json()) as Usage).rate_limit
  return [limits?.primary_window, limits?.secondary_window].find((w) => w?.limit_window_seconds === WEEK_S) ?? undefined
}

function usageLine(w: Window | undefined): string {
  if (!w) return ""
  const when = new Date(w.reset_at * 1000).toLocaleString(undefined, { weekday: "short", hour: "2-digit", minute: "2-digit", hour12: false })
  return `wk ${Math.round(100 - w.used_percent)}% ↻${when}`
}

type Resets = {
  scheduled?: { tweet_id: string; scheduled_for: string } | null
  events?: { tweet_id: string; announced_at: string }[]
}

function human(ms: number): string {
  const s = Math.abs(ms) / 1000
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

function line(data: Resets | undefined, now: number): { text: string; tone: "warn" | "ok" | "muted" } {
  if (!data) return { text: "", tone: "muted" }
  const at = data.scheduled ? Date.parse(data.scheduled.scheduled_for) : NaN
  if (at > now) {
    return { text: `⟳ reset in ${human(at - now)}`, tone: "warn" }
  }
  const last = data.events?.[0] ? Date.parse(data.events[0].announced_at) : NaN
  if (Number.isNaN(last)) return { text: "", tone: "muted" }
  if (now - last < FRESH_MS) return { text: `✓ reset ${human(now - last)} ago`, tone: "ok" }
  return { text: `reset ${human(now - last)} ago`, tone: "muted" }
}

// same figures as OpenCode's own prompt: tokens of the last answer against the model limit
function contextLine(api: Parameters<TuiPlugin>[0], sessionID: string): string {
  const last = api.state.session
    .messages(sessionID)
    .findLast((m: any) => m.role === "assistant" && m.tokens.output > 0) as any
  if (!last) return ""
  const t = last.tokens
  const tokens = t.input + t.output + t.reasoning + t.cache.read + t.cache.write
  if (tokens <= 0) return ""
  const size = tokens >= 1e6 ? `${(tokens / 1e6).toFixed(1)}M` : tokens >= 1e3 ? `${(tokens / 1e3).toFixed(1)}K` : `${tokens}`
  const limit = api.state.provider.find((p) => p.id === last.providerID)?.models[last.modelID]?.limit.context
  return limit ? `${size} (${Math.round((tokens / limit) * 100)}%)` : size
}

export const tui: TuiPlugin = async (api) => {
  const [data, setData] = createSignal<Resets>()
  const [now, setNow] = createSignal(Date.now())
  let seen: string | undefined

  const refresh = async () => {
    try {
      const res = await fetch(API, { signal: AbortSignal.timeout(10_000) })
      if (!res.ok) return
      const next = (await res.json()) as Resets
      const key = next.scheduled?.tweet_id ?? next.events?.[0]?.tweet_id
      if (seen !== undefined && key !== seen) {
        api.ui.toast({ variant: "info", title: "Codex resets", message: line(next, Date.now()).text, duration: 10_000 })
      }
      seen = key
      setData(next)
    } catch {
      // network errors keep the last known data
    }
  }

  const [usage, setUsage] = createSignal<Window>()
  const refreshUsage = async () => {
    try {
      setUsage(await weekly())
    } catch {
      // a missing login or network error keeps the last known usage
    }
  }

  await Promise.all([refresh(), refreshUsage()])
  const fetchTimer = setInterval(refresh, FETCH_MS)
  const usageTimer = setInterval(refreshUsage, USAGE_MS)
  const tickTimer = setInterval(() => setNow(Date.now()), TICK_MS)
  api.lifecycle.onDispose(() => {
    clearInterval(fetchTimer)
    clearInterval(usageTimer)
    clearInterval(tickTimer)
  })

  const view = (ctx: { theme: { current: Record<string, unknown> } }) => {
    // read the signals inside JSX expressions so the compiled effects track them
    const current = () => line(data(), now())
    const color = () => {
      const t = ctx.theme.current
      const tone = current().tone
      return tone === "warn" ? t.warning : tone === "ok" ? t.success : t.textMuted
    }
    return (
      <box flexDirection="row" gap={2}>
        <text fg={ctx.theme.current.textMuted as never}>{usageLine(usage())}</text>
        <text fg={color() as never}>{current().text}</text>
      </box>
    )
  }

  // OpenCode's Prompt with the context beside the model. Its last row has no option
  // to hide it, so it is hidden on the rendered tree: Prompt's root box holds the
  // input, a border line and that row.
  const prompt = (
    ctx: { theme: { current: Record<string, unknown> } },
    props: { session_id: string; visible?: boolean; disabled?: boolean; on_submit?: () => void; ref?: any },
  ) => {
    let wrap: any
    const tidy = () => {
      const row = wrap?.getChildren()[0]?.getChildren()[2]
      if (!row) return
      row.visible = (api.state.session.status(props.session_id)?.type ?? "idle") !== "idle"
      // the right side: editor file, context, ctrl+p hint
      for (const item of row.getChildren().slice(1)) item.visible = false
    }
    createEffect(() => {
      api.state.session.status(props.session_id)
      setTimeout(tidy, 0)
    })
    return (
      <box
        ref={(r: unknown) => {
          wrap = r
          setTimeout(tidy, 0)
        }}
      >
        <api.ui.Prompt
          sessionID={props.session_id}
          visible={props.visible}
          disabled={props.disabled}
          onSubmit={props.on_submit}
          ref={props.ref}
          right={
            <box flexDirection="row" gap={2}>
              <text fg={ctx.theme.current.textMuted as never}>{contextLine(api, props.session_id)}</text>
              <api.ui.Slot name="session_prompt_right" session_id={props.session_id} />
            </box>
          }
        />
      </box>
    )
  }

  api.slots.register({
    slots: {
      home_prompt_right: view,
      session_prompt_right: view,
      session_prompt: prompt,
    },
  })
}

export default { id: "codex-resets", tui }
