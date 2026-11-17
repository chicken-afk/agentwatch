package cli

const usage = `agentwatch — live dashboard for AI coding agents (Claude Code, Codex, Gemini CLI, OpenCode, omp, …)

  agentwatch                  open the dashboard (only sessions in the current folder + subfolders)
  agentwatch --all            sessions from all folders
  agentwatch --dir ~/Works/personal/pos   only sessions in a specific folder
  agentwatch --agent codex,gemini            only sessions from these agents (claude, codex, gemini, opencode, omp, <name>)
  agentwatch --window 48h                    how far back to load Codex/Gemini/OpenCode/omp sessions (default 24h)
  agentwatch watch --session <id> --advisor opus --recent 30m
  agentwatch --advisor opus                  advisor label for all sessions
  agentwatch --advisor 3f2a=opus             advisor label only for session 3f2a… (prefix allowed)
  agentwatch --advisor 3f2a=opus --advisor 9c1b=sonnet
  agentwatch --session 3f2a --advisor opus   pin session 3f2a… + advisor label for that session only
  agentwatch install [--statusline] [--no-rules]   hooks + skill rules in ~/.claude/CLAUDE.md
  agentwatch uninstall                              remove hooks + skill rules
  agentwatch rules [--remove | --show]              skill rules only
  agentwatch demo             try the dashboard with fake data
  agentwatch hook             (called by Claude Code)
  agentwatch hook --agent <name>   events from any other tool (JSON on stdin, same fields as Claude hooks)
  agentwatch statusline       (called by Claude Code)`
