// Package config resolves agentwatch/Claude Code file locations and reads the
// relevant Claude Code settings. Paths are read from the environment on every
// call, so env changes (e.g. by `agentwatch demo`) take effect immediately.
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

// DataDir: where agentwatch stores events.jsonl (AGENTWATCH_DIR overrides the default).
func DataDir() string {
	if d := Env("DIR"); d != "" {
		return d
	}
	return filepath.Join(ClaudeDir(), "agentwatch")
}

func EventsPath() string { return filepath.Join(DataDir(), "events.jsonl") }

// Env reads AGENTWATCH_<name>, falling back to the pre-rebrand CCWATCH_<name>.
func Env(name string) string {
	if v := os.Getenv("AGENTWATCH_" + name); v != "" {
		return v
	}
	return os.Getenv("CCWATCH_" + name)
}

// LegacyEventsPath: where hooks installed before the rebrand (ccwatch) still write.
// The dashboard reads it too, so Claude sessions started with the old hooks stay visible.
func LegacyEventsPath() string { return filepath.Join(ClaudeDir(), "ccwatch", "events.jsonl") }

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

// CodexDir: Codex CLI home ($CODEX_HOME or ~/.codex). Session logs live in sessions/YYYY/MM/DD/rollout-*.jsonl.
func CodexDir() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

// GeminiDir: Gemini CLI home (~/.gemini, or AGENTWATCH_GEMINI_DIR). Chats live in tmp/<project>/chats/session-*.json.
func GeminiDir() string {
	if d := Env("GEMINI_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini")
}

// OpenCodeDir: OpenCode data folder ($XDG_DATA_HOME/opencode or ~/.local/share/opencode).
func OpenCodeDir() string {
	if d := Env("OPENCODE_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode")
}

// OmpDir: oh-my-pi (omp) home (~/.omp, or AGENTWATCH_OMP_DIR). Sessions live in
// agent/sessions/<encoded-cwd>/<time>_<id>.jsonl (and profiles/<name>/agent/sessions/…).
func OmpDir() string {
	if d := Env("OMP_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".omp")
}
