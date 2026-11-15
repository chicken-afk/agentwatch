package cli

const usage = `ccwatch — live dashboard for Claude Code

  ccwatch                  open the dashboard (only sessions in the current folder + subfolders)
  ccwatch --all            sessions from all folders
  ccwatch --dir ~/Works/personal/pos   only sessions in a specific folder
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
  ccwatch statusline       (called by Claude Code)`
