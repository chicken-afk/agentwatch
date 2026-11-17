package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chicken-afk/agentwatch/internal/install"
)

// Prebuilt binaries are published to GitHub Releases by GoReleaser (.goreleaser.yaml)
// as agentwatch_<os>_<arch>.tar.gz plus checksums.txt.

// DefaultReleaseRepo: the GitHub owner/name that publishes releases.
const DefaultReleaseRepo = "chicken-afk/agentwatch"

// errNoRelease: the repository has no published release yet.
var errNoRelease = errors.New("no release published yet")

var httpClient = &http.Client{Timeout: 60 * time.Second}

// apiBase and downloadBase can be pointed at a test server.
var (
	apiBase      = "https://api.github.com"
	downloadBase = "https://github.com"
)

// targetPath: the binary to replace (the running one; tests point it elsewhere).
var targetPath = executable

// latestTag: the tag of the latest published release (e.g. "v0.2.0").
func latestTag(repo string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agentwatch-updater")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking the latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", errNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checking the latest release: GitHub answered %s", resp.Status)
	}
	var r struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.TagName == "" {
		return "", fmt.Errorf("checking the latest release: unexpected response")
	}
	return r.TagName, nil
}

// assetName: the archive for this machine.
func assetName() string { return fmt.Sprintf("agentwatch_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH) }

// sameVersion: "v0.2.0" and "0.2.0" are the same; "dev" never matches.
func sameVersion(current, tag string) bool {
	return current != "" && current != "dev" && strings.TrimPrefix(current, "v") == strings.TrimPrefix(tag, "v")
}

// runRelease updates from the latest GitHub release. Returns errNoRelease when there is none.
func (o Options) runRelease() error {
	tag, err := latestTag(o.ReleaseRepo)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "current: %s\nlatest:  %s\n", o.Version, tag)
	upToDate := sameVersion(o.Version, tag)
	if o.Check {
		if upToDate {
			fmt.Fprintln(o.Out, "agentwatch is up to date.")
		} else {
			fmt.Fprintln(o.Out, "An update is available: run `agentwatch update`.")
		}
		return nil
	}
	if upToDate && !o.Force {
		fmt.Fprintln(o.Out, "agentwatch is already up to date (use --force to reinstall).")
		return nil
	}

	base := fmt.Sprintf("%s/%s/releases/download/%s/", downloadBase, o.ReleaseRepo, tag)
	fmt.Fprintf(o.Out, "downloading %s …\n", assetName())
	archive, err := download(base + assetName())
	if err != nil {
		return fmt.Errorf("no %s in release %s (%w): build from source with `agentwatch update --source`", assetName(), tag, err)
	}
	sums, err := download(base + "checksums.txt")
	if err != nil {
		return fmt.Errorf("downloading checksums.txt: %w", err)
	}
	if err := verify(archive, sums, assetName()); err != nil {
		return err
	}
	bin, err := extractBinary(archive, "agentwatch")
	if err != nil {
		return err
	}
	if _, err := install.MigrateBinaryName(o.Out); err != nil {
		fmt.Fprintln(o.Out, "warning:", err)
	}
	exe, err := targetPath()
	if err != nil {
		return err
	}
	if err := replaceBinary(exe, bin); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "updated %s → %s\n", exe, tag)
	fmt.Fprintln(o.Out, "Restart any open agentwatch dashboards to use the new version.")
	return nil
}

func download(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "agentwatch-updater")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

// verify checks data against its line in a checksums.txt ("<sha256>  <name>").
func verify(data, sums []byte, name string) error {
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name {
			got := sha256.Sum256(data)
			if hex.EncodeToString(got[:]) != strings.ToLower(f[0]) {
				return fmt.Errorf("checksum mismatch for %s: the download is corrupt or was tampered with", name)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not listed in checksums.txt", name)
}

// extractBinary returns the file called name from a .tar.gz archive.
func extractBinary(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, fmt.Errorf("reading the archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s not found in the archive", name)
		}
		if err != nil {
			return nil, fmt.Errorf("reading the archive: %w", err)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == name {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
}

// replaceBinary writes bin next to exe and renames it over exe (atomic on the same filesystem).
func replaceBinary(exe string, bin []byte) error {
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", exe, err)
	}
	return nil
}
