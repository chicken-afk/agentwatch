package install

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Skill rules block managed by ccwatch in ~/.claude/CLAUDE.md (user-level memory,
// loaded by Claude Code in EVERY project). Wrapped in markers so it can be updated/removed
// without touching the rest of the file.
const (
	rulesStart = "<!-- ccwatch:skills:start -->"
	rulesEnd   = "<!-- ccwatch:skills:end -->"
)

const rulesBody = `## Skills (managed by ccwatch)
- Before starting a task, load every relevant skill by calling the **Skill** tool. Mentioning a skill by name does not load it.
- When you report which skills you used, list only the skills you invoked with the Skill tool in the current task. Skills invoked in earlier tasks are "still in context", not "used".
- If no skill is relevant, say so instead of naming one.`

// RulesBlock: the full rules block including markers.
func RulesBlock() string { return rulesStart + "\n" + rulesBody + "\n" + rulesEnd }

// upsertRules: install/update the ccwatch block. Returns the new content and whether it changed.
func upsertRules(content string) (string, bool) {
	block := RulesBlock()
	if i, j, ok := findBlock(content); ok {
		out := content[:i] + block + content[j:]
		return out, out != content
	}
	out := strings.TrimRight(content, "\n")
	if out != "" {
		out += "\n\n"
	}
	return out + block + "\n", true
}

// removeRules: remove the ccwatch block (along with the surrounding blank lines).
func removeRules(content string) (string, bool) {
	i, j, ok := findBlock(content)
	if !ok {
		return content, false
	}
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

// findBlock: the range [i, j) of the ccwatch block in content, including both markers.
func findBlock(content string) (i, j int, ok bool) {
	i = strings.Index(content, rulesStart)
	if i < 0 {
		return 0, 0, false
	}
	end := strings.Index(content[i:], rulesEnd)
	if end < 0 {
		return 0, 0, false
	}
	return i, i + end + len(rulesEnd), true
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
		fmt.Println("ccwatch skill rules already installed in", path)
		return nil
	}
	if err := writeWithBackup(path, []byte(out), orig); err != nil {
		return err
	}
	fmt.Println("ccwatch skill rules installed in", path, "(applies to all projects)")
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
		// file was created by ccwatch and contains only this block → just delete it
		if err := backup(path, orig); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Println("ccwatch skill rules removed;", path, "deleted because it was empty")
		return nil
	}
	if err := writeWithBackup(path, []byte(out), orig); err != nil {
		return err
	}
	fmt.Println("ccwatch skill rules removed from", path)
	return nil
}
