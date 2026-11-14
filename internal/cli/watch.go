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

// watchFlags: hasil parsing flag `ccwatch watch`.
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
	fs.StringVar(&f.session, "session", "", "kunci ke session id tertentu (prefix boleh)")
	fs.Var(&f.advisors, "advisor", "label model advisor kalau tidak terbaca dari settings: `opus` (semua sesi, atau hanya --session kalau di-set) atau `<session-id>=opus` (per sesi, prefix boleh, bisa diulang)")
	fs.DurationVar(&f.recent, "recent", 0, "ikut tampilkan transcript yang aktif dalam durasi ini walau tanpa hooks (mis. 30m)")
	fs.BoolVar(&f.all, "all", false, "tampilkan sesi dari semua folder (default: hanya sesi di folder saat ini)")
	fs.StringVar(&f.dir, "dir", "", "hanya tampilkan sesi di folder ini (default: folder saat ini)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return ui.Run(f.options())
}

// options menerjemahkan flag ke store.Options.
func (f watchFlags) options() store.Options {
	o := store.Options{Session: f.session, Recent: f.recent, AdvisorBySession: f.advisors.bySession}

	// default: hanya sesi di folder tempat ccwatch dijalankan.
	// --all mematikan filter; --session (sesi eksplisit) juga tidak difilter folder.
	if !f.all && (f.session == "" || f.dir != "") {
		o.Dir = f.watchDir()
	}

	if g := f.advisors.global; g != "" {
		if f.session == "" {
			o.Advisor = g
			return o
		}
		// --session + --advisor opus → label hanya untuk sesi itu
		if o.AdvisorBySession == nil {
			o.AdvisorBySession = map[string]string{}
		}
		if _, ok := o.AdvisorBySession[f.session]; !ok {
			o.AdvisorBySession[f.session] = g
		}
	}
	return o
}

// watchDir: folder filter sesi (--dir atau folder saat ini), path absolut dengan symlink di-resolve.
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

// advisorFlag: --advisor bisa diulang; "opus" = global, "<id>=opus" = per sesi.
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
			return fmt.Errorf("format --advisor salah: %q (pakai <session-id>=<model>)", part)
		}
		if a.bySession == nil {
			a.bySession = map[string]string{}
		}
		a.bySession[id] = mdl
	}
	return nil
}
