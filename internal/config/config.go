// Package config resolves ccwatch/Claude Code file locations and reads the
// relevant Claude Code settings. Paths are read from the environment on every
// call, so env changes (e.g. by `ccwatch demo`) take effect immediately.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClaudeDir: ~/.claude (or CLAUDE_CONFIG_DIR if set).
func ClaudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// DataDir: where ccwatch stores events.jsonl (CCWATCH_DIR overrides the default).
func DataDir() string {
	if d := os.Getenv("CCWATCH_DIR"); d != "" {
		return d
	}
	return filepath.Join(ClaudeDir(), "ccwatch")
}

func EventsPath() string { return filepath.Join(DataDir(), "events.jsonl") }

func SettingsPath() string { return filepath.Join(ClaudeDir(), "settings.json") }

// MemoryPath: CLAUDE.md in the same folder as settings.json.
func MemoryPath(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), "CLAUDE.md")
}

// AdvisorModel reads advisorModel from settings: user, then project, then
// project-local. The last one found wins; "" if none.
func AdvisorModel(cwd string) string {
	paths := []string{SettingsPath()}
	if cwd != "" {
		paths = append(paths,
			filepath.Join(cwd, ".claude", "settings.json"),
			filepath.Join(cwd, ".claude", "settings.local.json"))
	}
	val := ""
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		if v, _ := m["advisorModel"].(string); v != "" {
			val = v
		}
	}
	return val
}
