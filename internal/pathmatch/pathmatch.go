// Package pathmatch: perbandingan folder untuk memfilter sesi berdasarkan cwd.
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

// InDir: apakah cwd ada di dalam dir (sama, atau subfolder).
// Dibandingkan sebagai folder di disk (os.SameFile), jadi beda huruf besar/kecil
// (macOS) dan symlink tetap cocok. Hasil di-cache per pasangan cwd/dir.
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
			// cocok secara teks; pastikan beneran folder yang sama kalau keduanya ada
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
	unknown ancestry = iota - 1 // folder tidak ada, tidak bisa dicek
	no
	yes
)

// ancestorState: yes kalau dir adalah cwd atau salah satu parent-nya.
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

// Real: path bersih dengan symlink di-resolve (mis. /tmp → /private/tmp di macOS).
func Real(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(r)
	}
	return filepath.Clean(p)
}
