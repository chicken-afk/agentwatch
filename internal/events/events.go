// Package events menulis log event ccwatch (events.jsonl) yang dibaca dashboard.
package events

import (
	"os"

	"ccwatch/internal/config"
)

// maxSize: di atas ukuran ini file dirotasi ke events.jsonl.1.
const maxSize = 20 << 20

// Append menambahkan satu record JSON (tanpa newline) ke events.jsonl.
// Record ditulis dengan satu write supaya aman untuk append bersamaan.
func Append(record []byte) error {
	dir := config.DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := config.EventsPath()

	if st, err := os.Stat(p); err == nil && st.Size() > maxSize {
		_ = os.Rename(p, p+".1")
	}

	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line := make([]byte, 0, len(record)+1)
	line = append(append(line, record...), '\n')
	_, err = f.Write(line)
	return err
}
