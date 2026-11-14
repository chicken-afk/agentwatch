package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var hookEvents = []string{
	"SessionStart", "SessionEnd", "UserPromptSubmit",
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"SubagentStart", "SubagentStop", "Stop",
	"PreCompact", "Notification",
}

// selfCommand: perintah untuk memanggil ccwatch ini sendiri dengan subcommand sub.
func selfCommand(sub string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if strings.ContainsAny(exe, " \t'\"") {
		exe = `"` + exe + `"`
	}
	return exe + " " + sub, nil
}

// readSettings: isi settings.json beserta byte aslinya (nil kalau file belum ada).
func readSettings(path string) (map[string]any, []byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	s := map[string]any{}
	if len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, nil, fmt.Errorf("%s bukan JSON valid: %w", path, err)
		}
	}
	return s, b, nil
}

func writeSettings(path string, s map[string]any, orig []byte) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeWithBackup(path, append(b, '\n'), orig)
}

// isOurs: apakah command berakhir dengan " <sub>" dan menunjuk ke ccwatch.
func isOurs(cmd, sub string) bool {
	return strings.Contains(cmd, "ccwatch") && strings.HasSuffix(strings.TrimSpace(cmd), " "+sub)
}

// commandOf: field "command" dari satu hook (kosong kalau bukan object/string).
func commandOf(h any) string {
	hm, _ := h.(map[string]any)
	c, _ := hm["command"].(string)
	return c
}

// InstallHooks menambah hook ccwatch ke settings.json di path tanpa menghapus
// hook/setting lain. Hook ccwatch yang sudah ada hanya diperbarui path-nya.
func InstallHooks(path string, withStatusline bool) error {
	cmd, err := selfCommand("hook")
	if err != nil {
		return err
	}
	s, orig, err := readSettings(path)
	if err != nil {
		return err
	}
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	added := 0
	for _, ev := range hookEvents {
		groups, _ := hooks[ev].([]any)
		if !refreshHookCommand(groups, cmd) {
			groups = append(groups, map[string]any{
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": cmd,
					"timeout": 5,
				}},
			})
			added++
		}
		hooks[ev] = groups
	}
	s["hooks"] = hooks

	if withStatusline {
		installStatusline(s)
	}

	if err := writeSettings(path, s, orig); err != nil {
		return err
	}
	fmt.Printf("hooks ccwatch terpasang di %s (%d event baru, %d total)\n", path, added, len(hookEvents))
	fmt.Println("Mulai sesi Claude Code baru supaya hooks aktif, lalu jalankan `ccwatch` di split sebelah.")
	return nil
}

// refreshHookCommand memperbarui command hook ccwatch yang sudah ada di groups
// (path binary bisa berubah). Mengembalikan true kalau ada yang ditemukan.
func refreshHookCommand(groups []any, cmd string) bool {
	found := false
	for _, g := range groups {
		gm, _ := g.(map[string]any)
		hs, _ := gm["hooks"].([]any)
		for _, h := range hs {
			if isOurs(commandOf(h), "hook") {
				h.(map[string]any)["command"] = cmd
				found = true
			}
		}
	}
	return found
}

func installStatusline(s map[string]any) {
	if _, exists := s["statusLine"]; exists {
		fmt.Println("statusLine sudah ada, tidak saya timpa. Hapus dulu kalau mau pakai milik ccwatch.")
		return
	}
	sc, _ := selfCommand("statusline")
	s["statusLine"] = map[string]any{"type": "command", "command": sc, "padding": 0}
	fmt.Println("statusLine ccwatch dipasang.")
}

// UninstallHooks melepas hook dan status line ccwatch dari settings.json di path.
func UninstallHooks(path string) error {
	s, orig, err := readSettings(path)
	if err != nil {
		return err
	}
	if orig == nil {
		fmt.Println("settings.json tidak ada, tidak ada yang dilepas.")
		return nil
	}
	hooks, _ := s["hooks"].(map[string]any)
	removed := 0
	for ev, v := range hooks {
		groups, _ := v.([]any)
		kept, n := withoutOurHooks(groups)
		removed += n
		if len(kept) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = kept
		}
	}
	if hooks != nil && len(hooks) == 0 {
		delete(s, "hooks")
	}
	if sl, ok := s["statusLine"].(map[string]any); ok {
		if c, _ := sl["command"].(string); isOurs(c, "statusline") {
			delete(s, "statusLine")
			fmt.Println("statusLine ccwatch dilepas.")
		}
	}
	if err := writeSettings(path, s, orig); err != nil {
		return err
	}
	fmt.Printf("%d hook ccwatch dilepas dari %s\n", removed, path)
	return nil
}

// withoutOurHooks membuang hook ccwatch dari groups; group yang jadi kosong ikut dibuang.
// Mengembalikan group yang tersisa dan jumlah hook yang dilepas.
func withoutOurHooks(groups []any) (kept []any, removed int) {
	for _, g := range groups {
		gm, _ := g.(map[string]any)
		hs, _ := gm["hooks"].([]any)
		var keptHooks []any
		for _, h := range hs {
			if isOurs(commandOf(h), "hook") {
				removed++
				continue
			}
			keptHooks = append(keptHooks, h)
		}
		if len(keptHooks) > 0 {
			gm["hooks"] = keptHooks
			kept = append(kept, gm)
		}
	}
	return kept, removed
}
