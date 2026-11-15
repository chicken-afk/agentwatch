package pathmatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInDir(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	sub := filepath.Join(proj, "frontend")
	other := filepath.Join(root, "other")
	for _, d := range []string{sub, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(proj, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, cwd, dir string
		want           bool
	}{
		{"same folder", proj, proj, true},
		{"subfolder", sub, proj, true},
		{"trailing slash on dir", sub, proj + "/", true},
		{"sibling folder", other, proj, false},
		{"parent is not inside child", proj, sub, false},
		{"symlinked dir", sub, link, true},
		{"symlinked cwd", filepath.Join(link, "frontend"), proj, true},
		{"empty cwd", "", proj, false},
		{"empty dir", proj, "", false},
		{"missing folders compare by text", "/nonexistent/x/y", "/nonexistent/x", true},
		{"missing folders, no prefix match", "/nonexistent/xy", "/nonexistent/x", false},
	}
	for _, tt := range tests {
		if got := InDir(tt.cwd, tt.dir); got != tt.want {
			t.Errorf("%s: InDir(%q, %q) = %v, want %v", tt.name, tt.cwd, tt.dir, got, tt.want)
		}
	}

	// case differences (macOS) match textually
	if got := InDir("/NONEXISTENT/X/y", "/nonexistent/x"); !got {
		t.Error("case-insensitive text match expected for missing folders")
	}
	if strings.EqualFold(Real(link), Real(proj)) != true {
		t.Errorf("Real should resolve symlinks: %q vs %q", Real(link), Real(proj))
	}
}
