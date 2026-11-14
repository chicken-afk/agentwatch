package install

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Blok aturan skill yang dikelola ccwatch di ~/.claude/CLAUDE.md (memori level user,
// dimuat Claude Code di SEMUA proyek). Ditandai marker supaya bisa diperbarui/dilepas
// tanpa menyentuh isi file lainnya.
const (
	rulesStart = "<!-- ccwatch:skills:start -->"
	rulesEnd   = "<!-- ccwatch:skills:end -->"
)

const rulesBody = `## Skills (managed by ccwatch)
- Before starting a task, load every relevant skill by calling the **Skill** tool. Mentioning a skill by name does not load it.
- When you report which skills you used, list only the skills you invoked with the Skill tool in the current task. Skills invoked in earlier tasks are "still in context", not "used".
- If no skill is relevant, say so instead of naming one.`

// RulesBlock: blok aturan lengkap beserta marker.
func RulesBlock() string { return rulesStart + "\n" + rulesBody + "\n" + rulesEnd }

// upsertRules: pasang/perbarui blok ccwatch. Mengembalikan isi baru dan apakah berubah.
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

// removeRules: lepas blok ccwatch (beserta baris kosong di sekitarnya).
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

// findBlock: rentang [i, j) blok ccwatch di content, termasuk kedua marker.
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

// readMemory: isi CLAUDE.md beserta byte aslinya (nil kalau file belum ada).
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

// InstallRules memasang/memperbarui blok aturan skill di CLAUDE.md pada path.
func InstallRules(path string) error {
	content, orig, err := readMemory(path)
	if err != nil {
		return err
	}
	out, changed := upsertRules(content)
	if !changed {
		fmt.Println("aturan skill ccwatch sudah terpasang di", path)
		return nil
	}
	if err := writeWithBackup(path, []byte(out), orig); err != nil {
		return err
	}
	fmt.Println("aturan skill ccwatch dipasang di", path, "(berlaku di semua proyek)")
	return nil
}

// UninstallRules melepas blok aturan skill. Kalau file jadi kosong, file dihapus (backup tetap dibuat).
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
		// file dibuat oleh ccwatch dan isinya hanya blok ini → hapus saja
		if err := backup(path, orig); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Println("aturan skill ccwatch dilepas;", path, "dihapus karena kosong")
		return nil
	}
	if err := writeWithBackup(path, []byte(out), orig); err != nil {
		return err
	}
	fmt.Println("aturan skill ccwatch dilepas dari", path)
	return nil
}
