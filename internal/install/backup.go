// Package install memasang/melepas integrasi ccwatch di konfigurasi Claude Code:
// hooks (+ status line) di settings.json dan blok aturan skill di CLAUDE.md.
package install

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeWithBackup menulis data ke path. Kalau file sudah ada (orig != nil),
// salinan lama disimpan dulu sebagai <path>.ccwatch-backup-<waktu>.
func writeWithBackup(path string, data, orig []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := backup(path, orig); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// backup menyimpan orig sebagai file cadangan; tidak melakukan apa pun kalau orig == nil.
func backup(path string, orig []byte) error {
	if orig == nil {
		return nil
	}
	bak := path + ".ccwatch-backup-" + time.Now().Format("20060102-150405")
	if err := os.WriteFile(bak, orig, 0o644); err != nil {
		return err
	}
	fmt.Println("backup:", bak)
	return nil
}
