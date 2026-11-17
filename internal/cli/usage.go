package cli

const usage = `ccwatch — live dashboard for AI coding agents (Claude Code, Codex, Gemini CLI, OpenCode, …)

  ccwatch                  open the dashboard (only sessions in the current folder + subfolders)
  ccwatch --all            sessions from all folders
  ccwatch --dir ~/Works/personal/pos   only sessions in a specific folder
  ccwatch --agent codex,gemini            only sessions from these agents (claude, codex, gemini, opencode, <name>)
  ccwatch --window 48h                    how far back to load Codex/Gemini/OpenCode sessions (default 24h)
  ccwatch watch --session <id> --advisor opus --recent 30m
  ccwatch --advisor opus                  advisor label for all sessions
  ccwatch --advisor 3f2a=opus             advisor label only for session 3f2a… (prefix allowed)
  ccwatch --advisor 3f2a=opus --advisor 9c1b=sonnet
  ccwatch --session 3f2a --advisor opus   pin session 3f2a… + advisor label for that session only
  ccwatch install [--statusline] [--no-rules]   hooks + skill rules in ~/.claude/CLAUDE.md
  ccwatch uninstall                              remove hooks + skill rules
  ccwatch rules [--remove | --show]              skill rules only
  ccwatch demo             try the dashboard with fake data
  ccwatch hook             (called by Claude Code)
  ccwatch hook --agent <name>   events from any other tool (JSON on stdin, same fields as Claude hooks)
  ccwatch statusline       (called by Claude Code)`
