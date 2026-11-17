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
	for _, size := range [][2]int{{120, 40}, {80, 14}, {80, 12}, {40, 8}} {
		w, h := size[0], size[1]
		for _, at := range []time.Duration{0, 300 * time.Millisecond, scanEnd, creditStart, splashTotal, splashTotal + 3*time.Second} {
			f := SplashFrame(w, h, at)
			if got := lipgloss.Height(f); got != h {
				t.Errorf("%dx%d @%v: height %d", w, h, at, got)
			}
			if got := lipgloss.Width(f); got > w {
				t.Errorf("%dx%d @%v: width %d overflows", w, h, at, got)
			}
		}
		end := ansi.Strip(SplashFrame(w, h, splashTotal+time.Second))
		if !strings.Contains(end, splashCredit) || !strings.Contains(end, enterPrompt) {
			t.Errorf("%dx%d: final frame lacks the credit or the Enter prompt", w, h)
		}
		if w >= 80 && h >= 13 { // full layout (smaller sizes get the compact one)
			for _, a := range splashAgents {
				if !strings.Contains(end, a.name) {
					t.Errorf("%dx%d: final frame lacks %s", w, h, a.name)
				}
			}
		}
	}
	if s := ansi.Strip(SplashFrame(120, 40, 0)); strings.Contains(s, splashCredit) || strings.Contains(s, "claude") || strings.Contains(s, enterPrompt) {
		t.Error("frame 0 should show neither the roll call, the credit nor the prompt yet")
	}
	if s := ansi.Strip(SplashFrame(120, 40, creditStart)); strings.Contains(s, enterPrompt) {
		t.Error("the prompt should only appear once the animation is done")
	}
}

func TestSplashWaitsForEnter(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("AGENTWATCH_DIR", t.TempDir())
	newModel := func(age time.Duration) *model {
		return &model{st: store.New(store.Options{}), splash: true, splashStart: time.Now().Add(-age)}
	}

	// mid-animation: no key skips it, not even Enter
	m := newModel(300 * time.Millisecond)
	for _, k := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("x")}, {Type: tea.KeySpace}, {Type: tea.KeyEnter}} {
		m.Update(k)
	}
	if !m.splash {
		t.Fatal("the animation must not be skippable")
	}

	// finished: it keeps waiting (ticks don't end it) and shows the prompt
	m = newModel(splashTotal + time.Second)
	m.Update(tickMsg(time.Now()))
	if !m.splash {
		t.Fatal("the splash should wait for Enter, not close by itself")
	}
	if !strings.Contains(ansi.Strip(m.View()), enterPrompt) {
		t.Error("the Enter prompt should show once the animation is done")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if !m.splash {
		t.Error("other keys should not open the dashboard")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.splash {
		t.Error("Enter should open the dashboard")
	}

	// q still quits from the splash
	if _, cmd := newModel(0).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q should quit")
	}
}
