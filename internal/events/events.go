// Package events writes the ccwatch event log (events.jsonl) that the dashboard reads.
package events

import (
	"os"

	"ccwatch/internal/config"
)

// maxSize: above this size the file is rotated to events.jsonl.1.
const maxSize = 20 << 20

// Append adds one JSON record (without newline) to events.jsonl.
// The record is written in a single write so concurrent appends are safe.
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
