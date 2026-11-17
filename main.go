// agentwatch — live dashboard for Claude Code sessions.
//
// Subcommands:
//
//	agentwatch                 open the dashboard (follow the most active session)
//	agentwatch watch [flags]   same as above
//	agentwatch hook            called by Claude Code hooks (reads JSON from stdin)
//	agentwatch statusline      compact status line for Claude Code
//	agentwatch install         install hooks into ~/.claude/settings.json + skill rules into ~/.claude/CLAUDE.md
//	agentwatch uninstall       remove agentwatch hooks and skill rules
//	agentwatch rules           install/update only the skill rules (--remove, --show)
//	agentwatch demo            run the dashboard with a fake session (to try it out)
//
// The logic lives in the internal/* packages; this file is only the entry point.
package main

import (
	"os"

	"github.com/chicken-afk/agentwatch/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
