package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// fit memotong/mengisi s agar tepat selebar w kolom.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// box menggambar kotak dengan judul di garis atas, lebar total = w.
func box(title string, border lipgloss.Color, w int, lines []string, minH int) string {
	if w < 8 {
		w = 8
	}
	bs := lipgloss.NewStyle().Foreground(border)
	inner := w - 4
	t := ""
	if title != "" {
		t = " " + ansi.Truncate(title, inner-2, "…") + " "
	}
	top := bs.Render("╭─") + t + bs.Render(strings.Repeat("─", max(0, w-3-lipgloss.Width(t)))+"╮")
	var b strings.Builder
	b.WriteString(top)
	for len(lines) < minH {
		lines = append(lines, "")
	}
	for _, l := range lines {
		b.WriteString("\n" + bs.Render("│") + " " + fit(l, inner) + " " + bs.Render("│"))
	}
	b.WriteString("\n" + bs.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return b.String()
}

// pulse: warna border yang "bernafas" antara redup dan terang (siklus ~1.6s)
// untuk kotak yang sedang aktif (lead working, advisor memberi saran, subagent running).
func pulse(c lipgloss.Color) lipgloss.Color {
	const period = 1600 * time.Millisecond
	ph := float64(time.Now().UnixNano()%int64(period)) / float64(period)
	k := 0.5 - 0.5*math.Cos(2*math.Pi*ph) // 0 → 1 → 0
	dim := blend(c, cBorder, 0.65)
	bright := blend(c, lipgloss.Color("#FFFFFF"), 0.35)
	return blend(dim, bright, k)
}

// blend: campur warna hex a → b sebesar t (0..1). Kalau bukan hex, kembalikan a.
func blend(a, b lipgloss.Color, t float64) lipgloss.Color {
	ar, ag, ab, ok1 := hexRGB(string(a))
	br, bg, bb, ok2 := hexRGB(string(b))
	if !ok1 || !ok2 {
		return a
	}
	mix := func(x, y int) int { return int(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", mix(ar, br), mix(ag, bg), mix(ab, bb)))
}

func hexRGB(h string) (r, g, b int, ok bool) {
	if len(h) != 7 || h[0] != '#' {
		return 0, 0, 0, false
	}
	_, err := fmt.Sscanf(h[1:], "%02x%02x%02x", &r, &g, &b)
	return r, g, b, err == nil
}

func kv(k, v string) string { return sDim.Render(fmt.Sprintf("%-7s", k)) + " " + v }

func bar(frac float64, w int, c lipgloss.Color) string {
	frac = min(max(frac, 0), 1)
	n := int(frac*float64(w) + 0.5)
	return lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("█", n)) +
		lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("░", w-n))
}

// joinGap menyusun item berdampingan dengan jarak satu kolom.
func joinGap(items []string) string {
	var withGap []string
	for i, it := range items {
		if i > 0 {
			withGap = append(withGap, " ")
		}
		withGap = append(withGap, it)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, withGap...)
}

// scrollWindow memotong lines menjadi jendela setinggi rows yang digeser scroll baris dari bawah.
// Mengembalikan jendela dan nilai scroll yang sudah dibatasi.
func scrollWindow(lines []string, rows, scroll int) ([]string, int) {
	if rows <= 0 {
		return nil, 0
	}
	maxScroll := max(0, len(lines)-rows)
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := len(lines) - scroll
	start := max(0, end-rows)
	return lines[start:end], scroll
}
