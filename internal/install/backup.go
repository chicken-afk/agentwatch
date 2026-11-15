// Package install installs/removes the ccwatch integration in the Claude Code config:
// hooks (+ status line) in settings.json and the skill rules block in CLAUDE.md.
package install

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeWithBackup writes data to path. If the file already exists (orig != nil),
// the old copy is saved first as <path>.ccwatch-backup-<time>.
func writeWithBackup(path string, data, orig []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := backup(path, orig); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// backup saves orig as a backup file; does nothing if orig == nil.
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
