// ccwatch — live dashboard for Claude Code sessions.
//
// Subcommands:
//
//	ccwatch                 open the dashboard (follow the most active session)
//	ccwatch watch [flags]   same as above
//	ccwatch hook            called by Claude Code hooks (reads JSON from stdin)
//	ccwatch statusline      compact status line for Claude Code
//	ccwatch install         install hooks into ~/.claude/settings.json + skill rules into ~/.claude/CLAUDE.md
//	ccwatch uninstall       remove ccwatch hooks and skill rules
//	ccwatch rules           install/update only the skill rules (--remove, --show)
//	ccwatch demo            run the dashboard with a fake session (to try it out)
//
// The logic lives in the internal/* packages; this file is only the entry point.
package main

import (
	"os"

	"ccwatch/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
