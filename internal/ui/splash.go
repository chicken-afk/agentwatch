package ui

import (
	"math"
	"strings"
	"time"

	"github.com/chicken-afk/agentwatch/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

// Startup splash: a scan beam sweeps across the "agentwatch" wordmark and reveals it in
// the colors of the agents it watches, the agents check in one by one, then the credit
// types itself in. The animation always plays to the end; then an Enter prompt appears and
// only Enter opens the dashboard (--no-splash / AGENTWATCH_NO_SPLASH=1 turns it off).

const (
	splashCredit = "by Titikkoma.dev"

	scanEnd     = 700 * time.Millisecond  // beam has crossed the wordmark
	rollStart   = 650 * time.Millisecond  // first agent checks in
	rollStep    = 90 * time.Millisecond   // gap between agents
	creditStart = 1150 * time.Millisecond // credit starts typing
	creditChar  = 28 * time.Millisecond   // per character
	splashTotal = 1900 * time.Millisecond // animation done: the Enter prompt appears
	promptFade  = 400 * time.Millisecond  // the prompt fades in over this
	splashTick  = 33 * time.Millisecond   // ~30 fps while the splash animates
	splashIdle  = 100 * time.Millisecond  // slower ticks once it only waits for Enter

	enterPrompt = "press enter to open the dashboard"
)

// splashAgents: the roll call, in the order of the wordmark gradient.
var splashAgents = []struct {
	name  string
	color lipgloss.Color
}{
	{"claude", theme.AgentColor("claude")},
	{"codex", theme.AgentColor("codex")},
	{"opencode", theme.AgentColor("opencode")},
	{"omp", theme.AgentColor("omp")},
	{"gemini", theme.AgentColor("gemini")},
}

// glyphs: an 8-pixel-tall lowercase font (rows 0–1 ascender, 2–6 x-height, 7 descender),
// drawn two pixel rows per terminal line with half blocks.
var glyphs = map[rune][8]string{
	'a': {"     ", "     ", " ### ", "    #", " ####", "#   #", " ####", "     "},
	'g': {"     ", "     ", " ####", "#   #", "#   #", " ####", "    #", " ### "},
	'e': {"     ", "     ", " ### ", "#   #", "#####", "#    ", " ####", "     "},
	'n': {"     ", "     ", "#### ", "#   #", "#   #", "#   #", "#   #", "     "},
	't': {" #  ", " #  ", "####", " #  ", " #  ", " #  ", "  ##", "    "},
	'w': {"     ", "     ", "#   #", "#   #", "# # #", "# # #", " # # ", "     "},
	'c': {"     ", "     ", " ####", "#    ", "#    ", "#    ", " ####", "     "},
	'h': {"#    ", "#    ", "#### ", "#   #", "#   #", "#   #", "#   #", "     "},
}

// wordmark: the pixel grid of text (rows × columns), one column of space between letters.
func wordmark(text string) [8][]bool {
	var px [8][]bool
	for i, r := range text {
		g := glyphs[r]
		if i > 0 {
			for y := range px {
				px[y] = append(px[y], false)
			}
		}
		for y := range px {
			for _, c := range g[y] {
				px[y] = append(px[y], c == '#')
			}
		}
	}
	return px
}

var logo = wordmark("agentwatch")

// gradient: the agent color at position f (0..1) along the wordmark.
func gradient(f float64) lipgloss.Color {
	n := len(splashAgents) - 1
	pos := min(max(f, 0), 1) * float64(n)
	i := min(int(pos), n-1)
	return blend(splashAgents[i].color, splashAgents[i+1].color, pos-float64(i))
}

// ease: smooth start and stop (0..1 → 0..1).
func ease(x float64) float64 {
	x = min(max(x, 0), 1)
	return x * x * (3 - 2*x)
}

// logoLines renders the wordmark with the scan beam at column beam (in pixel columns).
func logoLines(beam float64) []string {
	w := len(logo[0])
	lines := make([]string, 4)
	for row := 0; row < 4; row++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			top, bot := logo[row*2][x], logo[row*2+1][x]
			ch := halfBlock(top, bot)
			d := beam - float64(x) // > 0: already scanned
			switch {
			case d >= 0 && d < 2.5: // the beam itself: a bright column of light
				c := blend(lipgloss.Color("#FFFFFF"), gradient(float64(x)/float64(w)), d/2.5)
				if ch == " " {
					ch = "│"
					c = blend(c, cBorder, 0.55)
				}
				b.WriteString(lipgloss.NewStyle().Foreground(c).Render(ch))
			case d >= 2.5:
				if ch == " " {
					b.WriteString(" ")
					continue
				}
				b.WriteString(lipgloss.NewStyle().Foreground(gradient(float64(x) / float64(w))).Render(ch))
			default: // not scanned yet: a faint outline of what is about to appear
				if top || bot {
					b.WriteString(lipgloss.NewStyle().Foreground(cBorder).Render("·"))
				} else {
					b.WriteString(" ")
				}
			}
		}
		lines[row] = b.String()
	}
	return lines
}

func halfBlock(top, bot bool) string {
	switch {
	case top && bot:
		return "█"
	case top:
		return "▀"
	case bot:
		return "▄"
	}
	return " "
}

// rollCall: the agents that have checked in by time t.
func rollCall(t time.Duration) string {
	var parts []string
	for i, a := range splashAgents {
		at := rollStart + time.Duration(i)*rollStep
		if t < at {
			parts = append(parts, strings.Repeat(" ", 2+len(a.name)))
			continue
		}
		// each dot lands bright, then settles into its color
		k := ease(float64(t-at) / float64(180*time.Millisecond))
		dot := lipgloss.NewStyle().Foreground(blend(lipgloss.Color("#FFFFFF"), a.color, k)).Render("●")
		name := lipgloss.NewStyle().Foreground(blend(cBorder, cText, k)).Render(a.name)
		parts = append(parts, dot+" "+name)
	}
	return strings.Join(parts, "   ")
}

// credit: the credit line typed up to time t, with a blinking cursor.
func credit(t time.Duration) string {
	if t < creditStart {
		return " "
	}
	n := min(len(splashCredit), int((t-creditStart)/creditChar)+1)
	by, name, _ := strings.Cut(splashCredit[:n], " ")
	out := sDim.Render(by)
	if n > len("by") {
		out += " " + sText.Bold(true).Render(name)
	}
	cursor := " "
	if n < len(splashCredit) || (t/(265*time.Millisecond))%2 == 0 {
		cursor = "▌"
	}
	return out + lipgloss.NewStyle().Foreground(gradient(1)).Render(cursor)
}

// prompt: the Enter prompt once the animation is done. It fades in, then breathes
// slowly between dim and bright so it reads as waiting for you.
func prompt(t time.Duration) string {
	if t < splashTotal {
		return " "
	}
	in := ease(float64(t-splashTotal) / float64(promptFade))
	phase := float64((t-splashTotal)%(2*time.Second)) / float64(2*time.Second)
	breath := 0.5 - 0.5*math.Cos(2*math.Pi*phase)
	st := lipgloss.NewStyle().Foreground(blend(cBorder, cText, in*(0.55+0.45*breath)))
	key := lipgloss.NewStyle().Foreground(blend(cBorder, gradient(1), in)).Bold(true).Render("enter")
	before, after, _ := strings.Cut(enterPrompt, "enter")
	return st.Render(before) + key + st.Render(after)
}

// splashReady: whether the animation has finished and Enter is accepted.
func splashReady(t time.Duration) bool { return t >= splashTotal }

// splashView renders the splash frame at time t since launch, centered in w×h.
func splashView(w, h int, t time.Duration) string {
	if w < len(logo[0])+4 || h < 13 {
		return splashCompact(w, h, t)
	}
	beam := ease(float64(t)/float64(scanEnd)) * float64(len(logo[0])+3)
	lines := logoLines(beam)
	lines = append(lines, "", "", rollCall(t), "", credit(t), "", "", prompt(t))
	block := lipgloss.JoinVertical(lipgloss.Center, lines...)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// splashCompact: small terminals get the name, the agent dots and the credit.
func splashCompact(w, h int, t time.Duration) string {
	word := "agentwatch"
	shown := int(math.Ceil(ease(float64(t)/float64(scanEnd)) * float64(len(word))))
	var name strings.Builder
	for i, r := range word[:shown] {
		name.WriteString(lipgloss.NewStyle().Foreground(gradient(float64(i) / float64(len(word)-1))).Bold(true).Render(string(r)))
	}
	var dots []string
	for i, a := range splashAgents {
		if t >= rollStart+time.Duration(i)*rollStep {
			dots = append(dots, lipgloss.NewStyle().Foreground(a.color).Render("●"))
		}
	}
	block := lipgloss.JoinVertical(lipgloss.Center, name.String(), strings.Join(dots, " "), credit(t), "", prompt(t))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// SplashFrame renders one splash frame (for previews and tests).
func SplashFrame(w, h int, t time.Duration) string { return splashView(w, h, t) }
