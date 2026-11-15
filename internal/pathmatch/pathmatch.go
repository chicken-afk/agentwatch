// Package pathmatch: folder comparison for filtering sessions by cwd.
package pathmatch

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	cacheMu sync.Mutex
	cache   = map[string]bool{}
)

// InDir: whether cwd is inside dir (the same folder, or a subfolder).
// Compared as folders on disk (os.SameFile), so case differences
// (macOS) and symlinks still match. Results are cached per cwd/dir pair.
func InDir(cwd, dir string) bool {
	if cwd == "" || dir == "" {
		return false
	}
	key := cwd + "\x00" + dir
	cacheMu.Lock()
	v, ok := cache[key]
	cacheMu.Unlock()
	if ok {
		return v
	}
	v = inDir(cwd, dir)
	cacheMu.Lock()
	cache[key] = v
	cacheMu.Unlock()
	return v
}

func inDir(cwd, dir string) bool {
	d := strings.TrimSuffix(dir, "/")
	for _, c := range []string{filepath.Clean(cwd), Real(cwd)} {
		if strings.EqualFold(c, d) || strings.HasPrefix(strings.ToLower(c), strings.ToLower(d)+"/") {
			// textual match; make sure it's really the same folder if both exist
			if r := ancestorState(c, d); r != unknown {
				return r == yes
			}
			return true
		}
	}
	return ancestorState(cwd, d) == yes
}

type ancestry int

const (
	unknown ancestry = iota - 1 // folder doesn't exist, can't be checked
	no
	yes
)

// ancestorState: yes if dir is cwd or one of its parents.
func ancestorState(cwd, dir string) ancestry {
	di, err := os.Stat(dir)
	if err != nil {
		return unknown
	}
	p := filepath.Clean(cwd)
	checked := false
	for {
		if pi, err := os.Stat(p); err == nil {
			checked = true
			if os.SameFile(pi, di) {
				return yes
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	if !checked {
		return unknown
	}
	return no
}

// Real: a clean path with symlinks resolved (e.g. /tmp → /private/tmp on macOS).
func Real(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(r)
	}
	return filepath.Clean(p)
}
