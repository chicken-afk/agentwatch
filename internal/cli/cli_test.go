package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chicken-afk/agentwatch/internal/pathmatch"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		args     []string
		wantCmd  string
		wantRest []string
	}{
		{nil, "watch", nil},
		{[]string{"--all"}, "watch", []string{"--all"}},
		{[]string{""}, "watch", []string{""}},
		{[]string{"install", "--statusline"}, "install", []string{"--statusline"}},
		{[]string{"--version"}, "version", []string{}},
		{[]string{"-v"}, "version", []string{}},
	}
	for _, tt := range tests {
		cmd, rest := splitCommand(tt.args)
		if cmd != tt.wantCmd || !slices.Equal(rest, tt.wantRest) {
			t.Errorf("splitCommand(%q) = %q, %q; want %q, %q", tt.args, cmd, rest, tt.wantCmd, tt.wantRest)
		}
	}
}

func TestAdvisorFlag(t *testing.T) {
	var a advisorFlag
	for _, v := range []string{"opus", "7f3c=sonnet,9c1b=haiku", " "} {
		if err := a.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	if a.global != "opus" {
		t.Errorf("global = %q, want opus", a.global)
	}
	want := map[string]string{"7f3c": "sonnet", "9c1b": "haiku"}
	if !reflect.DeepEqual(a.bySession, want) {
		t.Errorf("bySession = %v, want %v", a.bySession, want)
	}
	if err := a.Set("=opus"); err == nil {
		t.Error("Set(\"=opus\") should fail")
	}
}

func TestWatchOptions(t *testing.T) {
	dir := t.TempDir()
	real := pathmatch.Real(dir)

	t.Run("dir flag becomes resolved filter", func(t *testing.T) {
		o := watchFlags{dir: dir}.options()
		if o.Dir != real {
			t.Errorf("Dir = %q, want %q", o.Dir, real)
		}
	})
	t.Run("default filters by current folder", func(t *testing.T) {
		wd, _ := os.Getwd()
		if got := (watchFlags{}).options().Dir; got != pathmatch.Real(wd) {
			t.Errorf("Dir = %q, want %q", got, pathmatch.Real(wd))
		}
	})
	t.Run("all disables folder filter", func(t *testing.T) {
		if got := (watchFlags{all: true, dir: dir}).options().Dir; got != "" {
			t.Errorf("Dir = %q, want empty", got)
		}
	})
	t.Run("explicit session skips folder filter unless dir given", func(t *testing.T) {
		if got := (watchFlags{session: "7f3c"}).options().Dir; got != "" {
			t.Errorf("Dir = %q, want empty", got)
		}
		if got := (watchFlags{session: "7f3c", dir: dir}).options().Dir; got != real {
			t.Errorf("Dir = %q, want %q", got, real)
		}
	})
	t.Run("global advisor applies to all sessions", func(t *testing.T) {
		f := watchFlags{all: true, advisors: advisorFlag{global: "opus"}}
		if o := f.options(); o.Advisor != "opus" || len(o.AdvisorBySession) != 0 {
			t.Errorf("got Advisor=%q BySession=%v", o.Advisor, o.AdvisorBySession)
		}
	})
	t.Run("global advisor with session is scoped to it, per-session label wins", func(t *testing.T) {
		f := watchFlags{session: "7f3c", advisors: advisorFlag{global: "opus"}}
		if o := f.options(); o.Advisor != "" || o.AdvisorBySession["7f3c"] != "opus" {
			t.Errorf("got Advisor=%q BySession=%v", o.Advisor, o.AdvisorBySession)
		}
		f.advisors.bySession = map[string]string{"7f3c": "sonnet"}
		if o := f.options(); o.AdvisorBySession["7f3c"] != "sonnet" {
			t.Errorf("explicit per-session label was overridden: %v", o.AdvisorBySession)
		}
	})
}

func TestWatchDirRelative(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if got := (watchFlags{dir: "a"}).watchDir(); got != pathmatch.Real(sub) {
		t.Errorf("watchDir = %q, want %q", got, pathmatch.Real(sub))
	}
}
