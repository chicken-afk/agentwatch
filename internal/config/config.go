// Package config menyelesaikan lokasi file ccwatch/Claude Code dan membaca
// pengaturan Claude Code yang relevan. Path dibaca dari environment tiap
// dipanggil, jadi perubahan env (mis. oleh `ccwatch demo`) langsung berlaku.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClaudeDir: ~/.claude (atau CLAUDE_CONFIG_DIR kalau di-set).
func ClaudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// DataDir: tempat ccwatch menyimpan events.jsonl (CCWATCH_DIR menimpa default).
func DataDir() string {
	if d := os.Getenv("CCWATCH_DIR"); d != "" {
		return d
	}
	return filepath.Join(ClaudeDir(), "ccwatch")
}

func EventsPath() string { return filepath.Join(DataDir(), "events.jsonl") }

func SettingsPath() string { return filepath.Join(ClaudeDir(), "settings.json") }

// MemoryPath: CLAUDE.md di folder yang sama dengan settings.json.
func MemoryPath(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), "CLAUDE.md")
}

// AdvisorModel membaca advisorModel dari settings: user, lalu proyek, lalu
// proyek-lokal. Yang terakhir ditemukan menang; "" kalau tidak ada.
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
