// Package update implements `agentwatch update`: fetch the latest source from GitHub,
// build it, and replace the running binary in place.
//
// There are no prebuilt releases, so this needs git and Go on PATH. The source lives in
// a clone owned by agentwatch (os.UserCacheDir()/agentwatch/src, or AGENTWATCH_SRC), which is
// reset to the remote branch on every update; --src builds from your own checkout instead
// and only fast-forwards it.
package update

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/chicken-afk/agentwatch/internal/config"
	"github.com/chicken-afk/agentwatch/internal/install"
)

// DefaultRepo is where agentwatch updates from.
const DefaultRepo = "https://github.com/chicken-afk/agentwatch.git"

// Options for one update run.
type Options struct {
	Repo   string // git URL (DefaultRepo if empty)
	Branch string // branch to build ("main" if empty)
	Src    string // your own checkout to build from; "" = the managed clone
	Check  bool   // only report whether an update is available
	Force  bool   // rebuild even if already up to date
	Out    io.Writer
}

// Revision: the git commit this binary was built from ("" if unknown) and whether
// the build had uncommitted changes.
func Revision() (rev string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return rev, modified
}

// SourceDir: the managed clone (AGENTWATCH_SRC, or the old CCWATCH_SRC, overrides).
func SourceDir() (string, error) {
	if d := config.Env("SRC"); d != "" {
		return d, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "agentwatch", "src"), nil
}

// Run performs the update.
func Run(o Options) error {
	if o.Repo == "" {
		o.Repo = DefaultRepo
	}
	if o.Branch == "" {
		o.Branch = "main"
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git is not installed (needed to download the source)")
	}

	if o.Src != "" {
		return o.buildLocal()
	}

	current, modified := Revision()
	latest, err := remoteHead(o.Repo, o.Branch)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "current: %s\nlatest:  %s (%s)\n", describe(current, modified), short(latest), o.Branch)

	upToDate := current != "" && current == latest && !modified
	if o.Check {
		if upToDate {
			fmt.Fprintln(o.Out, "agentwatch is up to date.")
		} else {
			fmt.Fprintln(o.Out, "An update is available: run `agentwatch update`.")
		}
		return nil
	}
	if upToDate && !o.Force {
		fmt.Fprintln(o.Out, "agentwatch is already up to date (use --force to rebuild anyway).")
		return nil
	}
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("Go is not installed (needed to build agentwatch): https://go.dev/dl/")
	}

	dir, err := o.prepareSource()
	if err != nil {
		return err
	}
	if _, err := install.MigrateBinaryName(o.Out); err != nil {
		fmt.Fprintln(o.Out, "warning:", err)
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	if err := build(dir, exe, o.Out); err != nil {
		return err
	}
	rev, _ := gitOut(dir, "rev-parse", "HEAD")
	fmt.Fprintf(o.Out, "updated %s → %s\n", exe, short(rev))
	fmt.Fprintln(o.Out, "Restart any open agentwatch dashboards to use the new version.")
	fmt.Fprintln(o.Out, "If you upgraded from ccwatch, run `agentwatch install` once so the hooks use the new name.")
	return nil
}

// buildLocal: --src builds your own checkout as it is, after fast-forwarding it
// when its branch tracks a remote (--check, --branch and --repo don't apply).
func (o Options) buildLocal() error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("Go is not installed (needed to build agentwatch): https://go.dev/dl/")
	}
	if _, err := gitOut(o.Src, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		fmt.Fprintf(o.Out, "pulling %s …\n", o.Src)
		// a failed --ff-only pull leaves the checkout untouched, so building it as it is stays safe
		if err := git(o.Src, o.Out, "pull", "--ff-only"); err != nil {
			fmt.Fprintf(o.Out, "warning: git pull failed (%v): building %s as it is\n", err, o.Src)
		}
	} else {
		fmt.Fprintf(o.Out, "%s has no upstream branch: building it as it is\n", o.Src)
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	if err := build(o.Src, exe, o.Out); err != nil {
		return err
	}
	rev, _ := gitOut(o.Src, "rev-parse", "HEAD")
	dirty, _ := gitOut(o.Src, "status", "--porcelain")
	fmt.Fprintf(o.Out, "updated %s → %s\n", exe, describe(rev, dirty != ""))
	return nil
}

// prepareSource clones or updates agentwatch's own clone and returns its folder.
func (o Options) prepareSource() (string, error) {
	dir, err := SourceDir()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		fmt.Fprintf(o.Out, "cloning %s into %s …\n", o.Repo, dir)
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", err
		}
		if err := git("", o.Out, "clone", "--quiet", "--branch", o.Branch, o.Repo, dir); err != nil {
			return "", fmt.Errorf("git clone failed: %w", err)
		}
		return dir, nil
	}
	// the managed clone belongs to agentwatch: always match the remote branch exactly
	fmt.Fprintf(o.Out, "fetching %s …\n", o.Branch)
	if err := git(dir, o.Out, "remote", "set-url", "origin", o.Repo); err != nil {
		return "", err
	}
	if err := git(dir, o.Out, "fetch", "--quiet", "origin", o.Branch); err != nil {
		return "", fmt.Errorf("git fetch failed: %w", err)
	}
	if err := git(dir, o.Out, "checkout", "--quiet", "-B", o.Branch, "FETCH_HEAD"); err != nil {
		return "", err
	}
	return dir, git(dir, o.Out, "reset", "--quiet", "--hard", "FETCH_HEAD")
}

// build compiles dir into a temp file next to exe, then swaps it in with a rename
// (atomic on the same filesystem; a running dashboard keeps its old copy).
func build(dir, exe string, out io.Writer) error {
	fmt.Fprintln(out, "building …")
	tmp := exe + ".new"
	cmd := exec.Command("go", "build", "-o", tmp, ".")
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("go build failed: %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replacing %s failed: %w", exe, err)
	}
	return nil
}

// executable: the path of the running agentwatch binary, symlinks resolved.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return exe, nil
}

// remoteHead: the latest commit of branch on the remote.
func remoteHead(repo, branch string) (string, error) {
	out, err := gitOut("", "ls-remote", repo, "refs/heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("cannot reach %s: %w", repo, err)
	}
	rev, _, _ := strings.Cut(out, "\t")
	if rev == "" {
		return "", fmt.Errorf("branch %q not found in %s", branch, repo)
	}
	return rev, nil
}

func git(dir string, out io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

func short(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	if rev == "" {
		return "unknown"
	}
	return rev
}

// describe: "abc1234", "abc1234 (local changes)" or "unknown (built without git info)".
func describe(rev string, modified bool) string {
	switch {
	case rev == "":
		return "unknown (built without git info)"
	case modified:
		return short(rev) + " + local changes"
	}
	return short(rev)
}
