package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/chicken-afk/agentwatch/internal/config"
	"github.com/chicken-afk/agentwatch/internal/install"
)

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	statusline := fs.Bool("statusline", false, "also install the agentwatch status line (only if no other statusLine is set)")
	settings := fs.String("settings", config.SettingsPath(), "path settings.json")
	noRules := fs.Bool("no-rules", false, "do not install the skill rules into ~/.claude/CLAUDE.md")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// pre-rebrand installs: move …/ccwatch to …/agentwatch first, so the hooks point at the new name
	if _, err := install.MigrateBinaryName(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	if err := install.InstallHooks(*settings, *statusline); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}
	if !*noRules {
		if err := install.InstallRules(config.MemoryPath(*settings)); err != nil {
			return fmt.Errorf("installing skill rules failed: %w", err)
		}
	}
	return nil
}

func runUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	settings := fs.String("settings", config.SettingsPath(), "path settings.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := install.UninstallHooks(*settings); err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}
	if err := install.UninstallRules(config.MemoryPath(*settings)); err != nil {
		return fmt.Errorf("removing skill rules failed: %w", err)
	}
	return nil
}

// runRules installs/removes only the skill rules (without touching hooks).
func runRules(args []string) error {
	fs := flag.NewFlagSet("rules", flag.ExitOnError)
	settings := fs.String("settings", config.SettingsPath(), "path to settings.json (CLAUDE.md lives in the same folder)")
	remove := fs.Bool("remove", false, "remove the agentwatch skill rules")
	show := fs.Bool("show", false, "print the rules without writing anything")
	if err := fs.Parse(args); err != nil {
		return err
	}
	memory := config.MemoryPath(*settings)
	var err error
	switch {
	case *show:
		fmt.Println(install.RulesBlock())
	case *remove:
		err = install.UninstallRules(memory)
	default:
		err = install.InstallRules(memory)
	}
	if err != nil {
		return fmt.Errorf("rules failed: %w", err)
	}
	return nil
}
