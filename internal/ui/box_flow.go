package ui

import (
	"fmt"
	"time"

	"github.com/chicken-afk/agentwatch/internal/format"
	"github.com/chicken-afk/agentwatch/internal/store"

	"github.com/charmbracelet/lipgloss"
)

// flowBox: compact model chain + latest steps. rows = number of steps shown.
func (m *model) flowBox(s *store.Session, w, rows int) string {
	steps := s.Flow()
	lines := []string{flowChain(steps, w-4-2), ""}
	for _, st := range steps[max(0, len(steps)-rows):] {
		lines = append(lines, flowLine(st))
	}
	if len(steps) == 0 {
		lines = []string{sDim.Render("waiting for the first message…")}
	}
	title := sBold.Render("MODEL SEQUENCE") + sDim.Render(fmt.Sprintf(" · %d steps · tab 2 for full view", len(steps)))
	return box(title, cBorder, w, lines, 0)
}

// flowChain: a compact chain "sonnet → opus (advisor) → haiku (Explore) → sonnet",
// trimmed from the left with "…" to fit within width columns.
func flowChain(steps []store.FlowStep, width int) string {
	var chain []string
	for _, st := range steps {
		label := format.PrettyModel(st.Model)
		if st.Role != "lead" {
			label += sDim.Render(" (" + st.Role + ")")
		}
		chain = append(chain, mc(st.Model, "● ")+mc(st.Model, label))
	}
	arrow := sDim.Render(" → ")
	line := ""
	for i := len(chain) - 1; i >= 0; i-- {
		cand := chain[i]
		if line != "" {
			cand += arrow + line
		}
		if lipgloss.Width(cand) > width {
			line = sDim.Render("… ") + line
			break
		}
		line = cand
	}
	return line
}

func flowLine(st store.FlowStep) string {
	d := ""
	if span := st.End.Sub(st.Start); span >= time.Second {
		d = " · " + format.Dur(span)
	}
	info := fmt.Sprintf("%d msgs", st.Count)
	if st.Role == "advisor" {
		info = fmt.Sprintf("%d calls", st.Count)
	}
	if st.Tools > 0 {
		info += fmt.Sprintf(" · %d tools", st.Tools)
	}
	return sDim.Render(format.Clock(st.Start)) + "  " + mc(st.Model, "● "+fmt.Sprintf("%-11s", format.PrettyModel(st.Model))) +
		" " + sText.Render(fmt.Sprintf("%-16s", format.Clip(st.Role, 16))) + sDim.Render(info+d)
}

// flowView: tab 2, every model step with markers for long gaps.
func (m *model) flowView(s *store.Session, w, h int) string {
	steps := s.Flow()
	var lines []string
	var prev time.Time
	for i, st := range steps {
		gap := ""
		if i > 0 && st.Start.Sub(prev) > 30*time.Second {
			gap = sDim.Render(fmt.Sprintf("   (gap %s)", format.Dur(st.Start.Sub(prev))))
		}
		lines = append(lines, sDim.Render(fmt.Sprintf("%4d ", i+1))+flowLine(st)+gap)
		prev = st.End
	}
	if len(lines) == 0 {
		lines = []string{sDim.Render("no model messages yet")}
	}
	view, sc := scrollWindow(lines, h-2, m.scroll)
	m.scroll = sc
	title := sBold.Render("FULL MODEL SEQUENCE") + sDim.Render(fmt.Sprintf(" · %d steps · j/k scroll", len(steps)))
	return box(title, cBorder, w, view, h-2)
}
