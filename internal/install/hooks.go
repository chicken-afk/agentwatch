package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

var hookEvents = []string{
	"SessionStart", "SessionEnd", "UserPromptSubmit",
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"SubagentStart", "SubagentStop", "Stop",
	"PreCompact", "Notification",
}

// selfCommand: the command that invokes this agentwatch binary with subcommand sub.
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

// readSettings: settings.json content plus its original bytes (nil if the file doesn't exist yet).
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
			return nil, nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}
	return s, b, nil
}

// writeSettings writes s to path (with a backup of orig) and reports whether anything changed.
func writeSettings(path string, s map[string]any, orig []byte) (bool, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return false, err
	}
	if orig != nil && sameJSON(b, orig) {
		return false, nil // nothing changed: no rewrite, no backup
	}
	return true, writeWithBackup(path, append(b, '\n'), orig)
}

// sameJSON: whether a and b hold the same JSON value, regardless of key order,
// spacing or number formatting (so a file Claude Code formatted differently still matches).
func sameJSON(a, b []byte) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

// isOurs: whether command ends with " <sub>" and points to agentwatch.
func isOurs(cmd, sub string) bool {
	ours := strings.Contains(cmd, "agentwatch") || strings.Contains(cmd, "ccwatch") // ccwatch = pre-rebrand name
	return ours && strings.HasSuffix(strings.TrimSpace(cmd), " "+sub)
}

// commandOf: the "command" field of one hook (empty if it isn't an object/string).
func commandOf(h any) string {
	hm, _ := h.(map[string]any)
	c, _ := hm["command"].(string)
	return c
}

// InstallHooks adds the agentwatch hooks to the settings.json at path without removing
// other hooks/settings. Existing agentwatch hooks only get their path updated.
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

	if !repointStatusline(s) && withStatusline {
		installStatusline(s)
	}

	changed, err := writeSettings(path, s, orig)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("agentwatch hooks already up to date in", path)
		return nil
	}
	fmt.Printf("agentwatch hooks installed in %s (%d new events, %d total)\n", path, added, len(hookEvents))
	fmt.Println("Start a new Claude Code session so the hooks take effect, then run `agentwatch` in a split next to it.")
	return nil
}

// refreshHookCommand updates existing agentwatch hook commands in groups
// (the binary path may have changed). Returns true if any were found.
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

// repointStatusline points an existing agentwatch (or pre-rebrand ccwatch) status line at this
// binary. Reports whether one was found.
func repointStatusline(s map[string]any) bool {
	sl, ok := s["statusLine"].(map[string]any)
	if !ok {
		return false
	}
	c, _ := sl["command"].(string)
	if !isOurs(c, "statusline") {
		return false
	}
	if sc, err := selfCommand("statusline"); err == nil {
		sl["command"] = sc
	}
	return true
}

func installStatusline(s map[string]any) {
	if _, exists := s["statusLine"]; exists {
		fmt.Println("A statusLine is already set; leaving it alone. Remove it first if you want the agentwatch one.")
		return
	}
	sc, _ := selfCommand("statusline")
	s["statusLine"] = map[string]any{"type": "command", "command": sc, "padding": 0}
	fmt.Println("agentwatch statusLine installed.")
}

// UninstallHooks removes the agentwatch hooks and status line from the settings.json at path.
func UninstallHooks(path string) error {
	s, orig, err := readSettings(path)
	if err != nil {
		return err
	}
	if orig == nil {
		fmt.Println("settings.json not found, nothing to remove.")
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
			fmt.Println("agentwatch statusLine removed.")
		}
	}
	if _, err := writeSettings(path, s, orig); err != nil {
		return err
	}
	fmt.Printf("removed %d agentwatch hooks from %s\n", removed, path)
	return nil
}

// withoutOurHooks drops agentwatch hooks from groups; groups left empty are dropped too.
// Returns the remaining groups and the number of hooks removed.
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
