package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// legacyBinary: the binary name before the rebrand.
const legacyBinary = "ccwatch"

// MigrateBinaryName: if this binary still runs under the old name (…/ccwatch), move it to
// …/agentwatch and leave ccwatch as a symlink to it, so old hooks, scripts and habits keep
// working. Returns the path to use from now on. Does nothing for any other name.
func MigrateBinaryName(out io.Writer) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if fi, err := os.Lstat(exe); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		// already a symlink (migrated before): resolve it
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			return r, nil
		}
		return exe, nil
	}
	if filepath.Base(exe) != legacyBinary {
		return exe, nil
	}
	target := filepath.Join(filepath.Dir(exe), "agentwatch")
	if _, err := os.Stat(target); err == nil {
		// an agentwatch binary already sits next to it: keep that one
		if err := os.Remove(exe); err != nil {
			return exe, err
		}
	} else if err := os.Rename(exe, target); err != nil {
		return exe, fmt.Errorf("renaming %s to %s: %w", exe, target, err)
	}
	if err := os.Symlink(target, exe); err != nil {
		return target, fmt.Errorf("linking %s → %s: %w", exe, target, err)
	}
	fmt.Fprintf(out, "renamed %s → %s (ccwatch is now a symlink to it)\n", exe, target)
	return target, nil
}
