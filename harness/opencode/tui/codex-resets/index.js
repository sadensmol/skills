import { effect as _$effect } from "@opentui/solid";
import { insertNode as _$insertNode } from "@opentui/solid";
import { insert as _$insert } from "@opentui/solid";
import { setProp as _$setProp } from "@opentui/solid";
import { createElement as _$createElement } from "@opentui/solid";
import { createSignal } from "solid-js";
import { readFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
// Codex limit reset info from codex-resets.com and the weekly Codex usage of the
// OpenCode ChatGPT login, shown next to the prompt.
const API = "https://codex-resets.com/api/resets";
const USAGE_API = "https://chatgpt.com/backend-api/wham/usage";
const AUTH_FILE = join(process.env.XDG_DATA_HOME ?? join(homedir(), ".local/share"), "opencode/auth.json");
const WEEK_S = 7 * 24 * 3600;
const FETCH_MS = 10 * 60 * 1000;
const USAGE_MS = 5 * 60 * 1000;
const TICK_MS = 60 * 1000;
const FRESH_MS = 6 * 3600 * 1000;
// weekly window of the plan limits; the API puts it in primary or secondary
async function weekly() {
  const auth = JSON.parse(await readFile(AUTH_FILE, "utf8")).openai;
  if (auth?.type !== "oauth" || !auth.access) return undefined;
  const res = await fetch(USAGE_API, {
    headers: {
      Authorization: `Bearer ${auth.access}`,
      "ChatGPT-Account-Id": auth.accountId ?? ""
    },
    signal: AbortSignal.timeout(10_000)
  });
  if (!res.ok) return undefined;
  const limits = (await res.json()).rate_limit;
  return [limits?.primary_window, limits?.secondary_window].find(w => w?.limit_window_seconds === WEEK_S) ?? undefined;
}
function usageLine(w) {
  if (!w) return "";
  const when = new Date(w.reset_at * 1000).toLocaleString(undefined, {
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  });
  return `wk ${Math.round(100 - w.used_percent)}% ↻${when}`;
}
function human(ms) {
  const s = Math.abs(ms) / 1000;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}
function line(data, now) {
  if (!data) return {
    text: "",
    tone: "muted"
  };
  const at = data.scheduled ? Date.parse(data.scheduled.scheduled_for) : NaN;
  if (at > now) {
    return {
      text: `⟳ reset in ${human(at - now)}`,
      tone: "warn"
    };
  }
  const last = data.events?.[0] ? Date.parse(data.events[0].announced_at) : NaN;
  if (Number.isNaN(last)) return {
    text: "",
    tone: "muted"
  };
  if (now - last < FRESH_MS) return {
    text: `✓ reset ${human(now - last)} ago`,
    tone: "ok"
  };
  return {
    text: `reset ${human(now - last)} ago`,
    tone: "muted"
  };
}
export const tui = async api => {
  const [data, setData] = createSignal();
  const [now, setNow] = createSignal(Date.now());
  let seen;
  const refresh = async () => {
    try {
      const res = await fetch(API, {
        signal: AbortSignal.timeout(10_000)
      });
      if (!res.ok) return;
      const next = await res.json();
      const key = next.scheduled?.tweet_id ?? next.events?.[0]?.tweet_id;
      if (seen !== undefined && key !== seen) {
        api.ui.toast({
          variant: "info",
          title: "Codex resets",
          message: line(next, Date.now()).text,
          duration: 10_000
        });
      }
      seen = key;
      setData(next);
    } catch {
      // network errors keep the last known data
    }
  };
  const [usage, setUsage] = createSignal();
  const refreshUsage = async () => {
    try {
      setUsage(await weekly());
    } catch {
      // a missing login or network error keeps the last known usage
    }
  };
  await Promise.all([refresh(), refreshUsage()]);
  const fetchTimer = setInterval(refresh, FETCH_MS);
  const usageTimer = setInterval(refreshUsage, USAGE_MS);
  const tickTimer = setInterval(() => setNow(Date.now()), TICK_MS);
  api.lifecycle.onDispose(() => {
    clearInterval(fetchTimer);
    clearInterval(usageTimer);
    clearInterval(tickTimer);
  });
  const view = ctx => {
    // read the signals inside JSX expressions so the compiled effects track them
    const current = () => line(data(), now());
    const color = () => {
      const t = ctx.theme.current;
      const tone = current().tone;
      return tone === "warn" ? t.warning : tone === "ok" ? t.success : t.textMuted;
    };
    return (() => {
      var _el$ = _$createElement("box"),
        _el$2 = _$createElement("text"),
        _el$3 = _$createElement("text");
      _$insertNode(_el$, _el$2);
      _$insertNode(_el$, _el$3);
      _$setProp(_el$, "flexDirection", "row");
      _$setProp(_el$, "gap", 2);
      _$insert(_el$2, () => usageLine(usage()));
      _$insert(_el$3, () => current().text);
      _$effect(_p$ => {
        var _v$ = ctx.theme.current.textMuted,
          _v$2 = color();
        _v$ !== _p$.e && (_p$.e = _$setProp(_el$2, "fg", _v$, _p$.e));
        _v$2 !== _p$.t && (_p$.t = _$setProp(_el$3, "fg", _v$2, _p$.t));
        return _p$;
      }, {
        e: undefined,
        t: undefined
      });
      return _el$;
    })();
  };
  api.slots.register({
    slots: {
      home_prompt_right: view,
      session_prompt_right: view
    }
  });
};
export default {
  id: "codex-resets",
  tui
};