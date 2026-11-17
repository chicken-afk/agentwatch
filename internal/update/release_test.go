package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
	}{{"README.md", []byte("readme")}, {name, body}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(f.body)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// fakeGitHub serves releases/latest and the release assets; sumOverride replaces the archive's checksum.
func fakeGitHub(t *testing.T, tag string, archive []byte, sumOverride string) {
	t.Helper()
	sum := sha256.Sum256(archive)
	hexSum := hex.EncodeToString(sum[:])
	if sumOverride != "" {
		hexSum = sumOverride
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if tag == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	})
	mux.HandleFunc("/o/r/releases/download/"+tag+"/"+assetName(), func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/o/r/releases/download/"+tag+"/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\nffff  other.tar.gz\n", hexSum, assetName())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldAPI, oldDL, oldTarget := apiBase, downloadBase, targetPath
	apiBase, downloadBase = srv.URL, srv.URL
	t.Cleanup(func() { apiBase, downloadBase, targetPath = oldAPI, oldDL, oldTarget })
}

func fakeExe(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "agentwatch")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath = func() (string, error) { return exe, nil }
	return exe
}

func TestUpdateFromRelease(t *testing.T) {
	fakeGitHub(t, "v0.2.0", tarGz(t, "agentwatch", []byte("new binary")), "")
	exe := fakeExe(t)
	var out bytes.Buffer
	if err := Run(Options{Version: "0.1.0", ReleaseRepo: "o/r", Out: &out}); err != nil {
		t.Fatalf("update: %v\n%s", err, out.String())
	}
	if got, _ := os.ReadFile(exe); string(got) != "new binary" {
		t.Errorf("binary not replaced: %q", got)
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm()&0o100 == 0 {
		t.Error("new binary must be executable")
	}
}

func TestUpdateUpToDateAndCheck(t *testing.T) {
	fakeGitHub(t, "v0.2.0", tarGz(t, "agentwatch", []byte("new")), "")
	exe := fakeExe(t)
	var out bytes.Buffer
	if err := Run(Options{Version: "0.2.0", ReleaseRepo: "o/r", Out: &out}); err != nil || !strings.Contains(out.String(), "already up to date") {
		t.Fatalf("same version should be a no-op: %v %s", err, out.String())
	}
	out.Reset()
	if err := Run(Options{Version: "dev", ReleaseRepo: "o/r", Check: true, Out: &out}); err != nil || !strings.Contains(out.String(), "update is available") {
		t.Fatalf("--check from a dev build should report an update: %v %s", err, out.String())
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Error("no-op and --check must not touch the binary")
	}
}

func TestUpdateRejectsBadChecksum(t *testing.T) {
	fakeGitHub(t, "v0.2.0", tarGz(t, "agentwatch", []byte("evil")), strings.Repeat("0", 64))
	exe := fakeExe(t)
	err := Run(Options{Version: "0.1.0", ReleaseRepo: "o/r", Out: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum error, got %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Error("a bad download must not replace the binary")
	}
}

func TestSameVersion(t *testing.T) {
	for _, c := range []struct {
		cur, tag string
		want     bool
	}{{"0.2.0", "v0.2.0", true}, {"v0.2.0", "v0.2.0", true}, {"0.1.0", "v0.2.0", false}, {"dev", "vdev", false}, {"", "v0.1.0", false}} {
		if got := sameVersion(c.cur, c.tag); got != c.want {
			t.Errorf("sameVersion(%q, %q) = %v", c.cur, c.tag, got)
		}
	}
}
