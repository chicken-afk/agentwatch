// Package cli: agentwatch subcommand dispatch and flag parsing.
package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/chicken-afk/agentwatch/internal/demo"
	"github.com/chicken-afk/agentwatch/internal/hook"
	"github.com/chicken-afk/agentwatch/internal/statusline"
	"github.com/chicken-afk/agentwatch/internal/update"
)

// Version is set at build time for releases (-ldflags -X …/internal/cli.Version=0.2.0);
// builds from source report "dev".
var Version = "dev"

type command func(args []string) error

var commands = map[string]command{
	"watch":      runWatch,
	"hook":       runHook,
	"statusline": func([]string) error { statusline.Run(os.Stdin, os.Stdout); return nil },
	"install":    runInstall,
	"uninstall":  runUninstall,
	"rules":      runRules,
	"demo":       demo.Run,
	"update":     runUpdate,
	"version":    runVersion,
	"help":       func([]string) error { fmt.Println(usage); return nil },
}

// runVersion prints the version plus the git commit it was built from.
func runVersion([]string) error {
	rev, modified := update.Revision()
	switch {
	case rev == "":
		fmt.Println("agentwatch", Version)
	case modified:
		fmt.Printf("agentwatch %s (%s + local changes)\n", Version, rev[:min(7, len(rev))])
	default:
		fmt.Printf("agentwatch %s (%s)\n", Version, rev[:min(7, len(rev))])
	}
	return nil
}

// runUpdate: `agentwatch update [--check] [--force] [--source | --branch <b> | --src <checkout>]`.
func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	var o update.Options
	fs.BoolVar(&o.Check, "check", false, "only check whether a newer version exists")
	fs.BoolVar(&o.Force, "force", false, "update even if already up to date")
	fs.BoolVar(&o.Source, "source", false, "build the latest main from source (needs git + Go) instead of downloading the latest release")
	fs.StringVar(&o.Branch, "branch", "", "build this branch from source (implies --source)")
	fs.StringVar(&o.Repo, "repo", update.DefaultRepo, "git repository to update from")
	fs.StringVar(&o.Src, "src", "", "build from this local checkout (git pull --ff-only) instead of agentwatch's own clone")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o.Version = Version
	return update.Run(o)
}

// runHook: `agentwatch hook [--agent <name>]`. Never fails: a hook must not get in the agent's way.
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

// Run runs agentwatch and returns the exit code: 0 success, 1 failure, 2 unknown command.
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
