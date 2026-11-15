package store

import (
	"bufio"
	"bytes"
	"io"
	"os"
)

// tailer reads a JSONL file incrementally: each read() only processes
// complete lines appended since the previous call.
type tailer struct {
	path    string
	off     int64
	partial []byte
}

func (t *tailer) read(fn func([]byte)) {
	f, err := os.Open(t.path)
	if err != nil {
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() < t.off { // file rotated / truncated
		t.off, t.partial = 0, nil
	}
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return
	}
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		chunk, err := r.ReadBytes('\n')
		t.off += int64(len(chunk))
		if len(chunk) > 0 && chunk[len(chunk)-1] == '\n' {
			line := append(t.partial, chunk[:len(chunk)-1]...)
			t.partial = nil
			if len(bytes.TrimSpace(line)) > 0 {
				fn(line)
			}
		} else if len(chunk) > 0 {
			t.partial = append(t.partial, chunk...)
		}
		if err != nil {
			return
		}
	}
}
