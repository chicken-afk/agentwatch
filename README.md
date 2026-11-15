# ccwatch — live dashboard for Claude Code

A terminal dashboard that shows **which models are used in a Claude Code session, and in what order**:
the lead (main session), the advisor, and every subagent, live in a Ghostty split next to Claude Code.

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
| **1 dashboard** | LEAD box (model, status, running tool, prompt, tokens), ADVISOR (model, call count, history + the context that triggered each call), SKILLS (skills used in the current prompt: who invoked them, still active `●` or done `✓`, plus a summary of earlier prompts), MODEL SEQUENCE (chain + latest steps), SUBAGENT cards (model, running/done, duration, tools, tokens, task), MODEL USAGE table (role, messages, tokens in/out/cache, output share, active time), SESSION LOG |
| **2 model sequence** | Every model step since the session started, in order, including gaps |
| **3 log** | All events: prompts, tool calls, failures, subagent dispatches, advisor, compact |
| **4 subagents** | Table of every subagent in the session, plus configured subagents that haven't been called yet (status `ready`) |

Keys: `1-4`/`tab` switch view · `[` `]` switch session · `f` follow the most active session · `j/k`/mouse wheel to scroll · `q` quit.

Colors per model family: opus purple, sonnet orange, haiku blue, fable green.

## How it works

1. **Hooks** (`ccwatch hook`): Claude Code calls it on every event (SessionStart, UserPromptSubmit, Pre/PostToolUse, SubagentStart/Stop, Stop, etc.). The payload is summarized and appended to `~/.claude/ccwatch/events.jsonl`. The hook never writes to stdout and always exits 0, so it never gets in Claude's way.
2. **Transcript**: the dashboard tails the session transcript `~/.claude/projects/<proj>/<session>.jsonl` and the subagent transcripts `…/<session>/subagents/agent-*.jsonl`. This is where it gets the **actual model** per message, tokens, tool counts, and advisor calls (`server_tool_use` named `advisor`).
3. **Skills**: recorded from the `Skill` tool calls Claude makes (PreToolUse hook + transcript, for both lead and subagents) and from slash commands you type (`/name args`, except built-ins like `/clear`, `/model`). The first word after the name is read as a **subcommand**, e.g. `/impeccable layout page …` → `impeccable layout`, with the remaining args as context. Skills the **lead** invoked in earlier prompts are shown as **still in context (lead)** (`◆`), because their content stays in the lead's context for the rest of the session; Claude Code even re-attaches them after a compact (the `invoked_skills` attachment in the transcript, which ccwatch also reads). Skills invoked by a **subagent** only live in that subagent's context, so they appear as **subagent history** (`◇`). Note: "in context" means the skill's instructions are available to Claude, not that Claude actually followed them in this prompt; the only ones certainly used are those shown at the top (invoked in the current prompt). Skills since the last prompt = the current task; they stay active while the lead/subagent that invoked them is still working.
4. **Subagent definitions**: `.claude/agents/*.md` in the project (searched upward from the session folder) and `~/.claude/agents/*.md`. The `name`, `model`, and `description` from the frontmatter are used to show subagents that are **ready but not called yet**. If names collide, the project version wins. They're re-read every 15 seconds, so new agents show up right away.
5. **Status line** (optional, `ccwatch statusline`): shows the lead + advisor models under the Claude Code prompt, and records `/model` switches for the dashboard.

## Installation

### Use a prebuilt binary
```bash
# macOS Apple Silicon (pick the one for your OS/architecture)
mkdir -p ~/.local/bin
cp dist/ccwatch-darwin-arm64 ~/.local/bin/ccwatch
chmod +x ~/.local/bin/ccwatch
xattr -d com.apple.quarantine ~/.local/bin/ccwatch 2>/dev/null   # macOS: clear the Gatekeeper quarantine
```
Make sure `~/.local/bin` is on your `PATH`.

### Or build it yourself (Go 1.22+)
```bash
go mod tidy
go build -o ~/.local/bin/ccwatch .
```

### Install the hooks
```bash
ccwatch install --statusline
```
- Adds the ccwatch hooks to `~/.claude/settings.json` **without removing** existing hooks/settings, and creates a `settings.json.ccwatch-backup-*` backup.
- `--statusline` is only installed if you don't already have a `statusLine`.
- To remove: `ccwatch uninstall`.
- If you move the binary, run `ccwatch install` again so the path gets updated.

### Skill rules (applied to every project)
`ccwatch install` also adds a short rules block to `~/.claude/CLAUDE.md`, the user-level memory Claude Code loads in **every project**. It asks Claude to actually invoke skills through the `Skill` tool (not just mention them by name) and to report only the skills it actually invoked in that task. That way what Claude claims matches what the SKILLS box shows, without editing `AGENTS.md`/`CLAUDE.md` in each project.

```bash
ccwatch rules            # install/update the rules only (no hooks)
ccwatch rules --show     # print the rules without writing anything
ccwatch rules --remove   # remove the rules
ccwatch install --no-rules   # install hooks without the rules
```
- The block is wrapped in `<!-- ccwatch:skills:start -->` … `<!-- ccwatch:skills:end -->`; the rest of `CLAUDE.md` is left untouched, and a `CLAUDE.md.ccwatch-backup-*` backup is made whenever an existing file changes.
- `ccwatch uninstall` removes this block too.
- These are still instructions, not enforcement: Claude sometimes skips them. To be sure a skill runs, call it directly with a slash command (e.g. `/impeccable polish …`).

## Using it in Ghostty

1. Open a split: **macOS** `Cmd+D` (right) / `Cmd+Shift+D` (down), **Linux** `Ctrl+Shift+O` (right) / `Ctrl+Shift+E` (down).
2. Left pane: `claude --model sonnet --advisor opus`
3. Right pane: `ccwatch`

By default the dashboard **only shows Claude Code sessions in the folder where `ccwatch` was started** (including its subfolders), and automatically follows whichever of them was most recently active. Example: `cd ~/Works/personal/pos && ccwatch` → only sessions opened in `pos`, `pos/frontend`, `pos/backend`, etc. Ideal width is ≥ 110 columns (advisor on the left). In narrow panes the layout stacks automatically.

Try it without Claude Code: `ccwatch demo`.

### Watch options
```bash
ccwatch                        # only sessions in the current folder (+ subfolders)
ccwatch --all                  # sessions from all folders
ccwatch --dir ~/Works/personal/pos   # only sessions in a specific folder, wherever ccwatch is run from
ccwatch --session 7f3c9a21     # pin to a specific session (id prefix), no folder filter
ccwatch --advisor opus         # advisor label when it was set via the --advisor flag (not saved in settings)
ccwatch --recent 30m           # also show sessions active in the last 30 minutes, even without hooks installed
```

### Per-session advisor labels
`--advisor opus` applies to **all** sessions. If each session uses a different advisor, label them by session id (an id prefix is fine):

```bash
ccwatch --advisor 7f3c=opus                          # only session 7f3c…
ccwatch --advisor 7f3c=opus --advisor 9c1b=sonnet    # several sessions (the flag can be repeated)
ccwatch --advisor 7f3c=opus,9c1b=sonnet              # same, comma-separated
ccwatch --advisor opus --advisor 9c1b=sonnet         # opus by default, except session 9c1b… uses sonnet
ccwatch --session 7f3c --advisor opus                # pin session 7f3c… with an advisor label for that session only
```

Advisor label priority:
1. The model from `usage.iterations` in the transcript (once the advisor has been called).
2. Per-session label `--advisor <id>=<model>` (longest matching prefix wins).
3. Global label `--advisor <model>`. If `--session` is set, this label only applies to that session.
4. `advisorModel` from settings: `~/.claude/settings.json`, then `<project>/.claude/settings.json`, then `<project>/.claude/settings.local.json` (the last one found wins).

The short session id is shown in the dashboard header (`session 7f3c9a21`) and can be used directly as a prefix. The full id is in the transcript file name `~/.claude/projects/<proj>/<session-id>.jsonl`.

## Honest notes

- **Advisor model**: the model name comes from `usage.iterations` in the transcript when available, otherwise from `advisorModel` in settings. If you use `claude --advisor opus` (a flag, not settings), run `ccwatch --advisor opus` so the label is correct (or `ccwatch --advisor <session-id>=opus` for a single session).
- The Claude Code transcript format isn't an official API and can change between versions. If some numbers go blank after a Claude Code update, the format most likely changed.
- Subagents in `.claude/agents` are only *available*; Claude decides when to delegate. If one stays `ready`, ask for it directly, e.g. "use the search agent to find …".
- Claude Code sometimes sends "ghost" `SubagentStop` events for its internal summarizer (no `agent_type`, no transcript). ccwatch ignores them.
- Images you may have seen online show a "JEV fork layer" with scores like p=0.81 and so on. That isn't data Claude Code emits, so it isn't shown. Forked subagents still appear as regular subagents, with the type Claude Code reports.
- `events.jsonl` is rotated automatically once it exceeds 20 MB.

## Code layout

`main.go` is only the entry point; the logic lives in `internal/`:

| Package | Responsibility |
|---|---|
| `cli` | subcommand dispatch and flag parsing |
| `config` | file locations (`~/.claude`, `CCWATCH_DIR`) and reading `advisorModel` |
| `hook`, `statusline`, `events` | `ccwatch hook` / `ccwatch statusline` and writing `events.jsonl` |
| `install` | install/remove hooks, the status line, and skill rules (`settings.json`, `CLAUDE.md`) |
| `store` | session state from `events.jsonl` + transcripts (models, subagents, skills, advisor) |
| `ui`, `theme` | Bubble Tea dashboard and color palette |
| `demo` | fake session for `ccwatch demo` |
| `format`, `pathmatch` | text/number formatting helpers and session-folder matching |

Quick check before committing: `go vet ./... && go test ./...`
