package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chicken-afk/agentwatch/internal/store"
)

func TestSplashFrames(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 14}, {40, 8}} {
		w, h := size[0], size[1]
		for _, at := range []time.Duration{0, 300 * time.Millisecond, scanEnd, creditStart, splashTotal} {
			f := SplashFrame(w, h, at)
			if got := lipgloss.Height(f); got != h {
				t.Errorf("%dx%d @%v: height %d", w, h, at, got)
			}
			if got := lipgloss.Width(f); got > w {
				t.Errorf("%dx%d @%v: width %d overflows", w, h, at, got)
			}
		}
		end := ansi.Strip(SplashFrame(w, h, splashTotal))
		if !strings.Contains(end, splashCredit) {
			t.Errorf("%dx%d: final frame lacks the credit", w, h)
		}
		if w >= 80 {
			for _, a := range splashAgents {
				if !strings.Contains(end, a.name) {
					t.Errorf("%dx%d: final frame lacks %s", w, h, a.name)
				}
			}
		}
	}
	if s := ansi.Strip(SplashFrame(120, 40, 0)); strings.Contains(s, splashCredit) || strings.Contains(s, "claude") {
		t.Error("frame 0 should show neither the roll call nor the credit yet")
	}
}

func TestSplashSkipAndEnd(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("AGENTWATCH_DIR", t.TempDir())
	m := &model{st: store.New(store.Options{}), splash: true, splashStart: time.Now()}
	if !strings.Contains(ansi.Strip(m.View()), "·") {
		t.Error("splash should render while playing")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.splash {
		t.Error("a key press should skip the splash")
	}

	m = &model{st: store.New(store.Options{}), splash: true, splashStart: time.Now().Add(-splashTotal)}
	m.Update(tickMsg(time.Now()))
	if m.splash {
		t.Error("splash should end after splashTotal")
	}
}
