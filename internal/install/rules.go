package install

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Skill rules block managed by agentwatch in ~/.claude/CLAUDE.md (user-level memory,
// loaded by Claude Code in EVERY project). Wrapped in markers so it can be updated/removed
// without touching the rest of the file.
const (
	rulesStart = "<!-- agentwatch:skills:start -->"
	rulesEnd   = "<!-- agentwatch:skills:end -->"

	// markers written before the rebrand: replaced on install, removed on uninstall
	legacyStart = "<!-- ccwatch:skills:start -->"
	legacyEnd   = "<!-- ccwatch:skills:end -->"
)

const rulesBody = `## Skills (managed by agentwatch)
- Before starting a task, load every relevant skill by calling the **Skill** tool. Mentioning a skill by name does not load it.
- When you report which skills you used, list only the skills you invoked with the Skill tool in the current task. Skills invoked in earlier tasks are "still in context", not "used".
- If no skill is relevant, say so instead of naming one.`

// RulesBlock: the full rules block including markers.
func RulesBlock() string { return rulesStart + "\n" + rulesBody + "\n" + rulesEnd }

// upsertRules: install/update the agentwatch block. Returns the new content and whether it changed.
func upsertRules(content string) (string, bool) {
	block := RulesBlock()
	if i, j, ok := findBlock(content); ok {
		out := content[:i] + block + content[j:]
		if li, lj, ok := findMarked(out, legacyStart, legacyEnd); ok {
			out, _ = cutBlock(out, li, lj)
		}
		return out, out != content
	}
	if i, j, ok := findMarked(content, legacyStart, legacyEnd); ok {
		out := content[:i] + block + content[j:] // the old ccwatch block becomes the agentwatch block
		return out, true
	}
	out := strings.TrimRight(content, "\n")
	if out != "" {
		out += "\n\n"
	}
	return out + block + "\n", true
}

// removeRules: remove the agentwatch block (along with the surrounding blank lines).
func removeRules(content string) (string, bool) {
	changed := false
	for _, m := range [][2]string{{rulesStart, rulesEnd}, {legacyStart, legacyEnd}} {
		if i, j, ok := findMarked(content, m[0], m[1]); ok {
			content, _ = cutBlock(content, i, j)
			changed = true
		}
	}
	return content, changed
}

// cutBlock removes content[i:j] along with the surrounding blank lines.
func cutBlock(content string, i, j int) (string, bool) {
	before := strings.TrimRight(content[:i], "\n")
	after := strings.TrimLeft(content[j:], "\n")
	out := before
	if before != "" && after != "" {
		out += "\n\n"
	}
	out += after
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, true
}

// findBlock: the range [i, j) of the agentwatch block in content, including both markers.
func findBlock(content string) (i, j int, ok bool) { return findMarked(content, rulesStart, rulesEnd) }

// findMarked: the range [i, j) of the block between start and end markers (inclusive).
func findMarked(content, start, end string) (i, j int, ok bool) {
	i = strings.Index(content, start)
	if i < 0 {
		return 0, 0, false
	}
	e := strings.Index(content[i:], end)
	if e < 0 {
		return 0, 0, false
	}
	return i, i + e + len(end), true
}

// readMemory: CLAUDE.md content plus its original bytes (nil if the file doesn't exist yet).
func readMemory(path string) (string, []byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return string(b), b, nil
}

// InstallRules installs/updates the skill rules block in the CLAUDE.md at path.
func InstallRules(path string) error {
	content, orig, err := readMemory(path)
	if err != nil {
		return err
	}
	out, changed := upsertRules(content)
	if !changed {
		fmt.Println("agentwatch skill rules already installed in", path)
		return nil
	}
	if err := writeWithBackup(path, []byte(out), orig); err != nil {
		return err
	}
	fmt.Println("agentwatch skill rules installed in", path, "(applies to all projects)")
	return nil
}

// UninstallRules removes the skill rules block. If the file ends up empty, it's deleted (a backup is still made).
func UninstallRules(path string) error {
	content, orig, err := readMemory(path)
	if err != nil {
		return err
	}
	out, changed := removeRules(content)
	if !changed {
		return nil
	}
	if strings.TrimSpace(out) == "" {
		// file was created by agentwatch and contains only this block → just delete it
		if err := backup(path, orig); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Println("agentwatch skill rules removed;", path, "deleted because it was empty")
		return nil
	}
	if err := writeWithBackup(path, []byte(out), orig); err != nil {
		return err
	}
	fmt.Println("agentwatch skill rules removed from", path)
	return nil
}
