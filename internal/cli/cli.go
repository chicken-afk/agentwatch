// Package cli: ccwatch subcommand dispatch and flag parsing.
package cli

import (
	"fmt"
	"os"
	"strings"

	"ccwatch/internal/demo"
	"ccwatch/internal/hook"
	"ccwatch/internal/statusline"
)

const Version = "0.1.0"

type command func(args []string) error

var commands = map[string]command{
	"watch":      runWatch,
	"hook":       runHook,
	"statusline": func([]string) error { statusline.Run(os.Stdin, os.Stdout); return nil },
	"install":    runInstall,
	"uninstall":  runUninstall,
	"rules":      runRules,
	"demo":       demo.Run,
	"version":    func([]string) error { fmt.Println("ccwatch", Version); return nil },
	"help":       func([]string) error { fmt.Println(usage); return nil },
}

// runHook: `ccwatch hook [--agent <name>]`. Never fails: a hook must not get in the agent's way.
func runHook(args []string) error {
	agent := ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case (a == "--agent" || a == "-agent") && i+1 < len(args):
			agent = args[i+1]
			i++
		case strings.HasPrefix(a, "--agent="):
			agent = strings.TrimPrefix(a, "--agent=")
		}
	}
	hook.Run(os.Stdin, agent)
	return nil
}

// splitCommand separates the subcommand from the arguments. No subcommand (empty or
// starting with "-") means `watch`; --version/-v is a shortcut for `version`.
func splitCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "watch", nil
	}
	switch first := args[0]; {
	case first == "--version" || first == "-v":
		return "version", args[1:]
	case first != "" && first[0] != '-':
		return first, args[1:]
	}
	return "watch", args
}

// Run runs ccwatch and returns the exit code: 0 success, 1 failure, 2 unknown command.
func Run(args []string) int {
	name, rest := splitCommand(args)
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintln(os.Stderr, "unknown command:", name)
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	if err := cmd(rest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
