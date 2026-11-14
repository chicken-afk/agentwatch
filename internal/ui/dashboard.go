package ui

import (
	"ccwatch/internal/store"

	"github.com/charmbracelet/lipgloss"
)

const (
	wideMinWidth = 110 // di bawah ini kotak ditumpuk vertikal
	minLogRows   = 6   // ruang minimal log yang disisakan sebelum urutan model mengisi sisanya
)

// dashboard: tab 1. Layar lebar → kolom kiri (ADVISOR + SKILLS) dan kanan (LEAD, URUTAN MODEL,
// SUBAGENT); layar sempit → semuanya ditumpuk. PEMAKAIAN MODEL dan SESSION LOG di bawah.
func (m *model) dashboard(s *store.Session, w, h int) string {
	wide := w >= wideMinWidth
	// kolom kiri (ADVISOR + SKILLS) ikut lebar terminal: ±36%, 40–60 kolom
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
		left := lipgloss.JoinVertical(lipgloss.Left, m.advisorBox(s, leftW, advH), skills)
		top = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	} else {
		top = lipgloss.JoinVertical(lipgloss.Left, lead, m.advisorBox(s, leftW, 0), skills, flow, agents)
	}

	parts := []string{top, usage}
	if logRows := h - lipgloss.Height(top) - lipgloss.Height(usage) - 2; logRows >= 2 {
		parts = append(parts, m.logBox(s, w, logRows))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
