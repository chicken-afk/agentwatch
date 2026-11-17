package ui

import (
	"github.com/chicken-afk/agentwatch/internal/store"

	"github.com/charmbracelet/lipgloss"
)

const (
	wideMinWidth = 110 // below this, boxes are stacked vertically
	minLogRows   = 6   // minimum log rows kept before the model sequence fills the rest
)

// sideBox: the ADVISOR box for Claude Code; other agents have no advisor, so they get an AGENT info box.
func (m *model) sideBox(s *store.Session, w, h int) string {
	if s.IsClaude() {
		return m.advisorBox(s, w, h)
	}
	return m.agentInfoBox(s, w, h)
}

// dashboard: tab 1. Wide screen → left column (ADVISOR + SKILLS) and right (LEAD, MODEL SEQUENCE,
// SUBAGENTS); narrow screen → everything stacked. MODEL USAGE and SESSION LOG at the bottom.
func (m *model) dashboard(s *store.Session, w, h int) string {
	wide := w >= wideMinWidth
	// left column (ADVISOR + SKILLS) scales with terminal width: ±36%, 40–60 columns
	leftW := min(60, max(40, w*36/100))
	rightW, skillW := w-leftW-1, leftW
	if !wide {
		rightW, skillW = w, w
	}

	lead := m.leadBox(s, rightW)
	skills := m.skillsBox(s, skillW)
	agents := m.agentGrid(s, rightW)
	usage := m.usageBox(s, w)

	fixed := lipgloss.Height(lead) + lipgloss.Height(agents) + lipgloss.Height(usage) + minLogRows
	flowRows := max(3, min(10, h-fixed-2-2))
	flow := m.flowBox(s, rightW, flowRows)

	var top string
	if wide {
		right := lipgloss.JoinVertical(lipgloss.Left, lead, flow, agents)
		advH := max(8, lipgloss.Height(right)-lipgloss.Height(skills))
		left := lipgloss.JoinVertical(lipgloss.Left, m.sideBox(s, leftW, advH), skills)
		top = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	} else {
		top = lipgloss.JoinVertical(lipgloss.Left, lead, m.sideBox(s, leftW, 0), skills, flow, agents)
	}

	parts := []string{top, usage}
	if logRows := h - lipgloss.Height(top) - lipgloss.Height(usage) - 2; logRows >= 2 {
		parts = append(parts, m.logBox(s, w, logRows))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
