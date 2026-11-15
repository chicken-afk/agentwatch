package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ccwatch/internal/pathmatch"
	"ccwatch/internal/store"
	"ccwatch/internal/ui"
)

// watchFlags: parsed flags for `ccwatch watch`.
type watchFlags struct {
	session  string
	advisors advisorFlag
	recent   time.Duration
	all      bool
	dir      string
}

func runWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	var f watchFlags
	fs.StringVar(&f.session, "session", "", "pin to a specific session id (prefix allowed)")
	fs.Var(&f.advisors, "advisor", "advisor model label when it cannot be read from settings: `opus` (all sessions, or only --session if set) or `<session-id>=opus` (per session, prefix allowed, repeatable)")
	fs.DurationVar(&f.recent, "recent", 0, "also show transcripts active within this duration, even without hooks (e.g. 30m)")
	fs.BoolVar(&f.all, "all", false, "show sessions from all folders (default: only sessions in the current folder)")
	fs.StringVar(&f.dir, "dir", "", "only show sessions in this folder (default: current folder)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return ui.Run(f.options())
}

// options converts the flags into store.Options.
func (f watchFlags) options() store.Options {
	o := store.Options{Session: f.session, Recent: f.recent, AdvisorBySession: f.advisors.bySession}

	// default: only sessions in the folder where ccwatch was started.
	// --all disables the filter; --session (an explicit session) isn't folder-filtered either.
	if !f.all && (f.session == "" || f.dir != "") {
		o.Dir = f.watchDir()
	}

	if g := f.advisors.global; g != "" {
		if f.session == "" {
			o.Advisor = g
			return o
		}
		// --session + --advisor opus → label only for that session
		if o.AdvisorBySession == nil {
			o.AdvisorBySession = map[string]string{}
		}
		if _, ok := o.AdvisorBySession[f.session]; !ok {
			o.AdvisorBySession[f.session] = g
		}
	}
	return o
}

// watchDir: the session filter folder (--dir or the current folder), as an absolute path with symlinks resolved.
func (f watchFlags) watchDir() string {
	d := f.dir
	if d == "" {
		d, _ = os.Getwd()
	}
	if d == "" {
		return ""
	}
	if abs, err := filepath.Abs(d); err == nil {
		d = abs
	}
	return pathmatch.Real(d)
}

// advisorFlag: --advisor can be repeated; "opus" = global, "<id>=opus" = per session.
type advisorFlag struct {
	global    string
	bySession map[string]string
}

func (a *advisorFlag) String() string { return a.global }

func (a *advisorFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, mdl, ok := strings.Cut(part, "=")
		if !ok {
			a.global = part
			continue
		}
		id, mdl = strings.TrimSpace(id), strings.TrimSpace(mdl)
		if id == "" || mdl == "" {
			return fmt.Errorf("invalid --advisor format: %q (use <session-id>=<model>)", part)
		}
		if a.bySession == nil {
			a.bySession = map[string]string{}
		}
		a.bySession[id] = mdl
	}
	return nil
}
