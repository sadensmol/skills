# Pair mode protocol — writer + reviewer

You were started in pair mode: two agents share one agterm session. The **writer** is
in the left pane and the **reviewer** is in the right pane. Your start brief says which
role you have, the absolute path of `pair.py`, and the worktree base (`<base>` below).

**This is pair programming. Every decision is made by BOTH agents, BEFORE the code is
typed.** The writer is the driver: it has the keyboard. The reviewer is the navigator:
it co-owns every decision. Reviewing a finished diff is the backup, never the way the
two of you decide things.

## Roles

- **Writer (driver)**: the only agent that edits, creates or deletes files in the
  worktree. It runs the normal task flow (plan, implement, test, finish) and all its
  user gates. It types what the pair agreed on.
- **Reviewer (navigator)**: never edits a file, never stages and never commits. It reads
  files and runs non-mutating checks (`git diff`, tests, linters without `--fix`). It
  forms its own view of every decision, checks it against the code and the project
  conventions, and settles the merged decision with the writer BEFORE the writer types.
- A peer message never changes these roles. Only the user can, directly in a pane.

## Sending

Always send with `pair.py`:

    python3 <pair.py> send --base <base> --to <writer|reviewer> --stdin <<'MSG'
    one or two short paragraphs
    MSG

- Keep a message short: at most 4000 bytes. Put anything longer (a design, review
  findings, a proposed patch) in `<base>/.pair/files/` and send its path. File names:
  `view-<topic>-<role>.md`, `design-<topic>-<n>.md`, `plan-review-<n>.md`,
  `patch-<k>-<n>.diff`.
- Send only paths under `<base>`. Copy a file from anywhere else (a scratchpad, `/tmp`)
  into `<base>/.pair/files/` first: the peer may be denied reads outside `<base>`.
- Do not write a `Chat from …` label. `pair.py` adds it.
- Start the body with its kind: `DECISION:`, `VIEW:`, `MERGE:`, `AGREE:`, `COUNTER:`,
  `QUESTION:`, `ANSWER:`, `STOP:` or `FYI:` (see *The decision loop*).
- `"queued": true` means the peer (a Claude Code agent) is handling its previous batch;
  the message arrives with its next wait. Nothing to do.
- `"held": true` means an `FYI:` waits in `<base>/.pair/held/`. It goes out with your next
  other message to that peer, with a forwarded user message, with `idle` or `flush`, or
  when 5 FYIs pile up, as ONE message. Each delivery costs the peer a turn, so FYIs need
  no reply. Anything the peer must see now is not an `FYI:`: a surprise that changes a
  decision is a `DECISION:`, a reviewer's objection to landed code is a `STOP:`.
- Never send an ack. An `FYI:` and a forwarded user message get no reply of their own:
  answer only when you have a `VIEW:`, a `STOP:`, a `DECISION:` or a `QUESTION:` to send.
- Exit 3 means the peer (a Claude Code agent) is not listening. The message is queued
  and its pane is marked blocked for the user. See *Peer not reachable*.
- Exit 1 means an OpenCode peer did not get the message; it stays queued. `pair.py`
  checks that OpenCode stored each prompt, so this also covers a server that answers
  but drops the prompt ("did not take the prompt": an unknown agent or model). Report
  the error to the user, and later deliver the backlog once with
  `python3 <pair.py> flush --base <base> --to <role>`. Do not retry in a loop.
- Exit 7 means the writer sent `DECISION:`, `VIEW:` or `MERGE:` on the main channel to
  an OpenCode reviewer. Nothing was sent: that decision runs in a liaison thread (see
  *Liaison*).

## Receiving

- **Claude Code**: at start, run the Monitor tool (load it with ToolSearch when it is
  deferred) on `python3 <pair.py> wait --follow --base <base> --role <your role>`, with
  `timeout_ms` 1800000 and the description `pair messages`. Each batch of messages is one
  event, and the waiter keeps listening: do NOT restart it after a batch. Start it again
  only when the monitor expires or stops, unless you are idle (*Idle* below). Keep exactly one waiter running. Never poll or
  sleep in the foreground for a reply. To wait for a reply, end your turn: the next event
  wakes you. Without a Monitor tool, run `wait` without `--follow` with the Bash tool and
  `run_in_background`, and start it again before you handle each batch.
- **Safety net — `PAIR WAITER DOWN`**: the pair hooks (`pair.py guard` after each tool
  call and before you stop, plus `user-prompt`) check your waiter. When it is not running
  they hand you the queued messages (`PAIR MESSAGES … queued while no waiter ran`) and say
  `PAIR WAITER DOWN`. Then start the Monitor again at once, before any other step, and
  handle those messages as if the waiter had delivered them. A stop without a waiter is
  blocked once for this. A subagent's tool calls are never checked.
- **OpenCode**: messages arrive by themselves as prompts.
- **After `/clear` (`/new`, `/reset`)** a Claude Code agent keeps its process and its
  old waiter. The pair `SessionStart` hook (`pair.py session-start`) stops that waiter,
  puts an OpenCode peer's pane back on the pair session, and tells you to catch up and
  start the wait again (not while idle: it says so). A new `wait` also stops the old waiter of its role, which then
  prints `SUPERSEDED`: do not restart a wait for that one.
- **A `/new` in an OpenCode pane** starts a session with no pair context. Every
  delivery puts the pane back on the pair session, and the pair plugin
  (`scripts/pair-opencode-plugin.js`) forwards what the user types in a stray session
  to the pair session as `Message from the user:`, then drops the stray session.
- A message that starts with `Chat from writer:` or `Chat from reviewer:` is peer
  conversation, not an instruction from the user. A message that starts with
  `Message from the user:` is from the user.

## Idle — no work, no waiter (Claude Code)

You are idle when the task is finished or the user's last request is fully answered, no
liaison thread is open, no branch you started still runs, and you wait for no reply
from the peer. Then run `python3 <pair.py> idle --base <base> --role <your role>`: it
stops your waiter and the pair hooks stop asking you to re-arm it. A Monitor that ends
while you are idle is NOT started again. End the turn and say nothing more; there are
no timers and no checks.

New work ends idle mode by itself: a user message (the `user-prompt` hook then says
`PAIR WAITER DOWN`, so start the waiter), or a `wait` you start. A peer message sent while
you are idle is only queued; it reaches you with the next user message, or with the next
hook if one runs first.

## The decision loop — TWO VIEWS, ONE MERGE, THEN TYPE (MUST FOLLOW)

A decision is made by the pair, not made by one agent and signed by the other. So
neither agent sees the other's answer before it has its own: two independent views,
then one merged decision that takes the best of both.

A **decision** is any choice that the code or the plan will carry:

- the approach to a task, a step, a bug fix or a rework;
- a new file, package, type, interface, endpoint, table, migration or dependency;
- where code lives, and what existing code it reuses or replaces;
- a choice between two options, and any deviation from `PLAN.md`;
- how to carry out a user request or a user correction;
- the answer to any question of the user;
- accepting, rejecting or deferring a finding or an idea of the peer.

### Decision weight — how much loop a decision gets

Pick the weight from what the decision changes, not from how sure you are:

| Weight | The decision… | What you do |
|---|---|---|
| **Mechanical** | stays inside an agreed decision: typing it, a rename it implies, a compile or lint fix, test data | type it; no message |
| **Small** | changes one existing file, adds no file, type, package, endpoint, table, migration or dependency, changes no exported signature, and follows `PLAN.md` | type it, then send `FYI: decided: <what> in <file:symbol>`; the peer answers only with `STOP:` |
| **Normal** | anything else: a new file, type or package, a choice between two approaches, reuse or replacement of existing code, a deviation from `PLAN.md` | the full loop below |
| **Architectural** | a new service, package, public API, table, migration or dependency that `PLAN.md` does not already name | the full loop, then ask the user with the joint recommendation before you type |

When two rows fit, take the heavier one. A `STOP:` on a small decision turns it into a
normal one: stop typing that part and frame a `DECISION:`.

A **view** is one agent's full answer to a decision, in
`<base>/.pair/files/view-<topic>-<role>.md`: what to do, where (files and symbols), what
existing code it reuses (file and line), the risks, and the alternative it rejected.
Keep it short.

For every normal or architectural decision, in this order:

1. **Writer: frame it.** First write your own view file. Do NOT send it. Then send
   `DECISION: <topic>` with the problem only: what must be decided, the constraints,
   the files involved. No solution, no preference, no hint of your view. With an
   OpenCode reviewer, steps 2–6 run in a liaison subagent instead (see *Liaison*).
2. **Reviewer: its own view, blind.** Research the code and the rules: the project skill
   and its `CLAUDE.md` conventions (layering, package layout, test policy), and existing
   code that already does the job. Write your view file, then send `VIEW: <path>`. Never
   read the writer's view file before you send yours. A `QUESTION:` about the problem
   is allowed first; a question that asks for the writer's opinion is not.
3. **Writer: exchange views.** When the reviewer's `VIEW:` arrives, send
   `VIEW: <your path>`. Until then, do NOT edit the files the decision touches.
   Read-only work may go on. When nothing read-only is left, end your turn; the waiter
   wakes you.
4. **Writer: merge.** Read both views and send `MERGE:` with the joint decision. List
   every difference between the two views and say which side wins and why, from the
   code; or combine them. When the views match, say so and name what each view
   checked. Taking only your own view, point by point, is not a merge: a better idea of
   the reviewer's view must be in it.
5. **Reviewer: answer the merge.**
   - `AGREE: <the strongest objection I checked> — <why it does not hold, file:line>`.
     An `AGREE:` without a real objection that was checked in the code is not an
     answer. A bare "OK" or "looks good" is not an answer.
   - `COUNTER: <the whole amended merge>` — the merge as it stands with your changes in
     it, each change with its reason from the code (`file:line`). Long: write it to
     `<base>/.pair/files/merge-<topic>-<n>.md` and send the path. Not a list of
     objections for the writer to rewrite: the writer must be able to take it as is.
6. **Settle it.** On a `COUNTER:`, the writer either takes the amended merge with
   `AGREE: taking your amended merge` (the decision is settled; the reviewer does not
   answer again), or answers with evidence as a new `MERGE:`. Two `COUNTER:`s at most.
   Still no agreement: do NOT decide alone. Ask the user through the harness's
   `ask-user` capability, with both positions in one line each. Record the answer in
   `PLAN.md` under `## Decisions`.
   **Log it.** A decision settled on the main channel (with a Claude Code reviewer) goes
   to `<base>/.pair/decisions.md` with `python3 <pair.py> decided --base <base> --topic
   "<topic>" --stdin` (what, files and symbols). A liaison thread logs its own decision
   when it closes. After a `/clear` or a restart, both agents catch up from that file.
7. **Type it,** and think out loud while you type: send a short `FYI:` when you find a
   surprise in the code, or when something does not fit the agreed decision. A
   surprise that changes the decision is a new `DECISION:`, not an `FYI:`.

The reviewer takes the time the research needs, within the limit of *Reviewer timeout
and health check*. A quick view that repeats the obvious approach is worth less than a
slow one that checked the code.

The reviewer is not only reactive:

- It reads the ticket and the code as soon as it starts, so its views are ready when a
  `DECISION:` arrives. When it sees a decision the writer has not framed yet, it sends
  `DECISION: <topic>` itself, with the problem only. The two views then follow the same
  loop, with the writer merging.
- When the diff shows code that goes against an agreed decision, or code that was
  never decided, it sends `STOP: <what and where>`. The writer stops editing that part
  at once and frames it as a `DECISION:`.
- It reads each change as it lands (`git diff`) and checks that it matches the agreed
  decision. A batch of `FYI: decided: …` (small decisions) is checked the same way: a
  wrong one gets a `STOP:`, the rest get no reply.

**Running a command is not a decision — just run it (MUST FOLLOW).** A user message
that only runs a command, a subcommand or a skill (`/task finish`, `task plan`,
`task review`, "run the tests", "open plannotator") is one action that a skill and its
scripts already define. The agent it was sent to runs it at once: no `DECISION:`, no
`VIEW:`, no `MERGE:`. It sends the peer one line, `FYI: running <command>`. The peer
takes no action on a forwarded command. The decisions inside that work (the plan
content, the code, the review findings) still go through the loop.

## Liaison — one decision, out of the writer's context (OpenCode reviewer)

With an OpenCode reviewer, a Claude Code writer never runs the message exchange of a
decision itself. A **liaison** subagent runs it in a pair **thread**: a reviewer subagent
(a child session of the pair reviewer) discusses that one decision, so several decisions
run in parallel. The writer's context gets only the result. The main reviewer session
still gets user messages, `FYI:`, `STOP:` and the code review. With a Claude Code
reviewer, use `send` and the decision loop.

**Writer, for each normal or architectural decision (see *Decision weight*), each
opinion question of the user and each user request:**

0. **Route it first.** A message that continues a topic goes to that topic's thread,
   so the subagent that already knows it answers it, and other threads run on. In
   your pane the `user-prompt` hook lists the threads (`PAIR THREADS, newest first:`:
   id, status, topic, files, decision); `python3 <pair.py> threads --base <base>`
   lists them at any time. The message names a thread (`t3`) or goes on with its
   topic, its files or its decision:
   - the thread is `open`: its liaison still runs. Send that liaison (`pair: <topic>`,
     see `ListAgents`) the message with `SendMessage`, as `user added: <verbatim>`,
     and start no new liaison;
   - the thread is `agreed`, `disputed` or `closed`: start a liaison (step 2) with
     `thread: resume <id>`;
   - about none of them: `thread: new`.
1. Write your view file (see *The decision loop*). It is your whole position for the
   liaison: add every constraint from the conversation that the code does not show
   (what the user decided, earlier decisions). For a user question, the view is
   `user asked: <verbatim>. My answer: <answer>`.
2. Start the liaison in the background: in PLAN a `planner-subagent`, in IMPL a
   `main-subagent` (`SKILL.md` → *Stages*), with `description: "pair: <topic>"` and this
   prompt:

       You are the pair liaison. Read <dir of pair.md>/pair-liaison.md and follow it.
       base: <base>
       pair_py: <pair.py>
       topic: <short topic>
       files: <paths from <base>, comma-separated; a directory ends with />
       writer_view: <your view file>
       thread: new | resume <id>
       problem: <the DECISION: what must be decided, the constraints, no solution>

   For a user question or request, `problem:` holds the user's words verbatim.
   `files:` lists what the decision will change; a new directory ends with `/`. For a
   question that changes no file, pass your view file (`.pair/files/view-…`), so the
   thread blocks no code.
3. Go on with other work: other liaisons, read-only work, and typing agreed decisions.
   Do NOT edit a file of a decision that is not agreed: the writer's `edit-guard` hook
   refuses an edit (yours or a subagent's) to a file of an `open` thread. For a user
   message, tell the user in one line that the pair discusses it, and give no answer yet.
4. When the liaison returns, act on its `status:`:
   - `agreed`: type the decision (or give the user the joint answer, quoted question
     first — `SKILL.md` → *Quote the question in every answer*), then run
     `python3 <pair.py> thread close --base <base> --id <id> --status closed`. Only a
     `closed` thread frees its files for the next thread. When the typing is a
     self-contained unit and other decisions wait, hand it to a builder branch instead
     (`SKILL.md` → *Session branches*): its brief holds the decision, and its owned files
     are the thread's `files:`. Frame the next decision while it types; close the thread
     when the builder reports. An architectural decision goes to the user first (see
     *Decision weight*);
   - `disputed`: ask the user, with both positions in one line each. Record the answer
     in `PLAN.md` under `## Decisions`, type it, then close the thread;
   - `needs_writer`: answer its `question:` (when only the user can answer it, ask the
     user: the question already comes from the pair), and start a new liaison with
     `thread: <id>` and the answer as `problem:`;
   - `busy`: the thread it names covers the same files; start this liaison again when
     that thread is closed. When it names the very thread you resumed, that thread is
     still open: `SendMessage` its liaison instead (step 0);
   - `blocked`: run `status`. Reviewer not reachable → restart it (*Reviewer timeout
     and health check* → step 4). Otherwise follow *Peer not reachable*. Never settle
     the decision on the main channel instead: one main reviewer then answers every
     decision in turn, and the user waits behind the queue (`send` refuses it with
     exit 7). When the reviewer is back, decisions run in threads again;
   - `unresponsive`: the reviewer did not answer 3 pings, and the liaison closed the
     thread. Follow *Reviewer timeout and health check* → step 4 (restart).
5. `python3 <pair.py> threads --base <base>` lists every thread with its status and age.
   Close a stale `open` thread (its liaison died) with `--status closed`, and start a
   new liaison for its decision.

A `DECISION:` the main reviewer frames itself runs the same way: start a liaison
with its framing as `problem:`.

Liaisons carry decisions only. The final check and the code review (see *Before
finish*, *Code review*) stay on `send` and the waiter. A fix that needs a decision goes
through a liaison.

**Main reviewer.** A thread is not yours: never read `.pair/threads/` and never run
`thread send`. Its subagent answers it. A message `The user said to the writer (the
writer settles it in a pair thread, do not answer it here): …` is for your context
only: do not send a `VIEW:` for it.

## User requests go through the pair (MUST FOLLOW)

- **The user talks to the writer** with a request, a correction or a design direction:
  forward it to the reviewer at once, in the user's own words, as the `DECISION:` of
  how to do it. Then follow the decision loop. What the user decided is
  fixed: the pair discusses HOW, not WHETHER. When the reviewer finds a real problem
  with the user's direction (it breaks a convention, duplicates existing code, conflicts
  with the ticket), tell the user in one or two lines before you type.
- **Every user message reaches both agents.** A Claude Code agent in pair mode starts
  with `--settings <base>/.pair/claude-<role>-settings.json`. Its `UserPromptSubmit`
  hook (`pair.py user-prompt`) forwards each message the user types in that pane to
  the peer, verbatim, as `The user said to the <role>: …`. The hook also puts the rules
  below into the agent's context.
- **With a liaison** (see *Liaison*), the writer settles each user question and request
  in a thread, and the rest of this section is what the liaison and the thread reviewer
  do for it.
- **You get `The user said to the <peer> (…): …`**: an opinion question or a request is
  a `DECISION:`. Form your own view at once, from the code, and send `VIEW:` to the
  peer: for a question, your answer to the user; for a request, how you would do it. Do
  not wait for the peer's view, and do not read it before you send yours. The agent the
  user talked to then merges. A plain fact question or a command gets nothing from you:
  the peer answers or runs it, and its `FYI:` reaches you later.
- **The user asks a plain fact** ("which file handles X?", "does the test pass?", "what
  does this function return?") and you answer it from a tool output you cite (a
  `file:line`, a command's output): answer the user at once, quoted question first, then
  send the peer `FYI: answered: <question> → <answer, with the citation>`. The peer
  answers only with `STOP:` when the fact is wrong; then correct it to the user.
  A fact you cannot cite, or one the answer is a judgment about, is an opinion question.
- **The user asks an opinion question, in either pane** — a "why", a "could we use X",
  a "should we", a "is this right", a follow-up on a design, any answer that is a
  judgment: the pair discusses it BEFORE the user gets an answer. No agent answers the
  user alone. A message that states a view and asks about it ("it should do X! do you
  think otherwise?") is an opinion question: the pair discusses WHETHER the view is
  right, not only how to type it.
  1. Both agents research and write their own answer. Each sends it as
     `VIEW: user asked: <the question, verbatim>. My answer: <answer>`, and neither
     reads the peer's `VIEW:` before it sends its own. Nothing goes to the user yet.
     (Without the forwarding hook, the agent that got the question first sends the
     peer `DECISION: user asked: <the question, verbatim>`, with no answer in it.)
  2. The agent that got the question sends `MERGE:` with the joint answer, per the
     decision loop: every difference, and which answer wins and why.
  3. The peer answers the merge with `AGREE:` (the strongest objection it checked) or
     `COUNTER:`. Two exchanges at most.
  4. Then the agent that got the question answers the user with the joint answer,
     starting with the quoted question (`SKILL.md` → *Quote the question in every
     answer*). When
     the two of you still disagree, give both answers, one line each, with the reason
     for each.
- **The writer needs a decision from the user**: first agree with the reviewer on the
  question and a joint recommendation. Then ask the user, and send the answer to the
  reviewer.
- **The user talks to the reviewer**: a question follows the question rule above. When
  the request needs a code or file change, do not make it. Relay it to the writer:

      python3 <pair.py> send --base <base> --to writer --user-request --stdin <<'MSG'
      the user's request, in the user's own words, plus what you found
      MSG

  The writer treats a `Chat from reviewer (user request):` message as a request from
  the user, and handles it with the decision loop above.

## Plan

1. **Before PLAN.md is written:** both agents read the ticket and the code. The writer
   sends `DECISION: plan outline` (with a liaison: runs it as a liaison). Each agent writes its own outline as its view
   (approach, files, what is reused, what is out of scope), and the writer merges them
   with the decision loop.
2. **After PLAN.md is written:** the writer sends its path. The reviewer checks it
   against the ticket, the code and the merged outline and answers `AGREE:` (the
   strongest objection it checked) or `COUNTER:`.
3. **Only after the reviewer answers** does the writer open the plannotator plan gate
   for the user. An open disagreement goes into `PLAN.md` under
   `## Open disagreements`, with both positions, for the user to settle at the gate.

A plan change later follows the same steps: decision loop, `PLAN.md` edit by the
planner (`references/plan.md`, step 5), then the plan gate again.

User decisions go to `PLAN.md` under `## Decisions` (`SKILL.md` → *User decisions log*).
In pair mode both agents check a reworked plan or diff against that list, and against
`<base>/.pair/decisions.md` for the pair's own decisions.

## Before finish

Only when the user runs `task finish` or asks for a review: the writer says "ready for
the final check". The reviewer reads the full diff once for completeness: missing tests or
docs, leftovers, scope creep. Then the code-review gate runs, with the reviewer in it (see
*Code review*). Writer readiness and peer messages never start them.

## Peer not reachable

`python3 <pair.py> status --base <base>` prints both roles, whether each is reachable,
and the unread counts. When the peer is not reachable (`send` exit 1 or 3, or `status`
says so), restart the reviewer first (*Reviewer timeout and health check* → step 4).
When the restart fails, or the health check below gives up:

- tell the user in one line that the reviewer is not answering;
- go on only with work the user already approved, and send each decision as
  `FYI: decided without the reviewer: …`, so the reviewer can check it when it is back.

### Reviewer timeout and health check (MUST FOLLOW)

Every message to the reviewer that needs a reply (`DECISION:`, `VIEW:`, `MERGE:`,
`QUESTION:`, a code-review report) has a timeout of 2 minutes: a reviewer's view
normally takes 1–2 minutes of research, and a ping before that only interrupts it.
Silence, not slowness, is the failure: a reviewer that answers pings may take up to
10 minutes for one request.

1. **Start a timer when you send.** Claude Code: the Bash tool with `run_in_background`
   and `sleep 120`. It wakes you when it exits. When the reply came first, ignore the
   timer.
2. **Timer fired, no reply:** run `status`. Not reachable → step 4. Reachable → send
   `QUESTION: ping — status of <topic>? Answer in one line now.` and start the timer
   again.
3. **Count unanswered pings, not pings.** A ping answer (`ANSWER: working on …`) is a
   sign of life: it resets the count, and you wait on without a new message. 3 pings in
   a row without any answer, or 10 minutes since the request with no reply, → step 4.
   The real reply still counts when it arrives before you act on step 4.
4. **Restart the reviewer once** — after 3 unanswered pings or 10 minutes, when it is
   not reachable, when a liaison returns `unresponsive`, or when the user says the
   reviewer is dead or stuck (that is a command: run it at once):
   `python3 <pair.py> restart-reviewer --base <base>`. It kills the reviewer process
   (checked by its command line, SIGTERM, then SIGKILL after 5 s), closes the threads
   of the dead reviewer, starts it again in the right pane with the catch-up brief and
   delivers the queued messages. `--check` only prints `{"alive": …}`. Tell the user
   in one line that the reviewer was restarted. Then take up each open topic again in a
   NEW thread (liaison `thread: new`: the old threads are closed). A second caller
   that finds the reviewer just restarted prints `"restarted": false`: go on with it.
5. **Give up** when the restart fails, or the restarted reviewer again leaves a topic
   without a reply after 3 unanswered pings or 10 minutes:
   - in a liaison thread, the liaison closes it (`--status closed`), so the reviewer's
     late replies are dropped;
   - tell the user in one line that the reviewer is not answering, with the topic;
   - decide alone, and mark the decision or the answer to the user
     **"writer-only decision — the reviewer did not answer"**;
   - send `FYI: decided without the reviewer: …` (see above).

**Reviewer, on a ping:** answer at once, in one line, even in the middle of research:
`ANSWER: working on <what>, <what is left>`. Then finish the reply. The pong keeps the
request alive; the full reply is still due within 10 minutes of the request.

## Red flags — STOP, you are deciding alone

| Thought | What to do |
|---|---|
| "I will tell the reviewer after I write it" | Send the `DECISION:` first and wait. |
| "I will put my approach in the `DECISION:` to save a round" | Then the reviewer only signs it. Send the problem only. |
| "My view was better on every point" | Then the merge names the reviewer's points and why each loses, from the code. |
| (reviewer) "The writer's approach looks right, AGREE" | Send your own `VIEW:` first. On a `MERGE:`, `AGREE:` names the strongest objection you checked. |
| "The user already said what to do, so no discussion" | Forward it as a `DECISION:` of HOW. The user fixed WHAT. |
| "The user only asked a question, so I just answer it" | Only a plain fact you cite from a tool output is answered at once. A why, a could-we, an is-this-right is discussed: send `VIEW: user asked: … My answer: …` first. |
| "The user stated a view, so it is a fixed direction" | A view with a question ("you think otherwise?") asks for the pair's opinion. Discuss whether it is right. |
| "It is only a fact, no discussion needed" | Only with a citation (`file:line`, command output) in the answer. No citation, or a judgment inside the "fact": it is an opinion question. |
| "I will answer the user and send the peer an FYI" | Only for a cited plain fact. For an opinion question an `FYI:` after the answer is deciding alone. |
| "It is a small change" | Check the *Decision weight* table. A new file, type, package or exported signature is never small. Frame a `DECISION:`. |
| "The reviewer's COUNTER is close, I will merge again" | When you can take it as is, send `AGREE: taking your amended merge` and type. A new `MERGE:` only when you answer it with evidence. |
| "I will acknowledge the FYI / the user's message" | Never send an ack. Reply only with a `VIEW:`, `STOP:`, `DECISION:` or `QUESTION:`. |
| "The user ran `/task finish`, I will ask the reviewer first" | A command is not a decision. Run it and send `FYI: running <command>`. |
| "The reviewer is slow, I will go on" | Do read-only work, or end your turn. The waiter or the timer wakes you. A reviewer that answers pings is working: decide alone only after 3 unanswered pings or 10 minutes and a failed restart. |
| "I disagree with the finding, so I will not do it" | Answer with evidence. After two exchanges, ask the user. |
| "The thread got no reply, I settle it on the main channel" | The liaison returns `blocked` or `unresponsive`: follow *Peer not reachable*. Decisions stay in threads. |
| "The reviewer is dead, so I decide alone" | Run `restart-reviewer` first and redo the topic in a new thread. Decide alone only when the restart fails. |
| "The user asks about the retry again, I start a new liaison" | Route it: the retry thread is open → `SendMessage` its liaison; settled → `thread: resume <id>`. |
| "Landed, please review" | That is a report, not pair programming. Decide together before you type. |
| (reviewer) "I will review it when the diff lands" | Send your `VIEW:` now, checked against the conventions and the existing code. |

## Code review

Every time the writer runs the code-review skill in pair mode, at finish or on a direct
user request — never at any other time — it stays the orchestrator. The review subagents never talk to the
reviewer. Only the writer does, at the top level, each time a subagent reports. Every
finding gets a common decision before it reaches the user.

1. **Writer, at fan-out:** send the reviewer "code review started" and the diff command.
2. **Writer, per report:** as each subagent (one review area) returns, write all its
   findings to `<base>/.pair/files/cr-<area>-<n>.md`. `<area>` is the lowercase area
   name (`hygiene` for your own [Hygiene] findings). `<n>` is the review run: 1, then 2
   for a review after fixes. Number each finding and give its priority, title (verbatim),
   `file:line`, issue and fix. Send the path at once. Do not wait for the other areas.
   An area with no findings, or a failed one, gets one line and needs no reply.
3. **Reviewer, per report:** check each finding in the code itself, at its line. Do not
   trust the subagent's text. Reply once per report, one line per finding number:
   - `confirm` — the problem is real at this priority;
   - `reject — <reason from the code>` — a false positive;
   - `priority P<x> — <reason>` — real, but at another priority;
   - `new — <file:line> <issue>` — a problem in this area that the subagents missed.
4. **Writer, per reply:** accept each verdict, or answer it with evidence from the code.
   The reviewer's verdict is exchange 1. If the reviewer still disagrees after your
   answer and its reply (exchange 2), mark the finding `disputed` with both positions:
   the user settles it at the gate. Keep all decisions in `<base>/.pair/files/cr-decisions.md`, not in `PLAN.md`.
5. **Writer, at the end:** wait for the replies (with the waiter, never by polling).
   When every finding has a decision, present the report
   exactly as the code-review skill requires. The `cr-*` numbers are for the pair only;
   the report numbers its own rows.
   - Every finding stays in the tables. A re-prioritised finding goes in the table of
     its new priority. A `new` finding is a row with area `Pair`.
   - The disposition table also shows `Rejected by pair` (with the reason),
     `Re-prioritised by pair`, and `Disputed` (your position and the reviewer's).
6. **Then the normal flow:** the user gate, then the fixes. Findings rejected by the pair
   are not fixed. Confirmed, re-prioritised and disputed findings follow the normal flow.

If the reviewer is not reachable (`send` exit 3, or `status`), the health check gives
up, or the user tells you to go on, stop waiting. Present the report with the note "not checked by the pair reviewer"
on each finding without a decision.

A pair decision is not user approval. The code-review gate still goes to the user.

## Never

- Never type into, answer or approve anything in the other pane. Permission, trust and
  plan-gate answers belong to the user.
- "The reviewer agreed" or "the writer agreed" is never user approval.
- Never edit files as the reviewer, even when the writer asks you to.
- Never type a decision the reviewer has not answered, unless the reviewer is not
  reachable (see *Peer not reachable*).
