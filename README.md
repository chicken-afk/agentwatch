# agentwatch — live dashboard for AI coding agents

A terminal dashboard that shows **which models an AI coding agent session uses, and in what order**:
the lead (main session), the advisor, and every subagent, live in a Ghostty split next to your agent.
It works with **Claude Code, Codex CLI, Gemini CLI, OpenCode and oh-my-pi (omp)** out of the box, and with any other tool through a small hook adapter.

```
╭─ ADVISOR · opus 5.5 ──────╮ ╭─ LEAD · sonnet 5.5 · main session ───────────────────────────╮
│ calls   3                 │ │ status  ◓ working  ⟶ Bash: go test ./internal/leave/...     │
│ ◇ 00:27:15 lead · Bash    │ ╰──────────────────────────────────────────────────────────────╯
│ ◇ 00:27:10 lead · Bash    │ ╭─ MODEL SEQUENCE · 20 steps ──────────────────────────────────╮
│ ◇ 00:26:53 lead · Read    │ │ … sonnet 5.5 → opus 5.5 (advisor) → sonnet 5.5 → haiku 5.5   │
│                           │ │ 00:27:10  ● opus 5.5    advisor   1 calls                    │
│                           │ │ 00:27:11  ● haiku 5.5   search    1 msgs                     │
╰───────────────────────────╯ ╰──────────────────────────────────────────────────────────────╯
```

## What it shows

| Tab | Contents |
|---|---|
| **1 dashboard** | LEAD box (agent, model, status, running tool, prompt, tokens), ADVISOR for Claude Code (model, call count, history + the context that triggered each call) or AGENT for other agents (agent, model, folder, source file), SKILLS (skills used in the current prompt: who invoked them, still active `●` or done `✓`, plus a summary of earlier prompts), MODEL SEQUENCE (chain + latest steps), SUBAGENT cards (model, running/done, duration, tools, tokens, task), MODEL USAGE table (role, messages, tokens in/out/cache, output share, active time), SESSION LOG |
| **2 model sequence** | Every model step since the session started, in order, including gaps |
| **3 log** | All events: prompts, tool calls, failures, subagent dispatches, advisor, compact |
| **4 subagents** | Table of every subagent in the session, plus configured subagents that haven't been called yet (status `ready`) |

On launch a short animation (about 2 s) scans the agentwatch logo in the agents' colors while sessions load in the background. It always plays to the end; then press **Enter** to open the dashboard (`q` quits). `--no-splash` or `AGENTWATCH_NO_SPLASH=1` opens the dashboard directly.

Keys: `1-4`/`tab` switch view · `[` `]` switch session · `f` follow the most active session · `j/k`/mouse wheel to scroll · `q` quit.

Colors per model family: opus purple, sonnet orange, haiku blue, fable green, GPT/Codex cyan, Gemini pink. The header and LEAD box show which agent a session belongs to.

## Supported agents

| Agent | How agentwatch reads it | Setup |
|---|---|---|
| **Claude Code** | hooks (`events.jsonl`) + transcripts in `~/.claude/projects` | `agentwatch install` |
| **Codex CLI** | rollout files `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-*.jsonl` (`~/.codex` by default) | none |
| **Gemini CLI** | chat files `~/.gemini/tmp/<project>/chats/session-*.json` | none |
| **OpenCode** | JSON storage `~/.local/share/opencode/storage/{session,message,part}` | none |
| **oh-my-pi (`omp`)** | session files `~/.omp/agent/sessions/<project>/<time>_<id>.jsonl` (plus named profiles in `~/.omp/profiles/*/agent/sessions`) | none |
| **Anything else** | events piped to `agentwatch hook --agent <name>` | your tool's hook config |

All of them appear in the same dashboard: `[` `]` switches between sessions, whatever agent they come from. Codex, Gemini, OpenCode and omp are read straight from the files they already write, so they need no install. Only sessions active in the last 24 hours are loaded (`--window` changes this).

What each agent shows:
- **Model, prompts, tool calls, tokens (in / out / cache), turns, failures, model sequence**: all agents.
- **Subagents**: Claude Code (Agent tool), Codex (subagent threads: rollouts with `parent_thread_id`), OpenCode (child sessions from the task tool) and omp (workers saved as `<session>/<Name>.jsonl`, with their type such as `scout` or `task`).
- **Skills**: Claude Code (`Skill` tool, slash commands); OpenCode (`skill` tool); Gemini (`activate_skill`); Codex (when it reads a `…/<skill>/SKILL.md` file, which is how Codex loads skills); omp (`/skill:<name> …` prompts, and the `read` tool loading a `SKILL.md` or `skill://` file). Slash commands typed as a prompt count for every agent.
- **Advisor** and `.claude/agents` definitions: Claude Code only. Other agents show an **AGENT** box instead (agent, model, folder, and the file being read).

### Other tools: `agentwatch hook --agent <name>`
Any tool that can run a command on an event can feed the dashboard. Send one JSON object per event on stdin, using Claude Code's hook field names:

```bash
echo '{"session_id":"s1","cwd":"'"$PWD"'","hook_event_name":"UserPromptSubmit","prompt":"add tests","model":"gpt-5"}' | agentwatch hook --agent aider
```

| `hook_event_name` | Fields |
|---|---|
| `SessionStart` / `SessionEnd` | `model`, `cwd` |
| `UserPromptSubmit` | `prompt` |
| `PreToolUse` / `PostToolUse` / `PostToolUseFailure` | `tool_name`, `tool_input` (e.g. `{"command": …}`), `error` |
| `Message` | one model reply: `model`, `input_tokens`, `output_tokens`, `cache_read_tokens` |
| `Stop` | the turn finished |
| `SubagentStart` / `SubagentStop` | `agent_id`, `agent_type` |

Every event needs `session_id`; `cwd` is needed for the folder filter. Gemini-style hook names (`BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `PreCompress`) are accepted as aliases. Use your own name with `--agent` (e.g. `aider`): don't send hooks as `codex`, `gemini`, `opencode` or `omp`, because those sessions are already read from their files and the two sources would overwrite each other.

## How it works

Steps 1–5 are for Claude Code; step 6 covers the other agents.

1. **Hooks** (`agentwatch hook`): Claude Code calls it on every event (SessionStart, UserPromptSubmit, Pre/PostToolUse, SubagentStart/Stop, Stop, etc.). The payload is summarized and appended to `~/.claude/agentwatch/events.jsonl`. The hook never writes to stdout and always exits 0, so it never gets in Claude's way.
2. **Transcript**: the dashboard tails the session transcript `~/.claude/projects/<proj>/<session>.jsonl` and the subagent transcripts `…/<session>/subagents/agent-*.jsonl`. This is where it gets the **actual model** per message, tokens, tool counts, and advisor calls (`server_tool_use` named `advisor`).
3. **Skills**: recorded from the `Skill` tool calls Claude makes (PreToolUse hook + transcript, for both lead and subagents) and from slash commands you type (`/name args`, except built-ins like `/clear`, `/model`). The first word after the name is read as a **subcommand**, e.g. `/impeccable layout page …` → `impeccable layout`, with the remaining args as context. Skills the **lead** invoked in earlier prompts are shown as **still in context (lead)** (`◆`), because their content stays in the lead's context for the rest of the session; Claude Code even re-attaches them after a compact (the `invoked_skills` attachment in the transcript, which agentwatch also reads). Skills invoked by a **subagent** only live in that subagent's context, so they appear as **subagent history** (`◇`). Note: "in context" means the skill's instructions are available to Claude, not that Claude actually followed them in this prompt; the only ones certainly used are those shown at the top (invoked in the current prompt). Skills since the last prompt = the current task; they stay active while the lead/subagent that invoked them is still working.
4. **Subagent definitions**: `.claude/agents/*.md` in the project (searched upward from the session folder) and `~/.claude/agents/*.md`. The `name`, `model`, and `description` from the frontmatter are used to show subagents that are **ready but not called yet**. If names collide, the project version wins. They're re-read every 15 seconds, so new agents show up right away.
5. **Status line** (optional, `agentwatch statusline`): shows the lead + advisor models under the Claude Code prompt, and records `/model` switches for the dashboard.
6. **Other agents**: every ~1.5 s agentwatch looks for Codex, Gemini CLI, OpenCode and omp session files changed within `--window` (default 24 h). Codex and omp sessions are append-only JSONL, so they're tailed line by line like Claude transcripts. Gemini and OpenCode rewrite their JSON files, so a session is rebuilt from scratch whenever one of its files changes. Events from `agentwatch hook --agent <name>` go through the same `events.jsonl` as Claude Code's hooks, tagged with the agent name. A session still marked working whose files have been quiet for 5 minutes is shown as idle.

## Installation

### Use a prebuilt binary
```bash
# macOS Apple Silicon (pick the one for your OS/architecture)
mkdir -p ~/.local/bin
cp dist/agentwatch-darwin-arm64 ~/.local/bin/agentwatch
chmod +x ~/.local/bin/agentwatch
xattr -d com.apple.quarantine ~/.local/bin/agentwatch 2>/dev/null   # macOS: clear the Gatekeeper quarantine
```
Make sure `~/.local/bin` is on your `PATH`.

### Or build it yourself (Go 1.22+)
```bash
go mod tidy
go build -o ~/.local/bin/agentwatch .
```

### Or with `go install`
```bash
go install github.com/chicken-afk/agentwatch@latest   # installs to $(go env GOPATH)/bin/agentwatch
```

### Updating
```bash
agentwatch update            # download the latest main from GitHub, build it, replace this binary
agentwatch update --check    # only check whether a newer version exists
agentwatch version           # version + the git commit it was built from
```
- There are no prebuilt releases, so `update` needs **git** and **Go** on your `PATH`. It keeps its own clone of the repository in your cache folder (`~/Library/Caches/agentwatch/src` on macOS, `~/.cache/agentwatch/src` on Linux; `AGENTWATCH_SRC` overrides) and resets it to the remote branch every time, so don't edit files there.
- The binary is replaced in place (the same path `agentwatch` runs from), so installed hooks keep working. Restart open dashboards to use the new version.
- `--branch <name>` updates to another branch, `--force` rebuilds even when already up to date.
- `--src <folder>` builds your own checkout instead (e.g. `agentwatch update --src ~/src/agentwatch`): it fast-forwards the checkout when its branch tracks a remote, then builds it as it is, including uncommitted changes.
- `agentwatch update` only exists in builds that already have it. If yours says `unknown command: update`, build once by hand (`go build -o ~/.local/bin/agentwatch .`) and use `agentwatch update` from then on.

### Install the hooks (Claude Code only)
Codex, Gemini CLI, OpenCode and omp need no install: just run `agentwatch`.
```bash
agentwatch install --statusline
```
- Adds the agentwatch hooks to `~/.claude/settings.json` **without removing** existing hooks/settings, and creates a `settings.json.agentwatch-backup-*` backup.
- `--statusline` is only installed if you don't already have a `statusLine`.
- To remove: `agentwatch uninstall`.
- If you move the binary, run `agentwatch install` again so the path gets updated.

### Skill rules (applied to every project)
`agentwatch install` also adds a short rules block to `~/.claude/CLAUDE.md`, the user-level memory Claude Code loads in **every project**. It asks Claude to actually invoke skills through the `Skill` tool (not just mention them by name) and to report only the skills it actually invoked in that task. That way what Claude claims matches what the SKILLS box shows, without editing `AGENTS.md`/`CLAUDE.md` in each project.

```bash
agentwatch rules            # install/update the rules only (no hooks)
agentwatch rules --show     # print the rules without writing anything
agentwatch rules --remove   # remove the rules
agentwatch install --no-rules   # install hooks without the rules
```
- The block is wrapped in `<!-- agentwatch:skills:start -->` … `<!-- agentwatch:skills:end -->`; the rest of `CLAUDE.md` is left untouched, and a `CLAUDE.md.agentwatch-backup-*` backup is made whenever an existing file changes.
- `agentwatch uninstall` removes this block too.
- These are still instructions, not enforcement: Claude sometimes skips them. To be sure a skill runs, call it directly with a slash command (e.g. `/impeccable polish …`).

### Upgrading from ccwatch
agentwatch used to be called **ccwatch**. To move over, update once and reinstall:

```bash
ccwatch update        # builds the latest version (now agentwatch)
agentwatch install    # or `ccwatch install`: both run the same binary now
```
- The first `install` or `update` from the new version renames `~/.local/bin/ccwatch` to `agentwatch` and leaves `ccwatch` as a symlink, so old scripts and muscle memory keep working.
- `install` repoints the existing `ccwatch hook` / `ccwatch statusline` entries in `~/.claude/settings.json` to `agentwatch` (no duplicates), and swaps the `<!-- ccwatch:skills -->` block in `~/.claude/CLAUDE.md` for the agentwatch one. `uninstall` removes both old and new entries.
- Events now go to `~/.claude/agentwatch/events.jsonl`. Claude sessions that are still running with the old hooks keep writing to `~/.claude/ccwatch/events.jsonl`, which the dashboard still reads, so nothing disappears mid-session.
- The old `CCWATCH_*` environment variables still work; `AGENTWATCH_*` takes precedence.
- Once nothing uses the old name, you can remove the symlink: `rm ~/.local/bin/ccwatch`.

## Using it in Ghostty

1. Open a split: **macOS** `Cmd+D` (right) / `Cmd+Shift+D` (down), **Linux** `Ctrl+Shift+O` (right) / `Ctrl+Shift+E` (down).
2. Left pane: your agent, e.g. `claude --model sonnet --advisor opus`, `codex`, `gemini`, `opencode` or `omp`
3. Right pane: `agentwatch`

By default the dashboard **only shows agent sessions (Claude Code, Codex, Gemini, OpenCode, omp, …) in the folder where `agentwatch` was started** (including its subfolders), and automatically follows whichever of them was most recently active. Example: `cd ~/Works/personal/pos && agentwatch` → only sessions opened in `pos`, `pos/frontend`, `pos/backend`, etc. Ideal width is ≥ 110 columns (advisor on the left). In narrow panes the layout stacks automatically.

Try it without any agent: `agentwatch demo` (a live Claude Code session plus a finished Codex session; press `]` to switch).

### Watch options
```bash
agentwatch                        # only sessions in the current folder (+ subfolders)
agentwatch --all                  # sessions from all folders
agentwatch --dir ~/Works/personal/pos   # only sessions in a specific folder, wherever agentwatch is run from
agentwatch --session 7f3c9a21     # pin to a specific session (id prefix), no folder filter
agentwatch --advisor opus         # advisor label when it was set via the --advisor flag (not saved in settings)
agentwatch --recent 30m           # also show sessions active in the last 30 minutes, even without hooks installed
agentwatch --agent omp            # only omp sessions (claude, codex, gemini, opencode, omp, or a hook --agent name; comma-separated)
agentwatch --window 72h           # load Codex/Gemini/OpenCode/omp sessions from the last 72 hours (default 24h)
agentwatch --no-splash         # skip the startup animation (or set AGENTWATCH_NO_SPLASH=1)
```

### Per-session advisor labels
`--advisor opus` applies to **all** sessions. If each session uses a different advisor, label them by session id (an id prefix is fine):

```bash
agentwatch --advisor 7f3c=opus                          # only session 7f3c…
agentwatch --advisor 7f3c=opus --advisor 9c1b=sonnet    # several sessions (the flag can be repeated)
agentwatch --advisor 7f3c=opus,9c1b=sonnet              # same, comma-separated
agentwatch --advisor opus --advisor 9c1b=sonnet         # opus by default, except session 9c1b… uses sonnet
agentwatch --session 7f3c --advisor opus                # pin session 7f3c… with an advisor label for that session only
```

Advisor label priority:
1. The model from `usage.iterations` in the transcript (once the advisor has been called).
2. Per-session label `--advisor <id>=<model>` (longest matching prefix wins).
3. Global label `--advisor <model>`. If `--session` is set, this label only applies to that session.
4. `advisorModel` from settings: `~/.claude/settings.json`, then `<project>/.claude/settings.json`, then `<project>/.claude/settings.local.json` (the last one found wins).

The short session id is shown in the dashboard header (`session 7f3c9a21`) and can be used directly as a prefix. The full id is in the transcript file name `~/.claude/projects/<proj>/<session-id>.jsonl`.

## Honest notes

- **Advisor model**: the model name comes from `usage.iterations` in the transcript when available, otherwise from `advisorModel` in settings. If you use `claude --advisor opus` (a flag, not settings), run `agentwatch --advisor opus` so the label is correct (or `agentwatch --advisor <session-id>=opus` for a single session).
- The Claude Code transcript format isn't an official API and can change between versions. If some numbers go blank after a Claude Code update, the format most likely changed.
- Subagents in `.claude/agents` are only *available*; Claude decides when to delegate. If one stays `ready`, ask for it directly, e.g. "use the search agent to find …".
- Claude Code sometimes sends "ghost" `SubagentStop` events for its internal summarizer (no `agent_type`, no transcript). agentwatch ignores them.
- Images you may have seen online show a "JEV fork layer" with scores like p=0.81 and so on. That isn't data Claude Code emits, so it isn't shown. Forked subagents still appear as regular subagents, with the type Claude Code reports.
- `events.jsonl` is rotated automatically once it exceeds 20 MB.
- **Codex, Gemini CLI and OpenCode** are read from their on-disk formats, which aren't official APIs either. The Codex token, session_meta and turn_context fields were checked against Codex's source; the rest (Gemini chat files, OpenCode storage) was written from memory of those formats. All three are tested only with sample files, not yet with real sessions on this machine. If a field changes, that agent's numbers go blank while the others keep working.
- Codex reports one token count per model response; agentwatch counts it once even when Codex repeats it. Context Codex injects as a "user" message (`<environment_context>`, AGENTS.md) isn't counted as a prompt.
- Gemini CLI: older versions name the project folder `sha256(project path)`. agentwatch can only match it against the folder you run it in (or `--dir`); sessions from subfolders show up with `--all`. Newer versions store the path in `.project_root`, which works everywhere.
- OpenCode: only the JSON file storage is read. If your OpenCode version keeps sessions in a database instead, those sessions won't show.
- **omp** was checked against real omp sessions on this machine (session header, messages, `model_usage`, `session_init`, `skill-prompt`, subagent files). omp's own side calls (title generation, job captions, completion probes) are shown in MODEL USAGE under their purpose as the role, but not counted in the LEAD tokens. omp's advisor isn't shown yet: its entries haven't appeared in any session here, so the format is unknown. A session whose omp process exited shows as `ended` until you resume it.
- **Antigravity** (`~/.gemini/antigravity*`) stores conversations in a binary format and isn't supported. Gemini CLI uses `~/.gemini/tmp`.
- Folders can be overridden: `CODEX_HOME`, `AGENTWATCH_GEMINI_DIR`, `AGENTWATCH_OPENCODE_DIR` (or `XDG_DATA_HOME`), `AGENTWATCH_OMP_DIR`.

## Code layout

`main.go` is only the entry point; the logic lives in `internal/`:

| Package | Responsibility |
|---|---|
| `cli` | subcommand dispatch and flag parsing |
| `config` | file locations (`~/.claude`, `AGENTWATCH_DIR`) and reading `advisorModel` |
| `hook`, `statusline`, `events` | `agentwatch hook` / `agentwatch statusline` and writing `events.jsonl` |
| `install` | install/remove hooks, the status line, and skill rules (`settings.json`, `CLAUDE.md`) |
| `store` | session state from `events.jsonl` + transcripts (models, subagents, skills, advisor); `codex.go`, `gemini.go`, `opencode.go`, `omp.go` read the other agents' session files |
| `ui`, `theme` | Bubble Tea dashboard and color palette |
| `demo` | fake session for `agentwatch demo` |
| `format`, `pathmatch` | text/number formatting helpers and session-folder matching |

Quick check before committing: `go vet ./... && go test ./...`
