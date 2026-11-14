package cli

import (
	"flag"
	"fmt"

	"ccwatch/internal/config"
	"ccwatch/internal/install"
)

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	statusline := fs.Bool("statusline", false, "pasang juga status line ccwatch (kalau belum ada statusLine lain)")
	settings := fs.String("settings", config.SettingsPath(), "path settings.json")
	noRules := fs.Bool("no-rules", false, "jangan pasang aturan skill ke ~/.claude/CLAUDE.md")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := install.InstallHooks(*settings, *statusline); err != nil {
		return fmt.Errorf("install gagal: %w", err)
	}
	if !*noRules {
		if err := install.InstallRules(config.MemoryPath(*settings)); err != nil {
			return fmt.Errorf("pasang aturan skill gagal: %w", err)
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
		return fmt.Errorf("uninstall gagal: %w", err)
	}
	if err := install.UninstallRules(config.MemoryPath(*settings)); err != nil {
		return fmt.Errorf("lepas aturan skill gagal: %w", err)
	}
	return nil
}

// runRules memasang/melepas hanya aturan skill (tanpa menyentuh hooks).
func runRules(args []string) error {
	fs := flag.NewFlagSet("rules", flag.ExitOnError)
	settings := fs.String("settings", config.SettingsPath(), "path settings.json (CLAUDE.md di folder yang sama)")
	remove := fs.Bool("remove", false, "lepas aturan skill ccwatch")
	show := fs.Bool("show", false, "tampilkan isi aturan tanpa menulis apa pun")
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
		return fmt.Errorf("rules gagal: %w", err)
	}
	return nil
}
