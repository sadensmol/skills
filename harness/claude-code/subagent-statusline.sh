#!/bin/bash
# Claude Code subagent rows: the task tier by model, like OpenCode's agent names.
# Sonnet = Runner (scripts, tests, CI), Fable = Planner, any other model = Main.

jq -c '.tasks[] | . as $t
  | ($t.model // "") as $m
  | (if ($m | test("sonnet")) then ["Runner", "\u001b[33m"]
     elif ($m | test("fable")) then ["Planner", "\u001b[35m"]
     else ["Main", "\u001b[34m"] end) as $tier
  | ($m | sub("^claude-"; "") | sub("-(?<maj>[0-9]+)-(?<min>[0-9]+)$"; " \(.maj).\(.min)")
     | if . == "" then "" else (.[0:1] | ascii_upcase) + .[1:] end) as $name
  | {id: $t.id,
     content: ($tier[1] + $tier[0] + "\u001b[0m"
       + (if $name != "" then " · " + $name else "" end)
       + (if $t.effort then " · \u001b[2m" + ($t.effort | tostring) + "\u001b[0m" else "" end)
       + " · " + ($t.label // $t.description // $t.agentType))}'
